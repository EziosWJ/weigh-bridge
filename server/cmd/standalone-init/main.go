// Command standalone-init prepares one migrated PostgreSQL database for local use.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/nodesync"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
)

func main() {
	var configDir string
	var siteCode, siteName, scaleCode, scaleName, agentID, agentEnv string
	flag.StringVar(&configDir, "config-dir", config.DefaultDir, "directory containing the API configuration")
	flag.StringVar(&siteCode, "site-code", "", "stable local site code")
	flag.StringVar(&siteName, "site-name", "", "local site name")
	flag.StringVar(&scaleCode, "scale-code", "", "local scale code")
	flag.StringVar(&scaleName, "scale-name", "", "local scale name")
	flag.StringVar(&agentID, "agent-id", "", "stable local Agent ID")
	flag.StringVar(&agentEnv, "agent-env-path", "", "new agent.env path; existing files are never overwritten")
	flag.Parse()

	if siteCode == "" || siteName == "" || scaleCode == "" || scaleName == "" || agentEnv == "" {
		flag.Usage()
		log.Fatal("site/scale identity and --agent-env-path are required")
	}
	if agentID == "" {
		agentID = "scale-agent-" + siteCode
	}
	password := os.Getenv("WEIGHBRIDGE_ADMIN_PASSWORD")
	if password == "" {
		log.Fatal("set WEIGHBRIDGE_ADMIN_PASSWORD to a local admin password of at least 12 characters")
	}
	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Node.Role != "standalone" {
		log.Fatal("configuration must set node.role=standalone and node.site_id to the stable local site ID")
	}
	database, err := platformdatabase.Open(context.Background(), cfg.Database)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	legacyTemporary, err := nodesync.InitializeStandalone(context.Background(), database.GORM, cfg.Node, nodesync.StandaloneInit{
		SiteCode: siteCode, SiteName: siteName, ScaleCode: scaleCode, ScaleName: scaleName,
		AgentID: agentID, AdminPassword: password, AgentEnvPath: agentEnv,
	}, cfg.HTTP.Address)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("standalone site %s (%d) initialized; agent.env created at %s\n", siteCode, cfg.Node.SiteID, agentEnv)
	if legacyTemporary > 0 {
		fmt.Printf("retained %d legacy temporary parties in their historical status; none were promoted\n", legacyTemporary)
	}
}
