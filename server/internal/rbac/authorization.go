package rbac

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/auth"
	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// PermissionCodes reads current grants rather than trusting claims or menu visibility.
func (s *Service) PermissionCodes(ctx context.Context, userID int64) ([]string, error) {
	return s.store.PermissionCodes(ctx, userID)
}

func (s *Service) HasPermission(ctx context.Context, userID int64, code string) (bool, error) {
	codes, err := s.PermissionCodes(ctx, userID)
	return slices.Contains(codes, code), err
}

type PermissionChecker interface {
	HasPermission(context.Context, int64, string) (bool, error)
}

// RequirePermission is applied after BearerMiddleware, including hidden action grants.
func RequirePermission(checker PermissionChecker, code string) gin.HandlerFunc {
	return RequireAnyPermission(checker, code)
}

func RequireAnyPermission(checker PermissionChecker, codes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c.Request.Context())
		if !ok {
			platformhttp.AbortError(c, 401, 401, "请先登录", nil)
			return
		}
		for _, code := range codes {
			allowed, err := checker.HasPermission(c.Request.Context(), principal.UserID, code)
			if err != nil {
				if platformhttp.IsTemporaryUnavailable(err) {
					c.Abort()
					platformhttp.TemporaryUnavailable(c)
				} else {
					platformhttp.AbortError(c, 500, 500, "权限查询失败", nil)
				}
				return
			}
			if allowed {
				c.Next()
				return
			}
		}
		platformhttp.AbortError(c, 403, 403, "无权执行此操作", nil)
	}
}

// ProtectManagementWrites closes existing grant and account mutation paths.
// Self-service password/avatar routes retain their established authentication contract.
func ProtectManagementWrites(checker PermissionChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		path := c.FullPath()
		code := ""
		switch {
		case strings.HasPrefix(path, "/api/system/role"):
			code = "system:role"
		case strings.HasPrefix(path, "/api/system/menu"):
			code = "system:menu"
		case strings.HasPrefix(path, "/api/system/user/") && strings.Contains(path, "/me/"):
			// These routes explicitly operate on the authenticated user.
		case path == "/api/system/user" || strings.HasPrefix(path, "/api/system/user/"):
			code = "system:user"
		}
		if code == "" {
			c.Next()
			return
		}
		RequirePermission(checker, code)(c)
	}
}
