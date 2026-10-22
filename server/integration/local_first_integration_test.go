//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/nodesync"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestSQLiteLocalFirst(t *testing.T)   { checkLocalFirst(t, false) }
func TestPostgresLocalFirst(t *testing.T) { checkLocalFirst(t, true) }
func checkLocalFirst(t *testing.T, postgres bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	type node struct {
		router   http.Handler
		sync     *nodesync.Service
		server   *httptest.Server
		hub      *agentbridge.Hub
		token    string
		imageDir string
		dbPath   string
	}
	makeNode := func(name string, cfg config.NodeConfig, restore ...string) *node {
		dir := t.TempDir()
		path := filepath.Join(dir, name+".db")
		var db *platformdatabase.Database
		if postgres {
			temporary := startPostgres(t)
			runMigrations(t, projectRoot(t), temporary.dsn)
			db = openTemporaryDatabase(t, temporary.dsn)
		} else {
			if len(restore) > 0 {
				if err := platformdatabase.BackupSQLite(ctx, restore[0], path); err != nil {
					t.Fatal(err)
				}
			} else {
				runSQLiteMigrations(t, path)
			}
			db = openSQLiteDatabase(t, path)
		}
		t.Cleanup(func() { db.Close() })
		images := filepath.Join(dir, "images")
		service, e := nodesync.New(db.GORM, cfg, images)
		if e != nil {
			t.Fatal(e)
		}
		deps := sqliteDependencies(t, db, dir)
		deps.Node = service
		repo := weighing.NewRepository(db.GORM)
		configs := []agentbridge.Config{}
		if cfg.Role == "site" {
			configs = append(configs, agentbridge.Config{ID: fmt.Sprintf("lf-agent-%d", cfg.SiteID), ScaleID: cfg.SiteID, Key: "agent-key", ImageDir: images})
		}
		if e = repo.ImportAgentBindings(ctx, configs); e != nil {
			t.Fatal(e)
		}
		hub := agentbridge.NewMultiple(configs)
		deps.Weighing = weighing.NewService(repo, hub)
		apiCfg := testAPIConfig()
		apiCfg.Node = cfg
		apiCfg.Weighing.ImageDir = images
		router, e := app.Build(apiCfg, db, deps)
		if e != nil {
			t.Fatal(e)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/ready" || r.URL.Path == "/health" {
				router.ServeHTTP(w, r)
				return
			}
			dist := filepath.Join(projectRoot(t), "..", "web", "dist")
			name := filepath.Join(dist, filepath.Clean("/"+r.URL.Path))
			if info, err := os.Stat(name); err != nil || info.IsDir() {
				name = filepath.Join(dist, "index.html")
			}
			http.ServeFile(w, r, name)
		}))
		t.Cleanup(server.Close)
		return &node{router, service, server, hub, loginAdmin(t, router), images, path}
	}
	central := makeNode("central", config.NodeConfig{Role: "central", Peers: []config.NodePeer{{SiteID: 1, Key: "sync-one"}, {SiteID: 2, Key: "sync-two"}}})
	// Only the center/site internet hop is interrupted. Local HTTP and Agent
	// websocket links remain available throughout the production scenario.
	var packetMu sync.Mutex
	var originalPacket []byte
	var loseAcknowledgement atomic.Bool
	var delayImages atomic.Bool
	var connected atomic.Bool
	connected.Store(true)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !connected.Load() {
			http.Error(w, "internet unavailable", 503)
			return
		}
		if delayImages.Load() && r.URL.Path == "/api/node/images" {
			http.Error(w, "images delayed", 503)
			return
		}
		if r.URL.Path == "/api/node/exchange" && r.Header.Get("X-Site-ID") == "1" {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read", 500)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			var packet struct {
				Tables map[string][]map[string]any `json:"tables"`
			}
			_ = json.Unmarshal(data, &packet)
			packetMu.Lock()
			if originalPacket == nil && len(packet.Tables["wb_ticket"]) > 0 {
				originalPacket = append([]byte{}, data...)
			}
			packetMu.Unlock()
		}
		if r.URL.Path == "/api/node/exchange" && loseAcknowledgement.CompareAndSwap(true, false) {
			recorded := httptest.NewRecorder()
			central.router.ServeHTTP(recorded, r)
			http.Error(w, "acknowledgement lost after commit", 503)
			return
		}
		central.router.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	encode := func(v any) string {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	material := weighingData[weighing.Material](t, central.router, "POST", "/api/weighing/materials", `{"name":"统一高粱","spec":"一级"}`, central.token)
	party := weighingData[weighing.Party](t, central.router, "POST", "/api/weighing/parties", `{"name":"统一供收方","nature":"enterprise","roles":["supplier","customer"]}`, central.token)
	// Two named authorized people are provisioned centrally before site enablement.
	weighingData[any](t, central.router, "POST", "/api/system/user", `{"username":"shift-two","nickname":"第二班实名人员","status":1}`, central.token)
	users := weighingData[struct {
		Records []struct {
			ID int64 `json:"id"`
		} `json:"records"`
	}](t, central.router, "GET", "/api/system/user/page?username=shift-two", "", central.token)
	if len(users.Records) != 1 {
		t.Fatal("created shift user missing")
	}
	weighingData[any](t, central.router, "PUT", fmt.Sprintf("/api/weighing/users/%d/sites", users.Records[0].ID), `{"grants":[{"siteId":1,"canOperate":1},{"siteId":2,"canOperate":1}]}`, central.token)
	roles := weighingData[[]struct {
		ID       int64  `json:"id"`
		RoleCode string `json:"roleCode"`
	}](t, central.router, "GET", "/api/system/role/options", "", central.token)
	var operatorRole int64
	for _, r := range roles {
		if r.RoleCode == "WEIGHING_OPERATOR" {
			operatorRole = r.ID
		}
	}
	if operatorRole == 0 {
		t.Fatal("operator role missing")
	}
	weighingData[any](t, central.router, "PUT", fmt.Sprintf("/api/system/user/%d/roles", users.Records[0].ID), encode(map[string]any{"roleIds": []int64{operatorRole}}), central.token)
	sites := []*node{makeNode("one", config.NodeConfig{Role: "site", SiteID: 1, CentralURL: proxy.URL, SyncKey: "sync-one"}), makeNode("two", config.NodeConfig{Role: "site", SiteID: 2, CentralURL: proxy.URL, SyncKey: "sync-two"})}
	synchronize := func(n *node) {
		t.Helper()
		if e := n.sync.Sync(ctx); e != nil {
			t.Fatal(e)
		}
		n.token = loginAdmin(t, n.router)
	}
	for i, n := range sites {
		synchronize(n)
		existingToken := n.token
		if err := n.sync.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		assertEnvelopeCode(t, serveJSON(n.router, "GET", "/api/auth/me", "", existingToken), 200, 200, "success")
		status := weighingData[struct {
			Initialized bool `json:"initialized"`
		}](t, n.router, "GET", "/api/node/status", "", n.token)
		if !status.Initialized {
			t.Fatal("site not initialized with two named users")
		}
		caps := weighingData[struct {
			Sites []weighing.Site `json:"sites"`
		}](t, n.router, "GET", "/api/weighing/capabilities", "", n.token)
		if len(caps.Sites) != 1 || caps.Sites[0].ID != int64(i+1) {
			t.Fatalf("site scope: %+v", caps)
		}
		assertEnvelopeCode(t, serveJSON(n.router, "POST", "/api/weighing/materials", `{"name":"forbidden"}`, n.token), 403, 403, "当前部署角色不允许此操作")
	}
	restorePath := filepath.Join(t.TempDir(), "before-business.db")
	if !postgres {
		if err := platformdatabase.BackupSQLite(ctx, sites[0].dbPath, restorePath); err != nil {
			t.Fatal(err)
		}
	}
	assertEnvelopeCode(t, serveJSON(central.router, "POST", "/api/weighing/tasks/first-confirmation", `{}`, central.token), 403, 403, "当前部署角色不允许此操作")
	mocks := []*httptest.Server{}
	stops := []context.CancelFunc{}
	for i, n := range sites {
		id := int64(i + 1)
		mockCtx, stop := context.WithCancel(ctx)
		t.Cleanup(stop)
		stops = append(stops, stop)
		mock := agentmock.New(agentmock.Config{Server: n.server.URL, ID: fmt.Sprintf("lf-agent-%d", id), ScaleID: id, Key: "agent-key"})
		control := httptest.NewServer(mock.Handler())
		t.Cleanup(control.Close)
		mocks = append(mocks, control)
		done := make(chan struct{})
		go func() { defer close(done); mock.Run(mockCtx) }()
		t.Cleanup(func() {
			stop()
			select {
			case <-done:
			case <-time.After(12 * time.Second):
				t.Error("mock failed to stop")
			}
		})
	}
	weight := func(i int, value string) {
		t.Helper()
		for _, body := range []string{fmt.Sprintf(`{"action":"up","weight":%q}`, value), `{"action":"stable"}`} {
			r, e := http.Post(mocks[i].URL+"/api/control", "application/json", bytes.NewBufferString(body))
			if e != nil {
				t.Fatal(e)
			}
			r.Body.Close()
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			s := sites[i].hub.State(int64(i + 1))
			if s.Online && s.Stable && s.Weight.String() == value && len(s.Images) == 2 {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("Agent not ready")
	}
	connected.Store(false)
	if os.Getenv("LOCAL_FIRST_BROWSER") == "1" && !postgres {
		command := exec.CommandContext(ctx, "node", filepath.Join(projectRoot(t), "..", "web", "tests", "local-first-smoke.mjs"), sites[0].server.URL, central.server.URL)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("browser: %v\n%s", err, output)
		} else {
			t.Log(string(output))
		}
	}

	tasks := []weighing.CommandResult{}
	tickets := []weighing.CommandResult{}
	sessions := []weighing.SessionResult{}
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	for i, n := range sites {
		n.token = loginAdmin(t, n.router) // A new local login while the internet is down.
		direction, first := "inbound", "30"
		if i == 1 {
			direction, first = "outbound", "10"
		}
		weight(i, first)
		session := weighingData[weighing.SessionResult](t, n.router, "POST", fmt.Sprintf("/api/weighing/scales/%d/session/acquire", i+1), `{"clientId":"local-browser"}`, n.token)
		sessions = append(sessions, session)
		input := map[string]any{"scaleId": i + 1, "clientId": "local-browser", "sessionToken": session.SessionToken, "business": map[string]any{"plate": "川A23456", "direction": direction, "materialId": material.ID, "partyId": party.ID}}
		sample := weighingData[weighing.Sample](t, n.router, "POST", "/api/weighing/samples", encode(input), n.token)
		input["sampleId"] = sample.ID
		input["checks"] = checks
		input["requestId"] = "offline-first-shared-key"
		task := weighingData[weighing.CommandResult](t, n.router, "POST", "/api/weighing/tasks/first-confirmation", encode(input), n.token)
		tasks = append(tasks, task)
	}
	// Central master changes cannot rewrite the first weighing's snapshot.
	weighingData[weighing.Material](t, central.router, "PUT", fmt.Sprintf("/api/weighing/materials/%d", material.ID), `{"name":"新名称","spec":"新规格","version":1,"enabled":false}`, central.token)
	weighingData[weighing.Party](t, central.router, "PUT", fmt.Sprintf("/api/weighing/parties/%d", party.ID), `{"name":"改名主体","nature":"enterprise","roles":["carrier"],"version":1}`, central.token)
	connected.Store(true)
	for _, n := range sites {
		synchronize(n)
	}
	connected.Store(false)
	for i, n := range sites {
		second := "10"
		if i == 1 {
			second = "30"
		}
		weight(i, second)
		input := map[string]any{"scaleId": i + 1, "clientId": "local-browser", "sessionToken": sessions[i].SessionToken, "taskId": tasks[i].TaskID}
		sample := weighingData[weighing.Sample](t, n.router, "POST", "/api/weighing/samples", encode(input), n.token)
		input["sampleId"] = sample.ID
		input["checks"] = checks
		input["version"] = 1
		input["requestId"] = "offline-second-shared-key"
		result := weighingData[weighing.CommandResult](t, n.router, "POST", fmt.Sprintf("/api/weighing/tasks/%d/second-confirmation", tasks[i].TaskID), encode(input), n.token)
		tickets = append(tickets, result)
		ticket := weighingData[weighing.TicketView](t, n.router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", result.TicketID), "", n.token)
		if ticket.FinalGross-ticket.FinalTare != 20000 || ticket.Business.MaterialName != "统一高粱" {
			t.Fatalf("offline ticket: %+v", ticket)
		}
	}
	if tickets[0].TicketID == tickets[1].TicketID {
		t.Fatal("cross-site identity collision")
	}
	before := weighingData[weighing.Summary](t, central.router, "GET", "/api/weighing/tickets/summary", "", central.token)
	if before.TaskCount != 0 {
		t.Fatal("unsynchronized business leaked")
	}
	connected.Store(true)
	loseAcknowledgement.Store(true)
	if err := sites[0].sync.Sync(ctx); err == nil {
		t.Fatal("lost acknowledgement should leave durable retry state")
	}
	delayImages.Store(true)
	for _, n := range sites {
		if err := n.sync.Sync(ctx); err == nil {
			t.Fatal("image transport should be delayed")
		}
		n.token = loginAdmin(t, n.router)
	}
	partial := weighingData[weighing.Summary](t, central.router, "GET", "/api/weighing/tickets/summary", "", central.token)
	if partial.TaskCount != 2 {
		t.Fatal("image delay excluded valid business")
	}
	incomplete := weighingData[weighing.TicketView](t, central.router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", tickets[0].TicketID), "", central.token)
	if incomplete.ImagesComplete == nil || *incomplete.ImagesComplete {
		t.Fatal("missing image completeness indicator")
	}
	delayImages.Store(false)
	for _, n := range sites {
		synchronize(n)
		synchronize(n)
	} // duplicate transfer never creates another ticket.
	summary := weighingData[weighing.Summary](t, central.router, "GET", "/api/weighing/tickets/summary", "", central.token)
	if summary.TaskCount != 2 || summary.Net != 40000 {
		t.Fatalf("central summary: %+v", summary)
	}
	assertEnvelopeCode(t, serveJSON(sites[0].router, "GET", fmt.Sprintf("/api/weighing/tasks/%d", tasks[1].TaskID), "", sites[0].token), 404, 404, weighing.ErrNotFound.Error())
	for i := range sites {
		operation := weighingData[weighing.Operation](t, central.router, "GET", fmt.Sprintf("/api/weighing/operations/offline-first-shared-key?siteId=%d", i+1), "", central.token)
		if operation.TaskID == nil || *operation.TaskID != tasks[i].TaskID || operation.RequestID != "offline-first-shared-key" {
			t.Fatal("cross-site request key collision")
		}
	}
	// Site submissions remain local until explicitly approved by central staff.
	tmp := weighingData[weighing.Party](t, sites[0].router, "POST", "/api/weighing/temporary-parties", `{"name":"临时供方","nature":"individual","roles":["supplier"]}`, sites[0].token)
	synchronize(sites[0])
	synchronize(sites[1])
	others := weighingData[weighing.Page[weighing.Party]](t, sites[1].router, "GET", "/api/weighing/parties?keyword=临时供方", "", sites[1].token)
	if others.Total != 0 {
		t.Fatal("unconfirmed party leaked to another site")
	}
	review := fmt.Sprintf("/api/weighing/temporary-parties/%d/review", tmp.ID)
	weighingData[any](t, central.router, "POST", review, `{"revision":1,"action":"return","reason":"补全姓名"}`, central.token)
	synchronize(sites[0])
	weighingData[weighing.Party](t, sites[0].router, "PUT", fmt.Sprintf("/api/weighing/temporary-parties/%d", tmp.ID), `{"name":"临时供方补全","nature":"individual","roles":["supplier"],"version":1}`, sites[0].token)
	synchronize(sites[0])
	weighingData[any](t, central.router, "POST", review, `{"revision":2,"action":"approve"}`, central.token)
	synchronize(sites[0])
	synchronize(sites[1])
	shared := weighingData[weighing.Page[weighing.Party]](t, sites[1].router, "GET", "/api/weighing/parties?keyword=临时供方补全", "", sites[1].token)
	if shared.Total != 1 {
		t.Fatal("approved party was not distributed")
	}

	alias := weighingData[weighing.Party](t, sites[1].router, "POST", "/api/weighing/temporary-parties", `{"name":"临时供方补全","nature":"individual","roles":["supplier"]}`, sites[1].token)
	synchronize(sites[1])
	sameName := weighingData[[]weighing.TemporaryParty](t, central.router, "GET", "/api/weighing/temporary-parties", "", central.token)
	pendingAlias := false
	for _, row := range sameName {
		if row.PartyID == alias.ID && row.State == "pending" {
			pendingAlias = true
		}
	}
	if !pendingAlias {
		t.Fatal("same-name source was automatically merged")
	}
	weighingData[any](t, central.router, "POST", fmt.Sprintf("/api/weighing/temporary-parties/%d/review", alias.ID), encode(map[string]any{"revision": 1, "action": "link", "formalPartyId": shared.Records[0].ID}), central.token)
	synchronize(sites[1])
	associations := weighingData[[]weighing.TemporaryParty](t, sites[1].router, "GET", "/api/weighing/temporary-parties", "", sites[1].token)
	linked := false
	for _, row := range associations {
		if row.PartyID == alias.ID && row.State == "linked" && row.FormalPartyID != nil && *row.FormalPartyID == shared.Records[0].ID {
			linked = true
		}
	}
	if !linked {
		t.Fatal("explicit formal-party association not delivered")
	}
	complete := weighingData[weighing.TicketView](t, central.router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", tickets[0].TicketID), "", central.token)
	if complete.ImagesComplete == nil || !*complete.ImagesComplete {
		t.Fatal("images not completed after reconnect")
	}
	// Later changes retain the same business identity and remove voided totals.
	printPath := fmt.Sprintf("/api/weighing/tickets/%d/print-attempts", tickets[0].TicketID)
	weighingData[weighing.CommandResult](t, sites[0].router, "POST", printPath, `{"requestId":"lf-local-print","intent":"initial"}`, sites[0].token)
	synchronize(sites[0])
	synchronize(sites[0])
	localPrints := weighingData[[]weighing.PrintAttempt](t, sites[0].router, "GET", printPath, "", sites[0].token)
	centerPrints := weighingData[[]weighing.PrintAttempt](t, central.router, "GET", printPath, "", central.token)
	if len(localPrints) != 1 || len(centerPrints) != 1 {
		t.Fatal("print attempts duplicated or missing")
	}
	weighingData[weighing.CommandResult](t, sites[0].router, "POST", fmt.Sprintf("/api/weighing/tickets/%d/void", tickets[0].TicketID), `{"requestId":"local-first-void-request","taskVersion":2,"reason":"人工更正"}`, sites[0].token)
	synchronize(sites[0])
	afterVoid := weighingData[weighing.Summary](t, central.router, "GET", "/api/weighing/tickets/summary", "", central.token)
	if afterVoid.TaskCount != 1 {
		t.Fatal("voided ticket still counted")
	}
	if !postgres {
		restored := makeNode("restored", config.NodeConfig{Role: "site", SiteID: 1, CentralURL: proxy.URL, SyncKey: "sync-one"}, restorePath)
		restoredCtx, stopRestored := context.WithCancel(ctx)
		defer stopRestored()
		mock := agentmock.New(agentmock.Config{Server: restored.server.URL, ID: "lf-agent-1", ScaleID: 1, Key: "agent-key"})
		control := httptest.NewServer(mock.Handler())
		defer control.Close()
		done := make(chan struct{})
		go func() { defer close(done); mock.Run(restoredCtx) }()
		defer func() {
			stopRestored()
			select {
			case <-done:
			case <-time.After(12 * time.Second):
				t.Error("restored mock failed to stop")
			}
		}()
		session := weighingData[weighing.SessionResult](t, restored.router, "POST", "/api/weighing/scales/1/session/acquire", `{"clientId":"restored-browser"}`, restored.token)
		var restoredTask weighing.CommandResult
		var restoredTicket weighing.CommandResult
		for stage, value := range []string{"30", "10"} {
			for _, body := range []string{fmt.Sprintf(`{"action":"up","weight":%q}`, value), `{"action":"stable"}`} {
				r, err := http.Post(control.URL+"/api/control", "application/json", bytes.NewBufferString(body))
				if err != nil {
					t.Fatal(err)
				}
				r.Body.Close()
			}
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				state := restored.hub.State(1)
				if state.Online && state.Stable && state.Weight.String() == value && len(state.Images) == 2 {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			input := map[string]any{"scaleId": 1, "clientId": "restored-browser", "sessionToken": session.SessionToken}
			if stage == 0 {
				input["business"] = map[string]any{"plate": "川B12345", "direction": "inbound", "materialId": material.ID, "partyId": party.ID}
			} else {
				input["taskId"] = restoredTask.TaskID
			}
			sample := weighingData[weighing.Sample](t, restored.router, "POST", "/api/weighing/samples", encode(input), restored.token)
			input["sampleId"] = sample.ID
			input["checks"] = checks
			input["requestId"] = fmt.Sprintf("restored-confirmation-%d", stage)
			if stage == 0 {
				restoredTask = weighingData[weighing.CommandResult](t, restored.router, "POST", "/api/weighing/tasks/first-confirmation", encode(input), restored.token)
			} else {
				input["version"] = 1
				restoredTicket = weighingData[weighing.CommandResult](t, restored.router, "POST", fmt.Sprintf("/api/weighing/tasks/%d/second-confirmation", restoredTask.TaskID), encode(input), restored.token)
			}
		}
		restoredView := weighingData[weighing.TicketView](t, restored.router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", restoredTicket.TicketID), "", restored.token)
		if restoredView.TicketNo == complete.TicketNo || restoredView.ID == complete.ID {
			t.Fatal("restoring an older backup reused a ticket identity or public number")
		}
	}

	// Replay an older business snapshot inside a fresh transport envelope, as
	// could happen after restoring a backup. It must not resurrect a voided bill.
	packetMu.Lock()
	saved := append([]byte{}, originalPacket...)
	packetMu.Unlock()
	var stale map[string]json.RawMessage
	if json.Unmarshal(saved, &stale) != nil {
		t.Fatal("missing retained business snapshot")
	}
	stale["sequence"] = json.RawMessage(fmt.Sprintf("%d", time.Now().UnixMicro()+1))
	req, err := http.NewRequestWithContext(ctx, "POST", central.server.URL+"/api/node/exchange", bytes.NewBufferString(encode(stale)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sync-one")
	req.Header.Set("X-Site-ID", "1")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal("older restored history was accepted as a new business version")
	}
	retained := weighingData[weighing.Summary](t, central.router, "GET", "/api/weighing/tickets/summary", "", central.token)
	if retained.TaskCount != 1 {
		t.Fatal("old backup resurrected voided business")
	}
	// Verify the process worker, including automatic reconnection, against HTTP
	// observations within the one-minute visibility budget.
	automaticCtx, stopAutomatic := context.WithCancel(ctx)
	automaticDone := make(chan struct{})
	connected.Store(false)
	go func() { defer close(automaticDone); sites[1].sync.Run(automaticCtx) }()
	weighingData[weighing.Material](t, central.router, "PUT", fmt.Sprintf("/api/weighing/materials/%d", material.ID), `{"name":"自动下发更新","spec":"新规格","version":2,"enabled":false}`, central.token)
	connected.Store(true)
	deadlineAuto := time.Now().Add(45 * time.Second)
	visible := false
	for time.Now().Before(deadlineAuto) {
		page := weighingData[weighing.Page[weighing.Material]](t, sites[1].router, "GET", "/api/weighing/materials", "", sites[1].token)
		for _, m := range page.Records {
			if m.ID == material.ID && m.Name == "自动下发更新" {
				visible = true
			}
		}
		if visible {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stopAutomatic()
	select {
	case <-automaticDone:
	case <-time.After(35 * time.Second):
		t.Fatal("automatic worker failed to stop")
	}
	if !visible {
		t.Fatal("automatic reconnect did not distribute master update within one minute")
	}

	// A central disable request remains an intention until explicitly applied
	// locally after outstanding production/printing and the Agent are stopped.
	configs := weighingData[[]weighing.SiteConfiguration](t, central.router, "GET", "/api/weighing/site-config", "", central.token)
	var original weighing.SiteConfiguration
	for _, v := range configs {
		if v.ID == 1 {
			original = v
		}
	}
	proposal := weighing.SiteConfigurationInput{Code: original.Code, Name: original.Name, ScaleCode: original.ScaleCode, ScaleName: original.ScaleName, AgentID: "lf-agent-1", Enabled: 0, Version: original.Version, ServerURL: sites[0].server.URL}
	weighingData[weighing.SiteConfiguration](t, central.router, "PUT", "/api/weighing/site-config/1", encode(proposal), central.token)
	stillEnabled := weighingData[[]weighing.SiteConfiguration](t, central.router, "GET", "/api/weighing/site-config", "", central.token)
	if stillEnabled[0].Enabled != 1 {
		t.Fatal("central disable intention changed applied state")
	}
	synchronize(sites[0])
	pending := weighingData[struct {
		Revision int64 `json:"revision"`
	}](t, sites[0].router, "GET", "/api/weighing/configuration/pending", "", sites[0].token)
	deniedApply := serveJSON(sites[0].router, "POST", "/api/weighing/configuration/apply", encode(map[string]any{"revision": pending.Revision}), sites[0].token)
	var application struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(deniedApply.Body.Bytes(), &application) != nil || application.Code != 400 {
		t.Fatal("live Agent did not block disable")
	}
	stops[0]()
	deadline := time.Now().Add(10 * time.Second)
	for sites[0].hub.Connected(1) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	attempts := weighingData[[]weighing.PrintAttempt](t, sites[0].router, "GET", printPath, "", sites[0].token)
	if attempts[0].ClosedAt == nil {
		weighingData[weighing.CommandResult](t, sites[0].router, "POST", fmt.Sprintf("/api/weighing/print-attempts/%d/result", attempts[0].ID), `{"requestId":"local-print-resolved","status":"printed","reason":"现场核实已出纸"}`, sites[0].token)
	}
	weighingData[any](t, sites[0].router, "POST", "/api/weighing/configuration/apply", encode(map[string]any{"revision": pending.Revision}), sites[0].token)
	synchronize(sites[0])
	applied := weighingData[[]weighing.SiteConfiguration](t, central.router, "GET", "/api/weighing/site-config", "", central.token)
	if applied[0].Enabled != 0 || applied[0].PendingRevision != applied[0].AppliedRevision {
		t.Fatal("local disable acknowledgement was not reflected centrally")
	}

	// A password update revokes prior sessions on receipt; the old password no
	// longer authenticates locally, while the replacement works without internet.
	shift := weighingData[struct {
		TokenValue string `json:"tokenValue"`
	}](t, sites[0].router, "POST", "/api/auth/login", `{"username":"shift-two","password":"admin123"}`, "")
	oldToken := shift.TokenValue
	weighingData[any](t, central.router, "PATCH", fmt.Sprintf("/api/system/user/%d/status", users.Records[0].ID), `{"status":0}`, central.token)
	if err := sites[0].sync.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	denied := serveJSON(sites[0].router, "GET", "/api/auth/me", "", oldToken)
	if denied.Code != 401 {
		t.Fatalf("revoked local session remained usable: %s", denied.Body.String())
	}
}
