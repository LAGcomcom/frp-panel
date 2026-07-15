package middleware

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/pkg/license"
	"github.com/gin-gonic/gin"
)

// licensePageHTML stores the embedded license activation page.
// Set by SetLicensePage during initialization.
var licensePageHTML []byte

// SetLicensePage sets the license activation page content.
func SetLicensePage(data []byte) {
	licensePageHTML = data
}

// LoadLicensePageFromFS loads the license page from an embedded filesystem.
func LoadLicensePageFromFS(fsys fs.FS, path string) {
	data, err := fs.ReadFile(fsys, path)
	if err == nil {
		licensePageHTML = data
	}
}

// LicenseRequired blocks all routes until a valid license is activated.
// For browser requests, shows the activation page.
// For API requests, returns JSON 403.
func LicenseRequired(manager *license.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Allow license endpoints and health check without license
		if path == "/healthz" ||
			path == "/api/license/activate" ||
			path == "/api/license/status" ||
			path == "/api/license/device-id" {
			c.Next()
			return
		}

		// Allow static assets so the activation page can render
		if isStaticAsset(path) {
			c.Next()
			return
		}

		// Allow agent binary download
		if path == "/agent" {
			c.Next()
			return
		}

		// Anti-tamper: distributed check point 4 (per-request)
		// Log warning but don't block — recompilation changes binary hash
		if !license.AntiTamperCheck() {
			log.Printf("[LICENSE] Anti-tamper check failed for %s (warning, not blocking)", path)
		}

		if !manager.IsActive() {
			// Browser request -> serve activation page HTML
			if isBrowserRequest(c) && licensePageHTML != nil {
				c.Data(http.StatusOK, "text/html; charset=utf-8", licensePageHTML)
				c.Abort()
				return
			}
			// API request -> return JSON 403
			response.Forbidden(c, "panel license not activated. Please activate a license first.")
			c.Abort()
			return
		}

		c.Next()
	}
}

// isBrowserRequest checks if the request is from a browser (Accept: text/html).
func isBrowserRequest(c *gin.Context) bool {
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "text/html")
}

// isStaticAsset checks if the path is a static asset (CSS, JS, images, fonts).
func isStaticAsset(path string) bool {
	// Check file extensions
	extensions := []string{".css", ".js", ".png", ".jpg", ".jpeg", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".map"}
	for _, ext := range extensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	// Check asset subdirectories
	if strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/admin/assets/") ||
		strings.HasPrefix(path, "/user/assets/") {
		return true
	}
	return false
}
