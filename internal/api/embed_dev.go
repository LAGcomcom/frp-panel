//go:build dev

package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/frp-panel/frp-panel/internal/api/middleware"
	"github.com/frp-panel/frp-panel/internal/pkg/license"
	"github.com/gin-gonic/gin"
)

func projectRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func registerStaticRoutes(r *gin.Engine, lm *license.Manager) {
	root := projectRoot()

	// Load license activation page into middleware
	licensePage := filepath.Join(root, "internal/api/dist/license/index.html")
	if data, err := os.ReadFile(licensePage); err == nil {
		middleware.SetLicensePage(data)
	}

	r.Static("/admin/assets", filepath.Join(root, "web/admin/dist"))
	r.Static("/user/assets", filepath.Join(root, "web/user/dist"))
	r.StaticFile("/favicon.ico", filepath.Join(root, "web/landing/favicon.ico"))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// If license is not active, serve license activation page
		if lm != nil && !lm.IsActive() {
			licensePage := filepath.Join(root, "internal/api/dist/license/index.html")
			if _, err := os.Stat(licensePage); err == nil {
				c.File(licensePage)
				return
			}
		}

		switch {
		case strings.HasPrefix(path, "/admin"):
			serveSPA(c, filepath.Join(root, "web/admin/dist/index.html"))
		case strings.HasPrefix(path, "/user"):
			serveSPA(c, filepath.Join(root, "web/user/dist/index.html"))
		default:
			c.File(filepath.Join(root, "web/landing/index.html"))
		}
	})
}

func serveSPA(c *gin.Context, indexPath string) {
	if _, err := os.Stat(indexPath); err != nil {
		c.JSON(404, gin.H{"error": "frontend not built"})
		return
	}
	c.File(indexPath)
}
