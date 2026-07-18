//go:build !offline

package api

import (
	"testing"

	"github.com/frp-panel/frp-panel/internal/config"
	updateservice "github.com/frp-panel/frp-panel/internal/service/update"
	"github.com/gin-gonic/gin"
)

func TestOnlineRouterRegistersUpdateEndpoints(t *testing.T) {
	router := gin.New()
	registerUpdateRoutes(router.Group("/api/admin"), updateservice.NewClient(config.UpdateConfig{}, ""))
	paths := make(map[string]bool)
	for _, route := range router.Routes() {
		paths[route.Path] = true
	}
	for _, path := range []string{
		"/api/admin/update/check",
		"/api/admin/update/lease",
		"/api/admin/update/download/:version",
	} {
		if !paths[path] {
			t.Fatalf("online router did not register %s", path)
		}
	}
}
