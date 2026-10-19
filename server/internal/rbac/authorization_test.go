package rbac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/gin-gonic/gin"
)

type checkerStub struct{ code string }

func (s checkerStub) HasPermission(_ context.Context, _ int64, code string) (bool, error) {
	return code == s.code, nil
}

func TestManagementWriteAuthorizationAndSelfService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		path, route, grant string
		want               int
	}{
		{"/api/system/role/2/menus", "/api/system/role/:id/menus", "", 403},
		{"/api/system/role/2/menus", "/api/system/role/:id/menus", "system:role", 204},
		{"/api/system/user/1/roles", "/api/system/user/:id/roles", "", 403},
		{"/api/system/user/1/reset-password", "/api/system/user/:id/reset-password", "", 403},
		{"/api/system/user/1/roles", "/api/system/user/:id/roles", "system:user", 204},
		{"/api/system/menu/1", "/api/system/menu/:id", "system:role", 403},
		{"/api/system/user/me/password", "/api/system/user/me/password", "", 204},
		{"/api/system/user/me/avatar", "/api/system/user/me/avatar", "", 204},
	} {
		t.Run(test.path+test.grant, func(t *testing.T) {
			router := gin.New()
			router.Use(ProtectManagementWrites(checkerStub{test.grant}))
			router.PUT(test.route, func(c *gin.Context) { c.Status(204) })
			request := httptest.NewRequest(http.MethodPut, test.path, nil)
			request = request.WithContext(auth.ContextWithPrincipal(request.Context(), auth.Principal{UserID: 8}))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d, want %d: %s", response.Code, test.want, response.Body.String())
			}
		})
	}
}
