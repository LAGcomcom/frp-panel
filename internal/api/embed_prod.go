//go:build !dev

package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/frp-panel/frp-panel/internal/api/middleware"
	"github.com/frp-panel/frp-panel/internal/pkg/license"
	"github.com/gin-gonic/gin"
)

//go:embed all:dist/landing
var landingFS embed.FS

//go:embed all:dist/admin
var adminFS embed.FS

//go:embed all:dist/user
var userFS embed.FS

//go:embed all:dist/license
var licenseFS embed.FS

//go:embed all:bin
var binFS embed.FS

var licenseManager *license.Manager

func registerStaticRoutes(r *gin.Engine, lm *license.Manager) {
	licenseManager = lm

	landingSub, _ := fs.Sub(landingFS, "dist/landing")
	adminSub, _ := fs.Sub(adminFS, "dist/admin")
	userSub, _ := fs.Sub(userFS, "dist/user")
	licenseSub, _ := fs.Sub(licenseFS, "dist/license")

	// Load license activation page into middleware
	if licenseData, err := fs.ReadFile(licenseSub, "index.html"); err == nil {
		middleware.SetLicensePage(licenseData)
	}

	// Get assets subdirectories
	adminAssets, _ := fs.Sub(adminSub, "assets")
	userAssets, _ := fs.Sub(userSub, "assets")

	// Static assets
	r.StaticFS("/admin/assets", http.FS(adminAssets))
	r.StaticFS("/user/assets", http.FS(userAssets))

	// Serve agent binary
	r.GET("/agent", func(c *gin.Context) {
		data, err := fs.ReadFile(binFS, "bin/agent")
		if err != nil {
			c.Status(404)
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", data)
	})

	// SPA middleware
	r.Use(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip API, WebSocket, and asset routes
		if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/ws") ||
			strings.HasPrefix(path, "/admin/assets") || strings.HasPrefix(path, "/user/assets") {
			c.Next()
			return
		}

		// If license is not active, serve the license activation page
		if licenseManager != nil && !licenseManager.IsActive() {
			data, _ := fs.ReadFile(licenseSub, "index.html")
			if data != nil {
				c.Data(http.StatusOK, "text/html; charset=utf-8", data)
				c.Abort()
				return
			}
		}

		// Serve admin SPA
		if strings.HasPrefix(path, "/admin") {
			data, _ := fs.ReadFile(adminSub, "index.html")
			if data != nil {
				c.Data(http.StatusOK, "text/html; charset=utf-8", data)
				c.Abort()
				return
			}
		}

		// Serve user SPA
		if strings.HasPrefix(path, "/user") {
			data, _ := fs.ReadFile(userSub, "index.html")
			if data != nil {
				c.Data(http.StatusOK, "text/html; charset=utf-8", data)
				c.Abort()
				return
			}
		}

		// Serve landing page for root
		if path == "/" {
			data, _ := fs.ReadFile(landingSub, "index.html")
			if data != nil {
				c.Data(http.StatusOK, "text/html; charset=utf-8", data)
				c.Abort()
				return
			}
		}

		c.Next()
	})
}
