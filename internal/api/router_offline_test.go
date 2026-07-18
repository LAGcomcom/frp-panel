//go:build offline

package api

import (
	"strings"
	"testing"

	"github.com/frp-panel/frp-panel/internal/config"
	updateservice "github.com/frp-panel/frp-panel/internal/service/update"
	"github.com/gin-gonic/gin"
)

func TestOfflineRouterDoesNotRegisterUpdateEndpoints(t *testing.T) {
	router := gin.New()
	registerUpdateRoutes(router.Group("/api/admin"), updateservice.NewClient(config.UpdateConfig{}, ""))
	for _, route := range router.Routes() {
		if strings.HasPrefix(route.Path, "/api/admin/update/") {
			t.Fatalf("offline router registered update endpoint %s %s", route.Method, route.Path)
		}
	}
}
