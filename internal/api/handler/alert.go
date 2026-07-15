package handler

import (
	"strconv"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/service/monitor"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AlertHandler struct {
	db           *gorm.DB
	alertManager *monitor.AlertManager
}

func NewAlertHandler(db *gorm.DB, am *monitor.AlertManager) *AlertHandler {
	return &AlertHandler{db: db, alertManager: am}
}

// GetAlerts returns alerts for the current user
func (h *AlertHandler) GetAlerts(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	unreadOnly := c.DefaultQuery("unread_only", "false") == "true"

	alerts := h.alertManager.GetAlerts(userID, unreadOnly)
	response.Success(c, alerts)
}

// MarkAlertRead marks an alert as read
func (h *AlertHandler) MarkAlertRead(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	alertID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid alert id")
		return
	}

	h.alertManager.MarkAlertRead(uint(alertID), userID)
	response.SuccessWithMessage(c, "alert marked as read", nil)
}

// AdminGetAlerts returns all alerts (admin)
func (h *AlertHandler) AdminGetAlerts(c *gin.Context) {
	page := parseInt(c.DefaultQuery("page", "1"))
	size := parseInt(c.DefaultQuery("size", "20"))

	alerts, total := h.alertManager.GetAllAlerts(page, size)
	response.Page(c, alerts, total, page, size)
}

// AdminSendNotification sends a notification to a user or broadcasts to all
func (h *AlertHandler) AdminSendNotification(c *gin.Context) {
	var req struct {
		UserID  *uint  `json:"user_id"` // nil = broadcast to all
		Title   string `json:"title" binding:"required"`
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Validate user exists if specified
	if req.UserID != nil {
		var user model.User
		if err := h.db.First(&user, *req.UserID).Error; err != nil {
			response.BadRequest(c, "user not found")
			return
		}
		h.alertManager.SendNotification(req.UserID, req.Title, req.Message)
	} else {
		// Broadcast to all users
		var users []model.User
		h.db.Find(&users)
		for _, user := range users {
			uid := user.ID
			h.alertManager.SendNotification(&uid, req.Title, req.Message)
		}
	}

	response.SuccessWithMessage(c, "通知已发送", nil)
}
