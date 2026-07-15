package handler

import (
	"net/http"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WebsiteHandler struct {
	db *gorm.DB
}

func NewWebsiteHandler(db *gorm.DB) *WebsiteHandler {
	return &WebsiteHandler{db: db}
}

type CreateWebsiteRequest struct {
	Name        string `json:"name" binding:"required"`
	Domain      string `json:"domain" binding:"required"`
	Subdomain   string `json:"subdomain"`
	Type        string `json:"type" binding:"required,oneof=http https"`
	ServerID    uint   `json:"server_id" binding:"required"`
	UserID      uint   `json:"user_id" binding:"required"`
	BackendAddr string `json:"backend_addr" binding:"required"`
}

func (h *WebsiteHandler) CreateWebsite(c *gin.Context) {
	var req CreateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Check domain uniqueness on the same server
	var count int64
	h.db.Model(&model.Website{}).Where("server_id = ? AND domain = ? AND subdomain = ? AND deleted_at IS NULL", req.ServerID, req.Domain, req.Subdomain).Count(&count)
	if count > 0 {
		response.BadRequest(c, "域名已被占用")
		return
	}

	// Verify server exists
	if err := h.db.First(&model.Server{}, req.ServerID).Error; err != nil {
		response.BadRequest(c, "服务器不存在")
		return
	}

	// Verify user exists
	if err := h.db.First(&model.User{}, req.UserID).Error; err != nil {
		response.BadRequest(c, "用户不存在")
		return
	}

	// Create associated Proxy
	proxy := model.Proxy{
		UserID:        req.UserID,
		ServerID:      req.ServerID,
		Name:          req.Name,
		Type:          req.Type,
		LocalIP:       "127.0.0.1",
		CustomDomains: req.Domain,
		Subdomain:     req.Subdomain,
		Status:        "pending",
	}
	if err := h.db.Create(&proxy).Error; err != nil {
		response.InternalError(c, "创建代理记录失败")
		return
	}

	website := model.Website{
		Name:        req.Name,
		Domain:      req.Domain,
		Subdomain:   req.Subdomain,
		Type:        req.Type,
		ServerID:    req.ServerID,
		UserID:      req.UserID,
		ProxyID:     proxy.ID,
		BackendAddr: req.BackendAddr,
		SSLStatus:   "none",
		Status:      "pending",
	}

	if err := h.db.Create(&website).Error; err != nil {
		// Rollback proxy
		h.db.Delete(&proxy)
		response.InternalError(c, "创建网站失败")
		return
	}

	response.SuccessWithMessage(c, "网站创建成功", website)
}

func (h *WebsiteHandler) ListWebsites(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	size := c.DefaultQuery("size", "20")
	status := c.DefaultQuery("status", "")
	serverID := c.DefaultQuery("server_id", "")
	userID := c.DefaultQuery("user_id", "")
	search := c.DefaultQuery("search", "")

	var websites []model.Website
	var total int64

	query := h.db.Model(&model.Website{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if search != "" {
		query = query.Where("name LIKE ? OR domain LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	p := parseInt(page)
	s := parseInt(size)
	offset := (p - 1) * s
	query.Preload("Server").Preload("User").Offset(offset).Limit(s).Order("id desc").Find(&websites)

	response.Page(c, websites, total, p, s)
}

func (h *WebsiteHandler) GetWebsite(c *gin.Context) {
	id := c.Param("id")

	var website model.Website
	if err := h.db.Preload("Server").Preload("User").First(&website, id).Error; err != nil {
		response.NotFound(c, "网站不存在")
		return
	}

	response.Success(c, website)
}

type UpdateWebsiteRequest struct {
	Name        *string `json:"name"`
	Domain      *string `json:"domain"`
	Subdomain   *string `json:"subdomain"`
	Type        *string `json:"type"`
	BackendAddr *string `json:"backend_addr"`
	Status      *string `json:"status"`
}

func (h *WebsiteHandler) UpdateWebsite(c *gin.Context) {
	id := c.Param("id")

	var website model.Website
	if err := h.db.First(&website, id).Error; err != nil {
		response.NotFound(c, "网站不存在")
		return
	}

	var req UpdateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Domain != nil {
		updates["domain"] = *req.Domain
	}
	if req.Subdomain != nil {
		updates["subdomain"] = *req.Subdomain
	}
	if req.Type != nil {
		updates["type"] = *req.Type
	}
	if req.BackendAddr != nil {
		updates["backend_addr"] = *req.BackendAddr
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	// Check domain uniqueness if domain/subdomain changed
	if req.Domain != nil || req.Subdomain != nil {
		domain := website.Domain
		subdomain := website.Subdomain
		if req.Domain != nil {
			domain = *req.Domain
		}
		if req.Subdomain != nil {
			subdomain = *req.Subdomain
		}
		var count int64
		h.db.Model(&model.Website{}).Where("server_id = ? AND domain = ? AND subdomain = ? AND id != ? AND deleted_at IS NULL", website.ServerID, domain, subdomain, website.ID).Count(&count)
		if count > 0 {
			response.BadRequest(c, "域名已被占用")
			return
		}
	}

	if err := h.db.Model(&website).Updates(updates).Error; err != nil {
		response.InternalError(c, "更新失败")
		return
	}

	// Sync to associated proxy
	if req.Name != nil || req.Domain != nil || req.Subdomain != nil || req.Type != nil || req.Status != nil {
		proxyUpdates := map[string]interface{}{}
		if req.Name != nil {
			proxyUpdates["name"] = *req.Name
		}
		if req.Domain != nil {
			proxyUpdates["custom_domains"] = *req.Domain
		}
		if req.Subdomain != nil {
			proxyUpdates["subdomain"] = *req.Subdomain
		}
		if req.Type != nil {
			proxyUpdates["type"] = *req.Type
		}
		if req.Status != nil {
			proxyUpdates["status"] = *req.Status
		}
		if len(proxyUpdates) > 0 && website.ProxyID > 0 {
			h.db.Model(&model.Proxy{}).Where("id = ?", website.ProxyID).Updates(proxyUpdates)
		}
	}

	h.db.Preload("Server").Preload("User").First(&website, website.ID)
	response.SuccessWithMessage(c, "更新成功", website)
}

func (h *WebsiteHandler) DeleteWebsite(c *gin.Context) {
	id := c.Param("id")

	var website model.Website
	if err := h.db.First(&website, id).Error; err != nil {
		response.NotFound(c, "网站不存在")
		return
	}

	// Delete associated proxy
	if website.ProxyID > 0 {
		h.db.Where("id = ?", website.ProxyID).Delete(&model.Proxy{})
	}

	if err := h.db.Delete(&website).Error; err != nil {
		response.InternalError(c, "删除失败")
		return
	}

	response.SuccessWithMessage(c, "删除成功", nil)
}

func (h *WebsiteHandler) CheckWebsite(c *gin.Context) {
	id := c.Param("id")

	var website model.Website
	if err := h.db.First(&website, id).Error; err != nil {
		response.NotFound(c, "网站不存在")
		return
	}

	start := time.Now()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + website.BackendAddr)
	elapsed := time.Since(start).Milliseconds()

	updates := map[string]interface{}{
		"response_time": int(elapsed),
	}

	if err != nil {
		updates["status"] = "error"
		updates["error_msg"] = err.Error()
		h.db.Model(&website).Updates(updates)
		h.db.Preload("Server").Preload("User").First(&website, website.ID)
		response.SuccessWithMessage(c, "检测完成：不可达", website)
		return
	}
	defer resp.Body.Close()

	updates["status"] = "running"
	updates["error_msg"] = ""
	h.db.Model(&website).Updates(updates)
	h.db.Preload("Server").Preload("User").First(&website, website.ID)
	response.SuccessWithMessage(c, "检测完成：正常", website)
}
