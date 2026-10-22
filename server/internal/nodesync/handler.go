package nodesync

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/auth"
	ph "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// Boundary enforces deployment authority independently of menu/action grants.
func (s *Service) Boundary(c *gin.Context) {
	path := c.Request.URL.Path
	write := c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS"
	deny := false
	if s.cfg.Role == "central" {
		if strings.HasPrefix(path, "/api/agent/") || (strings.HasPrefix(path, "/api/weighing/scales/") && strings.HasSuffix(path, "/device")) {
			deny = true
		}
		if write && strings.HasPrefix(path, "/api/weighing/") {
			name := strings.TrimPrefix(path, "/api/weighing/")
			allowed := false
			for _, prefix := range []string{"material-categories", "materials", "parties", "site-config", "agent-settings", "users/", "temporary-parties/"} {
				if strings.HasPrefix(name, prefix) {
					allowed = true
				}
			}
			deny = !allowed
		}
	}
	if s.cfg.Role == "site" {
		if write && (strings.HasPrefix(path, "/api/system/") || strings.HasPrefix(path, "/api/auth/password")) {
			deny = true
		}
		if write && strings.HasPrefix(path, "/api/weighing/") {
			name := strings.TrimPrefix(path, "/api/weighing/")
			for _, prefix := range []string{"material", "parties", "site-config", "agent-settings", "users/"} {
				if strings.HasPrefix(name, prefix) {
					deny = true
				}
			}
			if !strings.HasPrefix(name, "temporary-parties") && !strings.HasPrefix(name, "configuration/apply") {
				id, e := loadIdentity(c.Request.Context(), s.db)
				if e != nil || id.Initialized != 1 {
					deny = true
				}
			}
		}
	}
	if s.cfg.Role == "standalone" && write && strings.HasPrefix(path, "/api/weighing/") {
		name := strings.TrimPrefix(path, "/api/weighing/")
		// Temporary-party review is a retired central workflow. Existing rows are
		// retained for history; new local parties use the ordinary master API.
		if strings.HasPrefix(name, "temporary-parties") {
			deny = true
		} else {
			management := false
			for _, prefix := range []string{"material-categories", "materials", "parties", "site-config", "agent-settings", "users/"} {
				if strings.HasPrefix(name, prefix) {
					management = true
					break
				}
			}
			if !management {
				ready, err := s.RefreshInitialized(c.Request.Context())
				if err != nil || !ready {
					deny = true
				}
			}
		}
	}
	if deny {
		ph.AbortError(c, 403, 403, "当前部署角色不允许此操作", nil)
		return
	}
	c.Next()
}
func (s *Service) Register(router *gin.Engine, a *auth.Service) {
	if s.cfg.Role == "" {
		return
	}
	node := router.Group("/api/node")
	node.Use(auth.BearerMiddleware(a))
	node.GET("/status", func(c *gin.Context) {
		p, _ := auth.PrincipalFromContext(c.Request.Context())
		rows := []State{}
		if s.cfg.Role != "standalone" {
			q := s.db.WithContext(c.Request.Context()).Where("site_id IN (SELECT site_id FROM wb_user_site WHERE user_id=?)", p.UserID)
			if s.cfg.Role == "site" {
				q = q.Where("site_id=?", s.cfg.SiteID)
			}
			if err := q.Find(&rows).Error; err != nil {
				ph.WriteError(c, 500, 500, "同步状态读取失败", nil)
				return
			}
		}
		type view struct {
			State
			Connected bool `json:"connected"`
			Pending   int  `json:"pending"`
		}
		views := []view{}
		for _, st := range rows {
			missing, _ := s.missingImages(c.Request.Context(), st.SiteID)
			if s.cfg.Role == "central" {
				st.PendingImages = len(missing)
			}
			last, _ := time.Parse(time.RFC3339Nano, st.LastSuccess)
			pending := 0
			if s.cfg.Role == "site" {
				tables, e := s.snapshot(s.db.WithContext(c.Request.Context()), s.cfg.SiteID, false)
				if e != nil {
					ph.WriteError(c, 500, 500, "同步状态读取失败", nil)
					return
				}
				_, count, e := delta(s.db.WithContext(c.Request.Context()), s.cfg.SiteID, tables)
				if e != nil {
					ph.WriteError(c, 500, 500, "同步状态读取失败", nil)
					return
				}
				pending = count
				if st.Pending != "" && pending == 0 {
					pending = 1
				}
			}
			views = append(views, view{st, time.Since(last) < time.Minute && st.LastError == "", pending})
		}
		initialized := false
		if s.cfg.Role == "standalone" {
			var err error
			initialized, err = s.RefreshInitialized(c.Request.Context())
			if err != nil {
				ph.WriteError(c, 500, 500, "本地初始化状态读取失败", nil)
				return
			}
		} else {
			id, _ := loadIdentity(c.Request.Context(), s.db)
			initialized = id.Initialized == 1
		}
		ph.OK(c, gin.H{"role": s.cfg.Role, "siteId": s.cfg.SiteID, "initialized": initialized, "sites": views})
	})
	if s.cfg.Role != "central" {
		return
	}
	incoming := router.Group("/api/node")
	incoming.Use(func(c *gin.Context) {
		site, _ := strconv.ParseInt(c.GetHeader("X-Site-ID"), 10, 64)
		key := ""
		for _, p := range s.cfg.Peers {
			if p.SiteID == site {
				key = p.Key
			}
		}
		if key == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+key)) != 1 {
			ph.AbortError(c, 401, 401, "同步身份无效", nil)
			return
		}
		c.Set("nodeSite", site)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<20)
		c.Next()
	})
	incoming.POST("/exchange", func(c *gin.Context) {
		var p Packet
		d := json.NewDecoder(c.Request.Body)
		d.UseNumber()
		if d.Decode(&p) != nil || p.Sequence < 0 {
			ph.WriteError(c, 400, 400, "无效同步数据", nil)
			return
		}
		out, e := s.receive(c.Request.Context(), c.GetInt64("nodeSite"), p)
		if e != nil {
			slog.Warn("node sync apply failed", "site", c.GetInt64("nodeSite"), "error", e)
			ph.WriteError(c, 400, 400, "同步数据应用失败", nil)
			return
		}
		ph.OK(c, out)
	})
	incoming.POST("/images/missing", func(c *gin.Context) {
		ids, e := s.missingImages(c.Request.Context(), c.GetInt64("nodeSite"))
		if e != nil {
			ph.WriteError(c, 500, 500, "图片清单读取失败", nil)
			return
		}
		ph.OK(c, ids)
	})
	incoming.POST("/images", func(c *gin.Context) {
		var in struct {
			ID   string `json:"id"`
			Data []byte `json:"data"`
		}
		if c.ShouldBindJSON(&in) != nil || !imageID.MatchString(in.ID) || len(in.Data) > 10<<20 {
			ph.WriteError(c, 400, 400, "无效图片", nil)
			return
		}
		manifest, e := s.imageEvidence(c.Request.Context(), c.GetInt64("nodeSite"))
		allowed := false
		digest := sha256.Sum256(in.Data)
		if expected, ok := manifest[in.ID]; ok && expected == hex.EncodeToString(digest[:]) {
			allowed = true
		}
		info, _, imageErr := image.DecodeConfig(bytes.NewReader(in.Data))
		if e != nil || !allowed || imageErr != nil || int64(info.Width)*int64(info.Height) > 40000000 {
			ph.WriteError(c, 400, 400, "图片不属于已同步称重依据", nil)
			return
		}
		if e = os.MkdirAll(s.images, 0700); e == nil {
			var f *os.File
			f, e = os.CreateTemp(s.images, ".incoming-")
			if e == nil {
				name := f.Name()
				defer os.Remove(name)
				if _, e = f.Write(in.Data); e == nil {
					e = f.Sync()
				}
				closeErr := f.Close()
				if e == nil {
					e = closeErr
				}
				if e == nil {
					e = os.Rename(name, filepath.Join(s.images, in.ID))
				}
			}
		}
		if e != nil {
			ph.WriteError(c, 500, 500, "图片保存失败", nil)
			return
		}
		ph.OK(c, nil)
	})
}
