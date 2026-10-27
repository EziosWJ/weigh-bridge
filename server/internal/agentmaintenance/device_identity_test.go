package agentmaintenance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
)

// A keyless Agent stores only the Server address (ADR 0030). Importing the
// Server-provided configuration must not be rejected just because it carries
// site/scale codes this host never knew; a different Server address is still a
// real identity change.
func TestBusinessImportAcceptsServerAssignedIdentityForKeylessAgent(t *testing.T) {
	dir := t.TempDir()
	initial := Defaults()
	initial.ServerURL = "http://127.0.0.1:18900"
	service, err := OpenDevice(dir, filepath.Join(dir, "agent.env"), initial, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	importConnection := func(serverURL string) *httptest.ResponseRecorder {
		t.Helper()
		connection := agentconfig.Connection{
			ServerURL: serverURL, SiteCode: "F001", SiteName: "一厂",
			ScaleCode: "WB01", ScaleName: "一号地磅", AgentID: "agent-WB01",
		}
		body, err := json.Marshal(map[string]any{"connection": connection})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/config/import", strings.NewReader(string(body)))
		request = request.WithContext(context.WithValue(request.Context(), businessRequestKey{}, true))
		response := httptest.NewRecorder()
		service.importConnection(response, request)
		return response
	}
	if response := importConnection("http://127.0.0.1:18900"); response.Code != http.StatusOK {
		t.Fatalf("keyless import rejected: %d %s", response.Code, response.Body)
	}
	if response := importConnection("http://127.0.0.1:18901"); response.Code == http.StatusOK {
		t.Fatalf("changed Server address accepted: %s", response.Body)
	}
}
