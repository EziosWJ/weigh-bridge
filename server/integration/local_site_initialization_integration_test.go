//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresLocalSiteInitializationHTTP(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)

	status := weighingData[localsite.Status](t, router, "GET", "/api/weighing/local-status", "", admin)
	if status != (localsite.Status{}) {
		t.Fatalf("new database status = %+v, want unconfigured and uninitialized", status)
	}
	if response := serveJSON(router, http.MethodGet, "/api/weighing/site-config", "", admin); response.Code != http.StatusOK {
		t.Fatalf("configuration management unavailable before identity initialization: %d %s", response.Code, response.Body.String())
	}
	var capabilities map[string]any
	response := serveJSON(router, http.MethodGet, "/api/weighing/capabilities", "", admin)
	if err := json.Unmarshal(response.Body.Bytes(), &capabilities); err != nil {
		t.Fatal(err)
	}
	data, ok := capabilities["data"].(map[string]any)
	if !ok || data["localSiteCode"] != "" || data["configured"] != false || data["initialized"] != false {
		t.Fatalf("unconfigured capability payload = %#v", capabilities)
	}
	if _, exists := data["nodeRole"]; exists {
		t.Fatalf("capabilities expose retired nodeRole: %#v", data)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/weighing/materials", `{"name":"未配置厂区不得写入"}`, admin), http.StatusForbidden, http.StatusForbidden, "无权执行此操作")

	input := weighing.LocalSiteInput{SiteCode: "LOCAL01", ScaleCode: "SCALE01", Name: "一厂", ScaleName: "一号地磅"}
	// ADR 0033: initialization installs the head-office codes and refuses a
	// second identity.
	// Force a late failure after the transaction has inserted site, scale and
	// identity rows. The failed initialization must leave no partial state.
	if err := db.GORM.Exec(`CREATE FUNCTION integration_reject_agent_binding() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced integration failure'; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`CREATE TRIGGER integration_reject_agent_binding BEFORE INSERT ON wb_agent_binding FOR EACH ROW EXECUTE FUNCTION integration_reject_agent_binding()`).Error; err != nil {
		t.Fatal(err)
	}
	body := encodeLocalSiteInput(t, input)
	failed := serveJSON(router, http.MethodPost, "/api/weighing/site-config/initialize", body, admin)
	if failed.Code == http.StatusOK {
		t.Fatal("forced failure unexpectedly initialized the local site")
	}
	if err := db.GORM.Exec(`DROP TRIGGER integration_reject_agent_binding ON wb_agent_binding`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`DROP FUNCTION integration_reject_agent_binding()`).Error; err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int64{"wb_local_identity": 0, "wb_site": 0, "wb_scale": 0, "wb_agent_binding": 0, "wb_local_user_access": 0} {
		var count int64
		if err := db.GORM.Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatalf("failed initialization left %s rows=%d want=%d err=%v", table, count, want, err)
		}
	}

	// Concurrent first-time submissions serialize on the settings singleton;
	// exactly one succeeds and all others observe the fixed identity.
	const attempts = 8
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- serveJSON(router, http.MethodPost, "/api/weighing/site-config/initialize", body, admin)
		}()
	}
	wg.Wait()
	close(responses)
	succeeded, conflicted := 0, 0
	for response := range responses {
		code := envelopeCode(t, response)
		switch {
		case response.Code == http.StatusOK && code == http.StatusOK:
			succeeded++
		case response.Code == http.StatusOK && code == http.StatusBadRequest:
			conflicted++
		default:
			t.Errorf("concurrent initialization status=%d envelope=%d body=%s", response.Code, code, response.Body.String())
		}
	}
	if succeeded != 1 || conflicted != attempts-1 {
		t.Fatalf("concurrent initialization successes=%d conflicts=%d", succeeded, conflicted)
	}
	status = weighingData[localsite.Status](t, router, http.MethodGet, "/api/weighing/local-status", "", admin)
	if status.SiteCode != "LOCAL01" || status.ScaleCode != "SCALE01" || !status.Configured || !status.Initialized {
		t.Fatalf("status after initialization = %+v, want configured and initialized", status)
	}
	// The legacy address fields are no longer written even when the old client
	// still sends serverUrl.
	var siteURL, settingURL string
	if err := db.GORM.Table("wb_site").Select("server_url").Where("site_code=?", "LOCAL01").Scan(&siteURL).Error; err != nil || siteURL != "" {
		t.Fatalf("initialization persisted a site server URL: %q err=%v", siteURL, err)
	}
	if err := db.GORM.Table("wb_agent_setting").Select("server_url").Where("id=1").Scan(&settingURL).Error; err != nil || settingURL != "" {
		t.Fatalf("initialization persisted a global server URL: %q err=%v", settingURL, err)
	}

	input.SiteCode = "OTHER01"
	input.ScaleCode = "OTHER-SCALE"
	input.Name = "不应覆盖的名称"
	body = encodeLocalSiteInput(t, input)
	repeated := serveJSON(router, http.MethodPost, "/api/weighing/site-config/initialize", body, admin)
	if repeated.Code != http.StatusOK || envelopeCode(t, repeated) != http.StatusBadRequest {
		t.Fatalf("repeated initialization status=%d envelope=%d body=%s", repeated.Code, envelopeCode(t, repeated), repeated.Body.String())
	}
	status = weighingData[localsite.Status](t, router, http.MethodGet, "/api/weighing/local-status", "", admin)
	if status.SiteCode != "LOCAL01" || !status.Configured || !status.Initialized {
		t.Fatalf("repeated initialization changed local identity: %+v", status)
	}
	var identity localsite.Identity
	if err := db.GORM.First(&identity, 1).Error; err != nil {
		t.Fatal(err)
	}
	// Databases configured by older builds keep initialized=0; status must still
	// report completion and must not write the legacy flag.
	if identity.SiteCode != "LOCAL01" || identity.ScaleCode != "SCALE01" || identity.NextID != 0 {
		t.Fatalf("local identity = %+v", identity)
	}
	var siteCount, scaleCount, bindingCount int64
	for table, count := range map[string]*int64{"wb_site": &siteCount, "wb_scale": &scaleCount, "wb_agent_binding": &bindingCount} {
		if err := db.GORM.Table(table).Count(count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if siteCount != 1 || scaleCount != 1 || bindingCount != 1 {
		t.Fatalf("repeated initialization duplicated setup: sites=%d scales=%d bindings=%d", siteCount, scaleCount, bindingCount)
	}
	configs := weighingData[[]weighing.SiteConfiguration](t, router, http.MethodGet, "/api/weighing/site-config", "", admin)
	if len(configs) != 1 {
		t.Fatalf("local configuration count=%d, want one", len(configs))
	}
	locked := weighing.SiteConfigurationInput{
		ServerURL: configs[0].ServerURL, Code: configs[0].Code, Name: configs[0].Name, ScaleCode: configs[0].ScaleCode,
		ScaleName: configs[0].ScaleName, AgentID: configs[0].AgentID, Enabled: configs[0].Enabled, Version: configs[0].Version,
	}
	for field, change := range map[string]func(*weighing.SiteConfigurationInput){
		"agent ID": func(in *weighing.SiteConfigurationInput) { in.AgentID = "mutated-agent" },
	} {
		candidate := locked
		change(&candidate)
		response := serveJSON(router, http.MethodPut, "/api/weighing/site-config", encodeLocalSiteInput(t, candidate), admin)
		if response.Code != http.StatusOK || envelopeCode(t, response) != http.StatusBadRequest {
			t.Errorf("changing fixed %s returned status=%d envelope=%d, want business rejection: %s", field, response.Code, envelopeCode(t, response), response.Body.String())
		}
	}
	locked.Name = "一厂改名"
	body = encodeLocalSiteInput(t, locked)
	if response := serveJSON(router, http.MethodPut, "/api/weighing/site-config", body, admin); response.Code != http.StatusOK {
		t.Fatalf("editing local site name returned %d: %s", response.Code, response.Body.String())
	}

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/node/status"},
		{http.MethodPost, "/api/node/exchange"},
		{http.MethodPost, "/api/node/images/missing"},
		{http.MethodPost, "/api/node/images"},
		{http.MethodGet, "/api/weighing/temporary-parties"},
		{http.MethodGet, "/api/weighing/configuration/pending"},
		{http.MethodPost, "/api/weighing/configuration/apply"},
	} {
		if response := serveJSON(router, route.method, route.path, "{}", admin); response.Code != http.StatusNotFound {
			t.Errorf("retired route %s %s returned %d, want 404", route.method, route.path, response.Code)
		}
	}
	createdMaterial := weighingData[weighing.Material](t, router, http.MethodPost, "/api/weighing/materials", `{"name":"编号检查物料"}`, admin)
	var nextID int64
	if err := db.GORM.Table("wb_local_identity").Select("next_id").Where("id=1").Scan(&nextID).Error; err != nil || nextID < 1 {
		t.Fatalf("local number allocator did not advance: next_id=%d err=%v", nextID, err)
	}
	deps2 := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps2.Notification.Close)
	router2, err := app.Build(testAPIConfig(), db, deps2)
	if err != nil {
		t.Fatal(err)
	}
	admin2 := loginAdmin(t, router2)
	status2 := weighingData[localsite.Status](t, router2, http.MethodGet, "/api/weighing/local-status", "", admin2)
	if status2.SiteCode != "LOCAL01" || !status2.Configured || !status2.Initialized {
		t.Fatalf("identity after API restart = %+v", status2)
	}
	var materialPage struct {
		Records []weighing.Material `json:"records"`
	}
	materialPage = weighingData[struct {
		Records []weighing.Material `json:"records"`
	}](t, router2, http.MethodGet, "/api/weighing/materials?page=1&pageSize=10", "", admin2)
	if len(materialPage.Records) == 0 || materialPage.Records[0].ID != createdMaterial.ID || nextID <= materialPage.Records[0].ID {
		t.Fatalf("local business number changed after API restart: next=%d created=%+v materials=%+v", nextID, createdMaterial, materialPage.Records)
	}
	nextMaterial := weighingData[weighing.Material](t, router2, http.MethodPost, "/api/weighing/materials", `{"name":"重启后编号检查"}`, admin2)
	if nextMaterial.ID <= createdMaterial.ID {
		t.Fatalf("local allocator did not resume after API restart: before=%d after=%d", createdMaterial.ID, nextMaterial.ID)
	}
}

func encodeLocalSiteInput(t *testing.T, input any) string {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func envelopeCode(t *testing.T, response *httptest.ResponseRecorder) int {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response envelope: %v; body=%s", err, response.Body.String())
	}
	return payload.Code
}

// ensureLocalSite initializes through the HTTP endpoint with the head-office
// assigned codes (ADR 0033). Callers pass (siteCode, siteName, scaleCode,
// scaleName, agentID); the numeric argument is retained only for signature
// compatibility with older call sites.
func ensureLocalSite(t *testing.T, router http.Handler, token string, _ int64, details ...string) {
	t.Helper()
	status := weighingData[localsite.Status](t, router, http.MethodGet, "/api/weighing/local-status", "", token)
	if status.Configured {
		return
	}
	siteCode, siteName, scaleCode, scaleName := "F001", "本厂", "WB01", "一号地磅"
	if len(details) > 3 {
		siteCode, siteName, scaleCode, scaleName = details[0], details[1], details[2], details[3]
	}
	input := weighing.LocalSiteInput{
		SiteCode: siteCode, ScaleCode: scaleCode,
		Name: siteName, ScaleName: scaleName,
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/weighing/site-config/initialize", string(body), token), http.StatusOK, http.StatusOK, "success")
}

// testLocalSetup initializes through the HTTP endpoint. Since ADR 0029 the
// successful identity transaction completes initialization; operator headcount
// no longer participates.
func testLocalSetup(t *testing.T, _ *platformdatabase.Database, router http.Handler, token string, siteID int64, details ...string) {
	t.Helper()
	ensureLocalSite(t, router, token, siteID, details...)
	status := weighingData[localsite.Status](t, router, http.MethodGet, "/api/weighing/local-status", "", token)
	if !status.Initialized {
		t.Fatalf("local business not enabled after the first identity transaction: %+v", status)
	}
}
