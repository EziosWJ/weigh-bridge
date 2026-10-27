package weighing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerSiteRoutes(router gin.IRouter, h *Handler) {
	router.GET("/sites", rbac.RequirePermission(h.permissions, "weighing:read"), func(c *gin.Context) {
		p, _ := auth.PrincipalFromContext(c.Request.Context())
		v, e := h.service.Sites(c, p.UserID)
		writeResult(c, v, e)
	})
	manage := rbac.RequirePermission(h.permissions, "system:user")
	router.GET("/site-options", manage, func(c *gin.Context) { v, e := h.service.SiteOptions(c); writeResult(c, v, e) })
	router.GET("/users/:userId/sites", manage, func(c *gin.Context) {
		id, e := strconv.ParseInt(c.Param("userId"), 10, 64)
		if e != nil || id < 1 {
			writeResult(c, nil, ErrInvalid)
			return
		}
		v, e := h.service.SiteGrants(c, id)
		writeResult(c, v, e)
	})
	router.PUT("/users/:userId/sites", manage, func(c *gin.Context) {
		id, e := strconv.ParseInt(c.Param("userId"), 10, 64)
		var in struct {
			Grants []SiteGrant `json:"grants"`
		}
		if e != nil || id < 1 || c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		writeResult(c, nil, h.service.SaveSiteGrants(c, id, in.Grants, requestEvent(c)))
	})
	router.POST("/scales/:code/agent/kick", rbac.RequirePermission(h.permissions, "weighing:agent:kick"), h.kickAgent)
}

// siteScope is the common HTTP boundary for both queries and ID-based actions.
// Reference ownership is checked before reading device data or running a command.
func (h *Handler) siteScope(c *gin.Context) {
	p, _ := auth.PrincipalFromContext(c.Request.Context())
	sites, err := h.service.Sites(c, p.UserID)
	if err != nil {
		writeResult(c, nil, err)
		c.Abort()
		return
	}
	scope := siteScope{Read: []string{}, Operate: []string{}}
	for _, s := range sites {
		scope.Read = append(scope.Read, s.Code)
		if s.CanOperate && s.Enabled == 1 {
			scope.Operate = append(scope.Operate, s.Code)
		}
	}
	c.Set(siteScopeKey, scope)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), siteScopeKey, scope))
	fail := func(err error) { writeResult(c, nil, err); c.Abort() }
	path := strings.TrimPrefix(c.FullPath(), "/api/weighing/")
	if strings.HasPrefix(path, "site-config") || strings.HasPrefix(path, "agent-settings") || strings.HasPrefix(path, "ticket-sequences") || strings.HasPrefix(path, "users/") || path == "site-options" {
		c.Next()
		return
	}
	status, statusErr := h.service.LocalStatus(c)
	if statusErr != nil {
		fail(statusErr)
		return
	}
	if strings.HasPrefix(path, "material") || strings.HasPrefix(path, "parties") {
		if c.Request.Method != http.MethodGet && !status.Configured {
			fail(ErrForbidden)
			return
		}
		c.Next()
		return
	}
	if c.Request.Method != http.MethodGet && !status.Initialized {
		fail(ErrForbidden)
		return
	}
	write := c.Request.Method != http.MethodGet
	allowed := scope.Read
	if write {
		allowed = scope.Operate
	}
	var expected string
	allow := func(site string) bool {
		if !slices.Contains(allowed, site) {
			fail(ErrForbidden)
			return false
		}
		if expected != "" && expected != site {
			fail(ErrForbidden)
			return false
		}
		expected = site
		return true
	}
	check := func(kind string, id int64) bool {
		site, e := h.service.ResourceSite(c, kind, id)
		if e != nil {
			fail(e)
			return false
		}
		return allow(site)
	}
	checkScale := func(scaleCode string) bool {
		site, e := h.service.ScaleSite(c, scaleCode)
		if e != nil {
			fail(e)
			return false
		}
		return allow(site)
	}
	if value := c.Query("siteCode"); value != "" {
		if !slices.Contains(scope.Read, value) {
			fail(ErrForbidden)
			return
		}
	}
	if value := c.Query("scale"); value != "" {
		if !checkScale(value) {
			return
		}
		scope.ScaleCode = value
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), siteScopeKey, scope))
	}
	parts := strings.Split(path, "/")
	if len(parts) > 1 && parts[0] == "scales" && parts[1] == ":code" {
		if !checkScale(c.Param("code")) {
			return
		}
	}
	if len(parts) > 1 && parts[1] == ":id" {
		id, e := pathID(c)
		if e != nil {
			fail(e)
			return
		}
		if !check(parts[0], id) {
			return
		}
	}
	if strings.HasPrefix(path, "agent-images/") {
		if _, ok := agentbridge.ParseImageID(c.Param("imageId")); !ok {
			fail(ErrInvalid)
			return
		}
		// Image prefixes are historical labels, not an authorization scope.
		local, err := h.service.LocalSiteCode(c)
		if err != nil {
			fail(err)
			return
		}
		if !allow(local) {
			return
		}
	}
	if value := c.Query("taskId"); value != "" {
		id, e := strconv.ParseInt(value, 10, 64)
		if e != nil {
			fail(ErrInvalid)
			return
		}
		if !check("tasks", id) {
			return
		}
	}
	if write && c.Request.Body != nil {
		body, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
		if e != nil {
			fail(ErrInvalid)
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		var refs struct {
			ScaleCode      string `json:"scaleCode"`
			TaskID         int64  `json:"taskId"`
			SampleID       int64  `json:"sampleId"`
			TargetRecordID int64  `json:"targetRecordId"`
			RequestID      string `json:"requestId"`
		}
		if len(body) > 0 && json.Unmarshal(body, &refs) != nil {
			fail(ErrInvalid)
			return
		}
		replay := false
		if refs.RequestID != "" {
			op, e := h.service.Operation(c, p.UserID, refs.RequestID)
			if e != nil {
				fail(e)
				return
			}
			replay = op.Status != "not_found"
			if replay && op.TaskID != nil && !check("tasks", *op.TaskID) {
				return
			}
		}
		if refs.ScaleCode != "" && !replay && !checkScale(refs.ScaleCode) {
			return
		}
		for _, v := range []struct {
			kind string
			id   int64
		}{{"tasks", refs.TaskID}, {"samples", refs.SampleID}, {"records", refs.TargetRecordID}} {
			if v.id < 0 {
				fail(ErrInvalid)
				return
			}
			if !replay && v.id > 0 && !check(v.kind, v.id) {
				return
			}
		}

	}
	if strings.HasPrefix(path, "operations/") {
		op, e := h.service.Operation(c, p.UserID, c.Param("requestId"))
		if e != nil {
			fail(e)
			return
		}
		if op.TaskID != nil && !check("tasks", *op.TaskID) {
			return
		}
	}
	c.Set(siteScopeKey, scope)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), siteScopeKey, scope))
	c.Next()
}
