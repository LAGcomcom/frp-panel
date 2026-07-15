package handler

import (
	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/pkg/license"
	"github.com/gin-gonic/gin"
)

type LicenseHandler struct {
	manager *license.Manager
}

func NewLicenseHandler(manager *license.Manager) *LicenseHandler {
	return &LicenseHandler{manager: manager}
}

// ActivateRequest is the request body for license activation.
type ActivateRequest struct {
	LicenseKey string `json:"license_key" binding:"required"`
}

// Activate validates and activates a license key.
// POST /api/license/activate
func (h *LicenseHandler) Activate(c *gin.Context) {
	var req ActivateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: license_key is required")
		return
	}

	// Verify with remote auth server
	info, err := h.manager.Verify(req.LicenseKey)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Save encrypted license locally
	if err := h.manager.SaveLicense(info); err != nil {
		response.InternalError(c, "failed to save license: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"license_key": info.LicenseKey,
		"expires_at":  info.ExpiresAt,
		"prefix":      info.Prefix,
		"valid":       true,
		"device_id":   license.GetDeviceID(),
	})
}

// Status returns the current license status.
// GET /api/license/status
func (h *LicenseHandler) Status(c *gin.Context) {
	if !h.manager.IsActive() {
		response.Success(c, gin.H{
			"active": false,
			"device_id": license.GetDeviceID(),
		})
		return
	}

	info := h.manager.GetCurrent()
	response.Success(c, gin.H{
		"active":      true,
		"license_key": info.LicenseKey,
		"expires_at":  info.ExpiresAt,
		"prefix":      info.Prefix,
		"device_id":   license.GetDeviceID(),
	})
}

// DeviceID returns the server's device ID (useful for pre-checking).
// GET /api/license/device-id
func (h *LicenseHandler) DeviceID(c *gin.Context) {
	response.Success(c, gin.H{
		"device_id": license.GetDeviceID(),
	})
}
