package nodesync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StandaloneInit struct {
	SiteCode      string
	SiteName      string
	ScaleCode     string
	ScaleName     string
	AgentID       string
	AdminPassword string
	AgentEnvPath  string
}

var localCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,40}$`)
var localAgentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`)

// InitializeStandalone binds one preassigned local site identity, prepares its
// first local administrator and Agent credentials, and writes a private agent.env.
// It never changes an existing node identity or starts any central connection.
func InitializeStandalone(ctx context.Context, db *gorm.DB, node config.NodeConfig, in StandaloneInit, httpAddress string) (int64, error) {
	if node.Role != "standalone" || node.SiteID < 1 || node.SiteID > 255 {
		return 0, errors.New("standalone initialization requires node.role=standalone and site_id 1..255")
	}
	in.SiteCode = strings.TrimSpace(in.SiteCode)
	in.SiteName = strings.TrimSpace(in.SiteName)
	in.ScaleCode = strings.TrimSpace(in.ScaleCode)
	in.ScaleName = strings.TrimSpace(in.ScaleName)
	in.AgentID = strings.TrimSpace(in.AgentID)
	if !localCodePattern.MatchString(in.SiteCode) || !localCodePattern.MatchString(in.ScaleCode) || !localAgentPattern.MatchString(in.AgentID) || in.SiteName == "" || len([]rune(in.SiteName)) > 100 || in.ScaleName == "" || len([]rune(in.ScaleName)) > 100 {
		return 0, errors.New("site/scale code or name is invalid")
	}
	if len(in.AdminPassword) < 12 || len(in.AdminPassword) > 72 || strings.TrimSpace(in.AdminPassword) != in.AdminPassword {
		return 0, errors.New("WEIGHBRIDGE_ADMIN_PASSWORD must contain 12..72 non-padded characters")
	}
	if strings.TrimSpace(in.AgentEnvPath) == "" {
		return 0, errors.New("agent env path is required")
	}
	if err := os.MkdirAll(filepath.Dir(in.AgentEnvPath), 0700); err != nil {
		return 0, err
	}
	envFile, err := os.OpenFile(in.AgentEnvPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return 0, errors.New("agent.env already exists; initialization will not overwrite it")
		}
		return 0, err
	}
	committed := false
	defer func() {
		_ = envFile.Close()
		if !committed {
			_ = os.Remove(in.AgentEnvPath)
		}
	}()
	serverURL, err := localServerURL(httpAddress)
	if err != nil {
		return 0, err
	}
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return 0, err
	}
	agentKey := hex.EncodeToString(keyBytes)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(in.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	var legacyTemporary int64
	var scaleID int64
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity Identity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&identity, 1).Error; err == nil {
			return errors.New("database already has a node identity; standalone-init will not reset credentials or change site identity")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		for _, table := range businessTables {
			var count int64
			if err := tx.Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("standalone initialization requires an empty business database; %s contains %d rows", table, count)
			}
		}
		var localUsers int64
		if err := tx.Table("sys_user").Where("id<>1 AND deleted=0").Count(&localUsers).Error; err != nil {
			return err
		}
		if localUsers != 0 {
			return errors.New("standalone initialization requires only the seeded admin account; preserve existing local accounts and initialize them through the management API")
		}
		if err := tx.Create(&Identity{ID: 1, Role: "standalone", SiteID: node.SiteID}).Error; err != nil {
			return err
		}
		var siteCount int64
		if err := tx.Table("wb_site").Where("id=?", node.SiteID).Count(&siteCount).Error; err != nil {
			return err
		}
		if siteCount == 0 {
			if err := tx.Exec("INSERT INTO wb_site (id,code,name,enabled,version,server_url) VALUES (?,?,?,1,1,?)", node.SiteID, in.SiteCode, in.SiteName, serverURL).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Exec("UPDATE wb_site SET code=?,name=?,enabled=1,server_url=?,version=version+1 WHERE id=?", in.SiteCode, in.SiteName, serverURL, node.SiteID).Error; err != nil {
				return err
			}
		}
		var scales []struct {
			ID int64
		}
		if err := tx.Table("wb_scale").Select("id").Where("site_id=?", node.SiteID).Order("id").Find(&scales).Error; err != nil {
			return err
		}
		if len(scales) > 1 {
			return errors.New("standalone initialization expects one scale assigned to this site")
		}
		if len(scales) == 1 {
			scaleID = scales[0].ID
			if err := tx.Exec("UPDATE wb_scale SET code=?,name=?,enabled=1,site_id=? WHERE id=?", in.ScaleCode, in.ScaleName, node.SiteID, scaleID).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Raw("SELECT COALESCE(MAX(id),0)+1 FROM wb_scale").Scan(&scaleID).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO wb_scale (id,code,name,enabled,site_id) VALUES (?,?,?,1,?)", scaleID, in.ScaleCode, in.ScaleName, node.SiteID).Error; err != nil {
				return err
			}
			if err := tx.Exec("SELECT setval(pg_get_serial_sequence('wb_scale','id'), GREATEST((SELECT MAX(id) FROM wb_scale),1))").Error; err != nil {
				return err
			}
		}
		var conflict int64
		if err := tx.Table("wb_agent_binding").Where("scale_id=?", scaleID).Count(&conflict).Error; err != nil {
			return err
		}
		if conflict != 0 {
			return errors.New("scale already has an Agent binding; initialization will not replace an existing key")
		}
		if err := tx.Exec("INSERT INTO wb_agent_binding (scale_id,agent_id,agent_key) VALUES (?,?,?) ON CONFLICT (scale_id) DO UPDATE SET agent_id=EXCLUDED.agent_id,agent_key=EXCLUDED.agent_key", scaleID, in.AgentID, agentKey).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE wb_agent_setting SET server_url=? WHERE id=1", serverURL).Error; err != nil {
			return err
		}
		passwordResult := tx.Exec("UPDATE sys_user SET password=? WHERE id=1 AND username='admin' AND deleted=0", string(passwordHash))
		if passwordResult.Error != nil {
			return passwordResult.Error
		}
		if passwordResult.RowsAffected != 1 {
			return errors.New("seed local admin account is missing")
		}
		if err := tx.Table("auth_session").Where("user_id=1 AND revoked_at IS NULL").Update("revoked_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO wb_user_site (user_id,site_id,can_operate) SELECT id,?,1 FROM sys_user WHERE id=1 AND deleted=0 ON CONFLICT (user_id,site_id) DO UPDATE SET can_operate=1", node.SiteID).Error; err != nil {
			return err
		}
		if err := tx.Table("wb_temporary_party").Where("site_id=?", node.SiteID).Count(&legacyTemporary).Error; err != nil {
			return err
		}
		return writeAgentEnvironmentFile(envFile, node.SiteID, in, scaleID, serverURL, agentKey)
	})
	if err != nil {
		return 0, err
	}
	committed = true
	return legacyTemporary, nil
}

func localServerURL(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return "", errors.New("http.address must be host:port for standalone initialization")
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

func writeAgentEnvironmentFile(f *os.File, siteID int64, in StandaloneInit, scaleID int64, serverURL, agentKey string) error {
	if scaleID < 1 {
		return errors.New("initialized scale identity could not be read")
	}
	defaults := agentprinter.Defaults()
	values := []string{
		"WEIGHBRIDGE_SITE_ID=" + strconv.FormatInt(siteID, 10),
		"WEIGHBRIDGE_SITE_CODE=" + strconv.Quote(in.SiteCode),
		"WEIGHBRIDGE_SITE_NAME=" + strconv.Quote(in.SiteName),
		"WEIGHBRIDGE_SCALE_CODE=" + strconv.Quote(in.ScaleCode),
		"WEIGHBRIDGE_SCALE_NAME=" + strconv.Quote(in.ScaleName),
		"WEIGHBRIDGE_SERVER_URL=" + serverURL,
		"WEIGHBRIDGE_AGENT_ID=" + in.AgentID,
		"WEIGHBRIDGE_SCALE_ID=" + strconv.FormatInt(scaleID, 10),
		"WEIGHBRIDGE_PRINT_QUEUE=" + defaults.Queue,
		"APP_WEIGHING__AGENT_KEY=" + agentKey,
		"WEIGHBRIDGE_PRINT_WIDTH_MM=" + strconv.Itoa(defaults.PrintWidthMM),
		"WEIGHBRIDGE_FORM_LENGTH_MM=" + strconv.FormatFloat(defaults.FormLengthMM, 'f', -1, 64),
		"WEIGHBRIDGE_PRINT_SCALE=" + strconv.FormatFloat(defaults.PrintScale, 'f', -1, 64),
		"WEIGHBRIDGE_SERIAL_ENABLED=false",
	}
	if _, err := f.WriteString(strings.Join(values, "\n") + "\n"); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}
