//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/mastersync"
	"github.com/EziosWJ/weighbridge/server/internal/tripsync"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresTemporaryPartyOfflineWeighingAndMerge(t *testing.T) {
	ctx := context.Background()
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	const site, scale, plate = "F001", "WB01", "晋A66666"
	var snapshotResponse, tripResponse atomic.Value
	snapshotResponse.Store("")
	tripResponse.Store("")
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("siteCode") != site {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body string
		switch r.URL.Path {
		case "/api/v1/weighbridge/sync/master":
			body = snapshotResponse.Load().(string)
		case "/api/v1/weighbridge/sync/trips/TRIP-1":
			body = tripResponse.Load().(string)
		}
		if body == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, `{"code":200,"message":"ok","data":%s}`, body)
	}))
	defer cloud.Close()

	dataDir := t.TempDir()
	deps := testDependencies(t, db, dataDir)
	repo := weighing.NewRepository(db.GORM)
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, scaleCode string) (agentbridge.Config, error) {
		return repo.AgentConfig(ctx, scaleCode, dataDir)
	})
	service := weighing.NewService(repo, hub)
	deps.Weighing = service
	deps.Camera, _, _ = newCaptureService(t, db, dataDir)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	ensureLocalSite(t, router, admin, 1, site, "本厂", scale, "本厂地磅", "integration-agent")
	addCaptureCameras(t, db, "rtsp://camera/site-1", "rtsp://camera/site-2")
	config, err := repo.AgentConfig(ctx, scale, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	device := startLocalSiteMock(t, router, hub, scale, config.ID, config.Key)
	if err := service.ConfigureMasterSync(ctx, true, cloud.URL); err != nil {
		t.Fatal(err)
	}
	snapshot := mastersync.Snapshot{
		SchemaVersion: 2, SiteCode: site,
		Categories:            []mastersync.Category{},
		Materials:             []mastersync.Material{{Code: "C-M-0001", Name: "高粱", Enabled: true}},
		Parties:               []mastersync.Party{},
		TemporaryPartyResults: []mastersync.TemporaryPartyResult{},
	}
	syncMaster := func(success bool) {
		t.Helper()
		snapshotResponse.Store(encodeLocalSite(t, snapshot))
		if err := service.RunMasterSync(ctx, "manual"); (err == nil) != success {
			t.Fatalf("master sync, expected success %t, got %v", success, err)
		}
	}
	syncMaster(true)

	snapshotResponse.Store("")
	if err := service.RunMasterSync(ctx, "manual"); err == nil {
		t.Fatal("expected cloud outage")
	}
	const name = "现场临时供货方"
	pending := weighingData[weighing.Party](t, router, http.MethodPost, "/api/weighing/temporary-parties",
		`{"name":"现场临时供货方","nature":"individual","roles":["supplier","carrier"]}`, admin)
	discarded := weighingData[weighing.Party](t, router, http.MethodPost, "/api/weighing/temporary-parties",
		`{"name":"错误登记","nature":"enterprise","roles":["customer"]}`, admin)
	getCandidates := func(role string) []weighing.Party {
		t.Helper()
		return weighingData[weighing.Page[weighing.Party]](t, router, http.MethodGet,
			"/api/weighing/parties?enabled=1&role="+role, "", admin).Records
	}
	assertCandidate := func(role string, id int64, want bool) {
		t.Helper()
		for _, candidate := range getCandidates(role) {
			if candidate.ID == id {
				if !want {
					t.Fatalf("role %s unexpectedly includes %+v", role, candidate)
				}
				return
			}
		}
		if want {
			t.Fatalf("role %s missing party %d", role, id)
		}
	}
	assertCandidate("supplier", pending.ID, true)
	assertCandidate("carrier", pending.ID, true)
	assertCandidate("customer", pending.ID, false)
	assertCandidate("customer", discarded.ID, true)
	materials := weighingData[weighing.Page[weighing.Material]](t, router, http.MethodGet, "/api/weighing/materials?enabled=1", "", admin)

	session := weighingData[weighing.SessionResult](t, router, http.MethodPost,
		"/api/weighing/scales/"+scale+"/session/acquire", `{"clientId":"temp-party-test"}`, admin)
	business := map[string]any{"plate": plate, "direction": "inbound", "materialId": materials.Records[0].ID, "partyId": pending.ID, "carrierId": pending.ID}
	sampleInput := map[string]any{"scaleCode": scale, "clientId": "temp-party-test", "sessionToken": session.SessionToken, "source": "device", "business": business}
	firstSample := weighingData[weighing.Sample](t, router, http.MethodPost, "/api/weighing/samples", encodeLocalSite(t, sampleInput), admin)
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	firstInput := map[string]any{"scaleCode": scale, "clientId": "temp-party-test", "sessionToken": session.SessionToken,
		"sampleId": firstSample.ID, "requestId": "temporary-party-first", "business": business, "checks": checks}
	// A cloud response carrying the same temporary code still cannot bind this supplier.
	tripResponse.Store(encodeLocalSite(t, tripsync.Trip{TripNo: "TRIP-1", Version: 1, Plate: plate,
		MaterialCode: materials.Records[0].Code, PartyCode: pending.Code, Linkable: true}))
	firstInput["trip"] = map[string]any{"tripNo": "TRIP-1", "tripVersion": 1}
	rejected := serveJSON(router, http.MethodPost, "/api/weighing/tasks/first-confirmation", encodeLocalSite(t, firstInput), admin)
	assertEnvelopeCode(t, rejected, http.StatusOK, 400, "流转单须使用总部正式供货方")
	delete(firstInput, "trip")
	first := weighingData[weighing.CommandResult](t, router, http.MethodPost, "/api/weighing/tasks/first-confirmation", encodeLocalSite(t, firstInput), admin)

	// Neither a name match nor a target missing one of the current roles can switch candidates.
	formalCode := "C-P-0001"
	snapshot.Parties = []mastersync.Party{{Code: formalCode, Name: name, Nature: "individual", Enabled: true, IsSupplier: true, Contacts: []mastersync.Contact{}}}
	syncMaster(true)
	assertCandidate("supplier", pending.ID, true)
	var formalID int64
	for _, candidate := range getCandidates("supplier") {
		if candidate.Code == formalCode {
			formalID = candidate.ID
		}
	}
	if formalID == 0 {
		t.Fatal("same-name formal party was not available alongside temporary party")
	}
	corrected := weighingData[weighing.Party](t, router, http.MethodPut, fmt.Sprintf("/api/weighing/temporary-parties/%d", pending.ID),
		`{"name":"核实后的临时供货方","nature":"individual","roles":["supplier","carrier"],"version":1}`, admin)
	if corrected.Code != pending.Code || corrected.Version != 2 || corrected.Name == name {
		t.Fatalf("temporary correction did not keep the identity and advance the version: %+v", corrected)
	}
	snapshot.TemporaryPartyResults = []mastersync.TemporaryPartyResult{
		{TemporaryCode: pending.Code, Version: corrected.Version, Status: "merged", FormalPartyCode: &formalCode},
		{TemporaryCode: discarded.Code, Version: 1, Status: "disabled"},
	}
	syncMaster(true)
	assertCandidate("supplier", pending.ID, true)
	assertCandidate("customer", discarded.ID, false)

	// Master omission rejects the whole update, including otherwise valid results.
	masterMaterial := snapshot.Materials
	snapshot.Materials = []mastersync.Material{}
	snapshot.Parties[0].IsCarrier = true
	syncMaster(false)
	assertCandidate("supplier", pending.ID, true)
	assertCandidate("carrier", formalID, false)
	snapshot.Materials = masterMaterial
	syncMaster(true)
	assertCandidate("supplier", pending.ID, false)
	assertCandidate("carrier", formalID, true)
	merged := weighingData[weighing.Party](t, router, http.MethodGet, fmt.Sprintf("/api/weighing/parties/%d", pending.ID), "", admin)
	if merged.TemporaryState != "merged" || merged.Code != pending.Code || merged.Enabled != 0 {
		t.Fatalf("temporary record after merge: %+v", merged)
	}

	snapshot.Parties[0].Enabled = false
	syncMaster(true)
	assertCandidate("supplier", formalID, false)
	assertCandidate("supplier", pending.ID, false)
	rejected = serveJSON(router, http.MethodPost, "/api/weighing/samples", encodeLocalSite(t, sampleInput), admin)
	assertEnvelopeCode(t, rejected, http.StatusOK, 400, "往来主体已不可用于该业务，请重新选择")

	device.weight("10")
	sampleInput["taskId"] = first.TaskID
	secondSample := weighingData[weighing.Sample](t, router, http.MethodPost, "/api/weighing/samples", encodeLocalSite(t, sampleInput), admin)
	secondInput := map[string]any{"scaleCode": scale, "clientId": "temp-party-test", "sessionToken": session.SessionToken,
		"taskId": first.TaskID, "sampleId": secondSample.ID, "requestId": "temporary-party-second", "version": first.TaskVersion, "checks": checks}
	completed := weighingData[weighing.CommandResult](t, router, http.MethodPost, fmt.Sprintf("/api/weighing/tasks/%d/second-confirmation", first.TaskID), encodeLocalSite(t, secondInput), admin)
	ticket := weighingData[weighing.TicketView](t, router, http.MethodGet, fmt.Sprintf("/api/weighing/tickets/%d", completed.TicketID), "", admin)
	if ticket.Business.PartyCode != pending.Code || ticket.Business.PartyName != name || ticket.Business.CarrierCode != pending.Code {
		t.Fatalf("ticket lost its original party snapshots: %+v", ticket.Business)
	}
	var payload []byte
	if err := db.GORM.Raw("SELECT payload FROM wb_ticket_sync_change_v5 WHERE ticket_no = ?", ticket.TicketNo).Row().Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var upload struct {
		Ticket struct{ PartyCode, PartyName string }
	}
	if err := json.Unmarshal(payload, &upload); err != nil || upload.Ticket.PartyCode != pending.Code || upload.Ticket.PartyName != name {
		t.Fatalf("ticket upload lost its original party: %+v (%v)", upload, err)
	}

	snapshot.Parties[0].Enabled = true
	syncMaster(true)
	device.weight("30")
	delete(sampleInput, "taskId")
	delete(business, "carrierId")
	business["partyId"] = formalID
	tripResponse.Store(encodeLocalSite(t, tripsync.Trip{TripNo: "TRIP-1", Version: 1, Plate: plate,
		MaterialCode: materials.Records[0].Code, PartyCode: formalCode, Linkable: true}))
	formalSample := weighingData[weighing.Sample](t, router, http.MethodPost, "/api/weighing/samples", encodeLocalSite(t, sampleInput), admin)
	firstInput["trip"] = map[string]any{"tripNo": "TRIP-1", "tripVersion": 1}
	firstInput["requestId"] = "formal-grain-party-first"
	firstInput["sampleId"] = formalSample.ID
	grain := weighingData[weighing.CommandResult](t, router, http.MethodPost, "/api/weighing/tasks/first-confirmation", encodeLocalSite(t, firstInput), admin)
	basis := weighingData[weighing.TaskTripBasis](t, router, http.MethodGet, fmt.Sprintf("/api/weighing/tasks/%d/trip-basis", grain.TaskID), "", admin)
	if !basis.Bound || basis.Trip == nil || basis.Trip.PartyCode != formalCode {
		t.Fatalf("formal supplier did not bind the grain trip: %+v", basis)
	}
}
