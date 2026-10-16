package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/gin-gonic/gin"
)

type testAuthorization struct{ admin bool }

func (a *testAuthorization) IsAdmin(context.Context, int64) (bool, error) { return a.admin, nil }

type testAuthentication struct{}

func (testAuthentication) Authenticate(_ context.Context, token string) (auth.Principal, error) {
	if token != "Bearer test-session" {
		return auth.Principal{}, errors.New("invalid session")
	}
	return auth.Principal{UserID: 1, JTI: "test-session"}, nil
}
func testRouter(service *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1", auth.BearerMiddleware(testAuthentication{}))
	RegisterRoutes(api, NewHandler(service))
	return r
}
func request(t *testing.T, r http.Handler, path string, out any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer test-session")
	result := httptest.NewRecorder()
	r.ServeHTTP(result, req)
	if out != nil {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(result.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			t.Fatal(err)
		}
	}
	return result
}
func controlledSample(at time.Time, cpu, memory float64) Snapshot {
	return Snapshot{SourceID: "host-1", SampledAt: at,
		Host:   Region[Host]{Status: StatusOK, CollectedAt: &at, Data: &Host{Hostname: "real-host"}},
		CPU:    Region[CPU]{Status: StatusOK, CollectedAt: &at, Data: &CPU{UsagePercent: ptr(cpu), LogicalCores: 2, PerCore: []*float64{ptr(cpu), ptr(cpu)}}},
		Memory: Region[Memory]{Status: StatusOK, CollectedAt: &at, Data: &Memory{UsagePercent: memory}},
	}
}
func overview(t *testing.T, r http.Handler) Snapshot {
	t.Helper()
	var out Snapshot
	result := request(t, r, "/api/v1/monitoring/overview", &out)
	if result.Code != 200 {
		t.Fatalf("overview: %d %s", result.Code, result.Body.String())
	}
	return out
}
func history(t *testing.T, r http.Handler, resource string) History {
	t.Helper()
	var out History
	result := request(t, r, "/api/v1/monitoring/history?resource="+resource, &out)
	if result.Code != 200 {
		t.Fatalf("history: %d %s", result.Code, result.Body.String())
	}
	return out
}

func TestHTTPFreshnessPartialFailureAndHistoryGaps(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	authorization := &testAuthorization{admin: true}
	svc := NewService(authorization, nil, func() time.Time { return now })
	router := testRouter(svc)
	if got := overview(t, router); got.CPU.Status != StatusCollecting || got.CPU.Data != nil {
		t.Fatalf("initial snapshot=%+v", got)
	}
	if len(history(t, router, "cpu").Points) != 0 {
		t.Fatal("first screen fabricated history")
	}
	first := controlledSample(now, 50, 60)
	svc.Ingest(first)
	now = now.Add(5 * time.Second)
	partial := controlledSample(now, 55, 65)
	partial.CPU = Region[CPU]{Status: StatusError, Message: "permission denied"}
	svc.Ingest(partial)
	got := overview(t, router)
	if got.CPU.Status != StatusError || *got.CPU.Data.UsagePercent != 50 || !got.CPU.CollectedAt.Equal(*first.CPU.CollectedAt) {
		t.Fatalf("CPU failure lost old sample=%+v", got.CPU)
	}
	if got.Memory.Data.UsagePercent != 65 || got.Memory.Status != StatusOK {
		t.Fatalf("other area failed=%+v", got.Memory)
	}
	points := history(t, router, "cpu").Points
	if len(points) != 2 || points[1].Values["usagePercent"] != nil {
		t.Fatalf("missing CPU gap=%+v", points)
	}
	now = now.Add(5 * time.Second)
	svc.Ingest(partial)
	got = overview(t, router)
	if !got.Memory.CollectedAt.Equal(*partial.Memory.CollectedAt) {
		t.Fatal("duplicate became fresh")
	}
	points = history(t, router, "memory").Points
	if len(points) != 3 || points[2].Values["usagePercent"] != nil {
		t.Fatalf("duplicate fabricated history=%+v", points)
	}
	now = first.SampledAt.Add(15 * time.Second)
	if overview(t, router).CPU.Stale {
		t.Fatal("15 second boundary prematurely stale")
	}
	now = now.Add(time.Nanosecond)
	if !overview(t, router).CPU.Stale {
		t.Fatal("older than 15 seconds not stale")
	}
	now = partial.SampledAt.Add(16 * time.Second)
	if got = overview(t, router); got.Memory.Status != StatusStale || !got.Memory.Stale {
		t.Fatal("normal old area not stale")
	}
	authorization.admin = false
	for _, path := range []string{"/api/v1/monitoring/overview", "/api/v1/monitoring/history?resource=cpu"} {
		if result := request(t, router, path, nil); result.Code != 403 {
			t.Fatalf("revoked admin=%d", result.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitoring/overview", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != 401 {
		t.Fatalf("missing bearer=%d", recorder.Code)
	}
}

func TestHTTPThresholdDurationRecoveryAndInterruptions(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := NewService(&testAuthorization{true}, nil, func() time.Time { return now })
	router := testRouter(svc)
	// Exact 90% boundary, 60 seconds trigger. No long sleeps.
	for i := 0; i <= 12; i++ {
		svc.Ingest(controlledSample(now, 90, 90))
		count := len(overview(t, router).Warnings)
		if i < 12 && count != 0 || i == 12 && count != 2 {
			t.Fatalf("at %d seconds warnings=%d", i*5, count)
		}
		now = now.Add(5 * time.Second)
	}
	// 85% is not recovery; only <85% continuously for 30 seconds recovers.
	svc.Ingest(controlledSample(now, 85, 85))
	if len(overview(t, router).Warnings) != 2 {
		t.Fatal("85% prematurely recovered")
	}
	now = now.Add(5 * time.Second)
	for i := 0; i <= 6; i++ {
		svc.Ingest(controlledSample(now, 84.99, 84.99))
		count := len(overview(t, router).Warnings)
		if i < 6 && count != 2 || i == 6 && count != 0 {
			t.Fatalf("recovery at %d seconds warnings=%d", i*5, count)
		}
		now = now.Add(5 * time.Second)
	}
	// Interrupt before 60 seconds and require a complete new duration.
	for i := 0; i < 10; i++ {
		svc.Ingest(controlledSample(now, 95, 50))
		now = now.Add(5 * time.Second)
	}
	svc.Ingest(Snapshot{CPU: Region[CPU]{Status: StatusError, Message: "interrupted"}})
	now = now.Add(5 * time.Second)
	for i := 0; i <= 12; i++ {
		svc.Ingest(controlledSample(now, 95, 50))
		count := len(overview(t, router).Warnings)
		if i < 12 && count != 0 || i == 12 && count != 1 {
			t.Fatalf("after interruption %d seconds warnings=%d", i*5, count)
		}
		now = now.Add(5 * time.Second)
	}
}

func TestHTTPHistoryWindowDevicesAndCollectorRestart(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := NewService(&testAuthorization{true}, nil, func() time.Time { return now })
	router := testRouter(svc)
	start := now
	for i := 0; i <= 362; i++ {
		sample := controlledSample(now, 50, 60)
		at := now
		disks := []Disk{{ID: "sda", ReadBytesPerSecond: ptr(1024), ReadIOPS: ptr(4)}}
		sample.Disks = Region[[]Disk]{Status: StatusOK, CollectedAt: &at, Data: &disks}
		svc.Ingest(sample)
		if i < 362 {
			now = now.Add(SampleInterval)
		}
	}
	result := history(t, router, "cpu")
	if len(result.Points) != 361 || result.Points[0].CollectedAt.Before(now.Add(-HistoryWindow)) || !result.Points[0].CollectedAt.After(start) {
		t.Fatalf("window=%d %+v", len(result.Points), result.Points[0])
	}
	var disk History
	request(t, router, "/api/v1/monitoring/history?resource=disk&device=sda", &disk)
	if *disk.Points[len(disk.Points)-1].Values["readIops"] != 4 {
		t.Fatal("missing device IOPS")
	}
	for _, path := range []string{"/api/v1/monitoring/history?resource=unknown", "/api/v1/monitoring/history?resource=cpu&device=-1", "/api/v1/monitoring/history?resource=memory&device=x"} {
		if request(t, router, path, nil).Code != 400 {
			t.Fatal("bad resource accepted")
		}
	}
	now = now.Add(5 * time.Second)
	sample := controlledSample(now, 50, 60)
	sample.SourceID = "restarted"
	sample.CPU.Data.UsagePercent = nil
	svc.Ingest(sample)
	result = history(t, router, "cpu")
	if len(result.Points) != 361 || result.Points[len(result.Points)-1].Values["usagePercent"] != nil {
		t.Fatal("collector restart lost history or gap")
	}
	if len(history(t, testRouter(NewService(&testAuthorization{true}, nil, func() time.Time { return now })), "cpu").Points) != 0 {
		t.Fatal("API restart retained history")
	}
	now = now.Add(31 * time.Minute)
	if len(history(t, router, "cpu").Points) != 0 {
		t.Fatal("query retained expired samples")
	}
}

func TestHTTPFilesystemImmediateHysteresisAndOldInitialSample(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := NewService(&testAuthorization{true}, nil, func() time.Time { return now })
	router := testRouter(svc)
	old := now.Add(-time.Minute)
	svc.Ingest(controlledSample(old, 10, 20))
	if got := overview(t, router); got.CPU.Status != StatusStale || got.CPU.Data == nil || !got.CPU.CollectedAt.Equal(old) {
		t.Fatal("initial remote old sample not marked stale")
	}
	for i, value := range []float64{90, 85, 84.99} {
		now = now.Add(5 * time.Second)
		sample := controlledSample(now, 10, 20)
		at := now
		fs := []Filesystem{{ID: "/", UsagePercent: ptr(value)}}
		sample.Filesystems = Region[[]Filesystem]{Status: StatusOK, CollectedAt: &at, Data: &fs}
		svc.Ingest(sample)
		count := len(overview(t, router).Warnings)
		if i < 2 && count != 1 || i == 2 && count != 0 {
			t.Fatalf("filesystem at %.2f warnings=%d", value, count)
		}
	}
}

type collectorFunc func(context.Context) (Snapshot, error)

func (f collectorFunc) Collect(ctx context.Context) (Snapshot, error) { return f(ctx) }
func TestSamplerTimeoutNoOverlapCancellationAndLateResult(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	svc := NewService(&testAuthorization{true}, collectorFunc(func(context.Context) (Snapshot, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return controlledSample(time.Now(), 99, 99), nil
	}), nil, WithTimeout(10*time.Millisecond))
	svc.interval = 15 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); svc.Run(ctx) }()
	<-started
	deadline := time.After(time.Second)
	for {
		if svc.Snapshot().CPU.Status == StatusError {
			break
		}
		select {
		case <-deadline:
			t.Fatal("sampler did not time out")
		case <-time.After(time.Millisecond):
		}
	}
	time.Sleep(40 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("timed out collection overlapped: %d", calls.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not exit while collector hung")
	}
	close(release)
	if svc.Snapshot().CPU.Data != nil {
		t.Fatal("late result made sample fresh")
	}
}

func TestHTTPHistoryUsesEachRegionsMeasurementTime(t *testing.T) {
	measured := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	now := measured.Add(10 * time.Second)
	svc := NewService(&testAuthorization{true}, nil, func() time.Time { return now })
	router := testRouter(svc)
	sample := controlledSample(measured, 25, 50)
	memoryTime := measured.Add(5 * time.Second)
	sample.Memory.CollectedAt = &memoryTime
	svc.Ingest(sample)
	if got := history(t, router, "cpu"); len(got.Points) != 1 || !got.Points[0].CollectedAt.Equal(measured) {
		t.Fatalf("CPU history shifted=%+v", got)
	}
	if got := history(t, router, "memory"); len(got.Points) != 1 || !got.Points[0].CollectedAt.Equal(memoryTime) {
		t.Fatalf("memory history shifted=%+v", got)
	}
	now = measured.Add(HistoryWindow)
	if len(history(t, router, "cpu").Points) != 1 {
		t.Fatal("exact history boundary removed too soon")
	}
	now = now.Add(time.Nanosecond)
	if len(history(t, router, "cpu").Points) != 0 || len(history(t, router, "memory").Points) != 1 {
		t.Fatal("window uses observation instead of measurement time")
	}
}

func TestHTTPSupportStateChangesPreserveExplanationAndRemoveAbsentWarnings(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := NewService(&testAuthorization{true}, nil, func() time.Time { return now })
	router := testRouter(svc)
	sample := controlledSample(now, 10, 20)
	at := now
	devices := []GPU{{ID: "gpu-1", UsagePercent: ptr(80)}}
	filesystems := []Filesystem{{ID: "/data", UsagePercent: ptr(95)}}
	sample.GPU = Region[[]GPU]{Status: StatusOK, CollectedAt: &at, Data: &devices}
	sample.Filesystems = Region[[]Filesystem]{Status: StatusOK, CollectedAt: &at, Data: &filesystems}
	svc.Ingest(sample)
	now = now.Add(SampleInterval)
	sample = controlledSample(now, 10, 20)
	sample.GPU = Region[[]GPU]{Status: StatusUnsupported, Message: "nvidia-smi missing"}
	sample.Filesystems = Region[[]Filesystem]{Status: StatusNoDevice, Message: "no filesystems"}
	svc.Ingest(sample)
	got := overview(t, router)
	if got.GPU.Status != StatusUnsupported || got.GPU.Message != "nvidia-smi missing" || !got.GPU.CollectedAt.Equal(at) {
		t.Fatalf("unsupported reason lost=%+v", got.GPU)
	}
	if got.Filesystems.Status != StatusNoDevice || got.Filesystems.Data != nil || len(got.Warnings) != 0 {
		t.Fatalf("absent device or warning retained=%+v", got)
	}
}

func TestHTTPDelayedSamplesAreChronologicalAcrossGaps(t *testing.T) {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	now := start
	svc := NewService(&testAuthorization{true}, nil, func() time.Time { return now })
	router := testRouter(svc)
	first := controlledSample(start, 30, 40)
	svc.Ingest(first)
	now = start.Add(5 * time.Second)
	svc.Ingest(first)
	now = start.Add(10 * time.Second)
	svc.Ingest(controlledSample(start.Add(3*time.Second), 50, 60))
	got := history(t, router, "cpu")
	if len(got.Points) != 3 || !got.Points[1].CollectedAt.Equal(start.Add(3*time.Second)) || got.Points[2].Values["usagePercent"] != nil {
		t.Fatalf("delayed sample sorted incorrectly: %+v", got.Points)
	}
	now = start.Add(15 * time.Second)
	svc.Ingest(controlledSample(start.Add(5*time.Second), 55, 65))
	got = history(t, router, "cpu")
	if len(got.Points) != 3 || got.Points[2].Values["usagePercent"] == nil || *got.Points[2].Values["usagePercent"] != 55 {
		t.Fatalf("measured point did not replace coinciding gap: %+v", got.Points)
	}
}
