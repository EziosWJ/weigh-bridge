package weighing

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service     *Service
	permissions *rbac.Service
}

func NewHandler(s *Service, permissions *rbac.Service) *Handler {
	return &Handler{service: s, permissions: permissions}
}
func RegisterRoutes(router gin.IRouter, h *Handler) {
	router.Use(h.siteScope)
	registerSiteRoutes(router, h)
	registerTemporaryRoutes(router, h)
	registerConfigurationApplication(router, h)
	registerSiteConfigurationRoutes(router, h)
	router.GET("/capabilities", h.capabilities)
	registerWorkflowRoutes(router, h)
	registerCorrectionRoutes(router, h)
	registerQueryRoutes(router, h)
	for _, entry := range []struct{ path, kind string }{{"material-categories", "category"}, {"materials", "material"}, {"parties", "party"}} {
		group := router.Group("/" + entry.path)
		read := rbac.RequirePermission(h.permissions, "weighing:master:read")
		write := rbac.RequirePermission(h.permissions, "weighing:master:write")
		group.GET("", read, h.page(entry.kind))
		group.GET("/:id", read, h.detail(entry.kind))
		group.POST("", write, h.save(entry.kind))
		group.PUT("/:id", write, h.save(entry.kind))
		group.DELETE("/:id", write, h.delete(entry.kind))
		group.PATCH("/:id/status", write, h.status(entry.kind))
	}
}

// capabilities godoc
// @Summary 当前称重业务权限及地磅
// @Tags 称重业务
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/capabilities [get]
func (h *Handler) capabilities(c *gin.Context) {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	codes, err := h.permissions.PermissionCodes(c.Request.Context(), principal.UserID)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	scales, err := h.service.Scales(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	sites, err := h.service.Sites(c, principal.UserID)
	writeResult(c, map[string]any{"userId": principal.UserID, "permissionCodes": codes, "scales": scales, "sites": sites, "maintenanceEnabled": h.service.maintenanceEnabled, "nodeRole": h.service.node.Role, "nodeSiteId": h.service.node.SiteID}, err)
}
func pageQuery(c *gin.Context) (PageQuery, error) {
	q := PageQuery{Page: 1, PageSize: 10, Keyword: strings.TrimSpace(c.Query("keyword")), Role: c.Query("role"), Nature: c.Query("nature")}
	for _, v := range []struct {
		key    string
		target *int
	}{{"page", &q.Page}, {"pageSize", &q.PageSize}} {
		if c.Query(v.key) != "" {
			n, err := strconv.Atoi(c.Query(v.key))
			if err != nil {
				return q, ErrInvalid
			}
			*v.target = n
		}
	}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 500 {
		return q, ErrInvalid
	}
	if value := c.Query("enabled"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || (n != 0 && n != 1) {
			return q, ErrInvalid
		}
		q.Enabled = &n
	}
	if value := c.Query("categoryId"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 1 {
			return q, ErrInvalid
		}
		q.CategoryID = &n
	}
	if value := c.Query("taskId"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 1 {
			return q, ErrInvalid
		}
		q.TaskID = &n
	}
	q.Direction = c.Query("direction")
	q.Status = c.Query("status")
	q.From = c.Query("from")
	q.To = c.Query("to")
	if q.Direction != "" && q.Direction != "inbound" && q.Direction != "outbound" {
		return q, ErrInvalid
	}
	for _, field := range []struct {
		key    string
		target **int64
	}{{"materialId", &q.MaterialID}, {"partyId", &q.PartyID}, {"siteId", &q.SiteID}} {
		if value := c.Query(field.key); value != "" {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 1 {
				return q, ErrInvalid
			}
			*field.target = &n
		}
	}
	if _, _, err := dateBounds(q); err != nil {
		return q, err
	}
	if q.Role != "" && q.Role != "supplier" && q.Role != "customer" && q.Role != "carrier" {
		return q, ErrInvalid
	}
	if q.Nature != "" && q.Nature != "enterprise" && q.Nature != "individual" {
		return q, ErrInvalid
	}
	return q, nil
}
func pathID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, ErrInvalid
	}
	return id, nil
}

// page godoc
// @Summary 资料分页查询
// @Tags 称重资料
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/material-categories [get]
// @Router /api/weighing/materials [get]
// @Router /api/weighing/parties [get]
func (h *Handler) page(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		q, err := pageQuery(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		switch kind {
		case "category":
			v, e := h.service.Categories(c, q)
			writeResult(c, v, e)
		case "material":
			v, e := h.service.Materials(c, q)
			writeResult(c, v, e)
		case "party":
			v, e := h.service.Parties(c, q)
			writeResult(c, v, e)
		}
	}
}

// detail godoc
// @Summary 资料详情
// @Tags 称重资料
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/material-categories/{id} [get]
// @Router /api/weighing/materials/{id} [get]
// @Router /api/weighing/parties/{id} [get]
func (h *Handler) detail(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		switch kind {
		case "category":
			v, e := h.service.FindCategory(c, id)
			writeResult(c, v, e)
		case "material":
			v, e := h.service.FindMaterial(c, id)
			writeResult(c, v, e)
		case "party":
			v, e := h.service.FindParty(c, id)
			writeResult(c, v, e)
		}
	}
}

// save godoc
// @Summary 保存资料
// @Tags 称重资料
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/material-categories [post]
// @Router /api/weighing/materials [post]
// @Router /api/weighing/parties [post]
// @Router /api/weighing/material-categories/{id} [put]
// @Router /api/weighing/materials/{id} [put]
// @Router /api/weighing/parties/{id} [put]
func (h *Handler) save(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := int64(0)
		var err error
		if c.Request.Method == http.MethodPut {
			id, err = pathID(c)
			if err != nil {
				writeResult(c, nil, err)
				return
			}
		}
		event := requestEvent(c)
		switch kind {
		case "category":
			var in CategoryInput
			if c.ShouldBindJSON(&in) != nil {
				writeResult(c, nil, ErrInvalid)
				return
			}
			v, e := h.service.SaveCategory(c, id, in, event)
			writeResult(c, v, e)
		case "material":
			var in MaterialInput
			if c.ShouldBindJSON(&in) != nil {
				writeResult(c, nil, ErrInvalid)
				return
			}
			v, e := h.service.SaveMaterial(c, id, in, event)
			writeResult(c, v, e)
		case "party":
			var in PartyInput
			if c.ShouldBindJSON(&in) != nil {
				writeResult(c, nil, ErrInvalid)
				return
			}
			v, e := h.service.SaveParty(c, id, in, event)
			writeResult(c, v, e)
		}
	}
}

// delete godoc
// @Summary 删除未引用资料
// @Tags 称重资料
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/material-categories/{id} [delete]
// @Router /api/weighing/materials/{id} [delete]
// @Router /api/weighing/parties/{id} [delete]
func (h *Handler) delete(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		var in struct {
			Version int64 `json:"version"`
		}
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		writeResult(c, nil, h.service.DeleteMaster(c, kind, id, in.Version, requestEvent(c)))
	}
}

// status godoc
// @Summary 资料启停
// @Tags 称重资料
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/material-categories/{id}/status [patch]
// @Router /api/weighing/materials/{id}/status [patch]
// @Router /api/weighing/parties/{id}/status [patch]
func (h *Handler) status(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		var in struct {
			Version int64 `json:"version"`
			Enabled *bool `json:"enabled"`
		}
		if c.ShouldBindJSON(&in) != nil || in.Enabled == nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		writeResult(c, nil, h.service.SetMasterStatus(c, kind, id, in.Version, flag(*in.Enabled), requestEvent(c)))
	}
}
func requestEvent(c *gin.Context) audit.Event {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	m, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
	return audit.Event{Metadata: audit.Metadata{ActorID: principal.UserID, RequestID: m.RequestID, ClientIP: m.ClientIP, UserAgent: m.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI()}}
}
func writeResult(c *gin.Context, value any, err error) {
	if err == nil {
		platformhttp.OK(c, value)
		return
	}
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(err, ErrForbidden):
		platformhttp.WriteError(c, 403, 403, err.Error(), nil)
	case errors.Is(err, ErrNotFound):
		platformhttp.WriteError(c, 404, 404, err.Error(), nil)
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrConflict), errors.Is(err, ErrReferenced), errors.Is(err, ErrCapturePolicyChanged), errors.Is(err, ErrConditions), errors.Is(err, ErrOccupied), errors.Is(err, ErrSession), errors.Is(err, ErrRequestKey), errors.Is(err, ErrWeight), isRejection(err):
		platformhttp.WriteError(c, 200, 400, err.Error(), nil)
	default:
		platformhttp.WriteError(c, 500, 500, "业务处理失败", nil)
	}
}

// HTTPEnvelope represents the existing API envelope in generated contracts.
type HTTPEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}
