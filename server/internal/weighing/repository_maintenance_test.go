package weighing

import (
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func TestMatchesMaintenanceIdentityAllowsDisplayNameChangesOnly(t *testing.T) {
	identity := maintenanceIdentity{SiteID: 7, ScaleID: 12, SiteCode: "SITE-7", ScaleCode: "SCALE-12", ServerURL: "https://site.example.test", AgentID: "local-agent", AgentKey: "site-agent-key"}
	candidate := agentprinter.Config{
		SiteID: identity.SiteID, ScaleID: identity.ScaleID, SiteCode: identity.SiteCode, ScaleCode: identity.ScaleCode,
		ServerURL: identity.ServerURL, AgentID: identity.AgentID, AgentKey: identity.AgentKey,
		SiteName: "本地修改后的名称", ScaleName: "本地修改后的磅名",
	}
	if !matchesMaintenanceIdentity(candidate, identity) {
		t.Fatal("display name changes must not invalidate local identity")
	}
	mutations := []struct {
		name   string
		change func(*agentprinter.Config)
	}{
		{"site id", func(c *agentprinter.Config) { c.SiteID++ }},
		{"scale id", func(c *agentprinter.Config) { c.ScaleID++ }},
		{"agent id", func(c *agentprinter.Config) { c.AgentID = "other-agent" }},
		{"site code", func(c *agentprinter.Config) { c.SiteCode = "OTHER-SITE" }},
		{"scale code", func(c *agentprinter.Config) { c.ScaleCode = "OTHER-SCALE" }},
		{"server url", func(c *agentprinter.Config) { c.ServerURL = "https://other.example.test" }},
		{"agent key", func(c *agentprinter.Config) { c.AgentKey = "other-key" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := candidate
			mutation.change(&changed)
			if matchesMaintenanceIdentity(changed, identity) {
				t.Fatal("identity change was accepted")
			}
		})
	}
}
