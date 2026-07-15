package handler

import (
	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AnnouncementHandler struct {
	db *gorm.DB
}

func NewAnnouncementHandler(db *gorm.DB) *AnnouncementHandler {
	return &AnnouncementHandler{db: db}
}

// --- Public ---

func (h *AnnouncementHandler) GetActiveAnnouncements(c *gin.Context) {
	var announcements []model.Announcement
	h.db.Where("enabled = ?", true).Order("sort_order desc, id desc").Find(&announcements)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": announcements})
}

// --- Admin CRUD ---

func (h *AnnouncementHandler) ListAnnouncements(c *gin.Context) {
	var announcements []model.Announcement
	h.db.Order("sort_order desc, id desc").Find(&announcements)
	response.Success(c, announcements)
}

type CreateAnnouncementRequest struct {
	Title     string `json:"title" binding:"required"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	Enabled   *bool  `json:"enabled"`
	SortOrder *int   `json:"sort_order"`
}

func (h *AnnouncementHandler) CreateAnnouncement(c *gin.Context) {
	var req CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	annType := "info"
	if req.Type != "" {
		annType = req.Type
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	announcement := model.Announcement{
		Title:     req.Title,
		Content:   req.Content,
		Type:      annType,
		Enabled:   enabled,
		SortOrder: sortOrder,
	}

	if err := h.db.Create(&announcement).Error; err != nil {
		response.InternalError(c, "创建失败")
		return
	}

	response.SuccessWithMessage(c, "创建成功", announcement)
}

func (h *AnnouncementHandler) GetAnnouncement(c *gin.Context) {
	id := c.Param("id")

	var announcement model.Announcement
	if err := h.db.First(&announcement, id).Error; err != nil {
		response.NotFound(c, "公告不存在")
		return
	}

	response.Success(c, announcement)
}

type UpdateAnnouncementRequest struct {
	Title     *string `json:"title"`
	Content   *string `json:"content"`
	Type      *string `json:"type"`
	Enabled   *bool   `json:"enabled"`
	SortOrder *int    `json:"sort_order"`
}

func (h *AnnouncementHandler) UpdateAnnouncement(c *gin.Context) {
	id := c.Param("id")

	var announcement model.Announcement
	if err := h.db.First(&announcement, id).Error; err != nil {
		response.NotFound(c, "公告不存在")
		return
	}

	var req UpdateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Content != nil {
		updates["content"] = *req.Content
	}
	if req.Type != nil {
		updates["type"] = *req.Type
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}

	if err := h.db.Model(&announcement).Updates(updates).Error; err != nil {
		response.InternalError(c, "更新失败")
		return
	}

	h.db.First(&announcement, announcement.ID)
	response.SuccessWithMessage(c, "更新成功", announcement)
}

func (h *AnnouncementHandler) DeleteAnnouncement(c *gin.Context) {
	id := c.Param("id")

	var announcement model.Announcement
	if err := h.db.First(&announcement, id).Error; err != nil {
		response.NotFound(c, "公告不存在")
		return
	}

	if err := h.db.Delete(&announcement).Error; err != nil {
		response.InternalError(c, "删除失败")
		return
	}

	response.SuccessWithMessage(c, "删除成功", nil)
}

func (h *AnnouncementHandler) ToggleAnnouncement(c *gin.Context) {
	id := c.Param("id")

	var announcement model.Announcement
	if err := h.db.First(&announcement, id).Error; err != nil {
		response.NotFound(c, "公告不存在")
		return
	}

	newState := !announcement.Enabled
	if err := h.db.Model(&announcement).Update("enabled", newState).Error; err != nil {
		response.InternalError(c, "操作失败")
		return
	}

	msg := "已禁用"
	if newState {
		msg = "已启用"
	}
	response.SuccessWithMessage(c, msg, gin.H{"enabled": newState})
}
