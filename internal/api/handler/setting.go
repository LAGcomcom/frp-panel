package handler

import (
	"fmt"
	"net/smtp"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SettingHandler struct {
	db *gorm.DB
}

func NewSettingHandler(db *gorm.DB) *SettingHandler {
	return &SettingHandler{db: db}
}

// GetSettings returns all settings (admin only), masks sensitive fields
func (h *SettingHandler) GetSettings(c *gin.Context) {
	var settings []model.Setting
	h.db.Find(&settings)

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}

	// Mask smtp_password
	if result["smtp_password"] != "" {
		result["smtp_password"] = "********"
	}

	response.Success(c, result)
}

// UpdateSettings batch-updates settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	// Don't allow updating password through masked value
	if req["smtp_password"] == "********" {
		delete(req, "smtp_password")
	}

	for key, val := range req {
		var existing model.Setting
		if err := h.db.Where("key = ?", key).First(&existing).Error; err != nil {
			// Create new
			h.db.Create(&model.Setting{Key: key, Value: val})
		} else {
			h.db.Model(&existing).Update("value", val)
		}
	}

	response.SuccessWithMessage(c, "设置已保存", nil)
}

// GetPublicSettings returns settings safe for unauthenticated access
func (h *SettingHandler) GetPublicSettings(c *gin.Context) {
	keys := []string{"registration_enabled", "login_enabled", "site_announcement", "email_verification_enabled", "site_title"}
	var settings []model.Setting
	h.db.Where("key IN ?", keys).Find(&settings)

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}

	// Defaults
	if result["registration_enabled"] == "" {
		result["registration_enabled"] = "true"
	}
	if result["login_enabled"] == "" {
		result["login_enabled"] = "true"
	}

	response.Success(c, result)
}

// TestSMTP sends a test email via configured SMTP
func (h *SettingHandler) TestSMTP(c *gin.Context) {
	var req struct {
		To string `json:"to" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请输入有效的测试邮箱地址")
		return
	}

	settings := h.getSettingsMap()

	host := settings["smtp_host"]
	port := settings["smtp_port"]
	user := settings["smtp_user"]
	password := settings["smtp_password"]
	from := settings["smtp_from"]

	if host == "" || user == "" || password == "" {
		response.BadRequest(c, "请先配置 SMTP 参数")
		return
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	auth := smtp.PlainAuth("", user, password, host)

	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: FRP Panel SMTP Test\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n这是一封来自 FRP Panel 的测试邮件。SMTP 配置成功！", from, req.To))

	if err := smtp.SendMail(addr, auth, from, []string{req.To}, msg); err != nil {
		response.InternalError(c, "发送失败: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "测试邮件已发送", nil)
}

func (h *SettingHandler) getSettingsMap() map[string]string {
	var settings []model.Setting
	h.db.Find(&settings)
	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result
}
