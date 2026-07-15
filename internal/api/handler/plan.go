package handler

import (
	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PlanHandler struct {
	db *gorm.DB
}

func NewPlanHandler(db *gorm.DB) *PlanHandler {
	return &PlanHandler{db: db}
}

type CreatePlanRequest struct {
	Name           string  `json:"name" binding:"required"`
	Description    string  `json:"description"`
	MaxProxies     int     `json:"max_proxies"`
	MaxBandwidth   int64   `json:"max_bandwidth"`
	MaxTraffic     int64   `json:"max_traffic"`
	MaxPorts       int     `json:"max_ports"`
	DurationDays   int     `json:"duration_days" binding:"required"`
	PriceMonthly   float64 `json:"price_monthly"`
	PriceQuarterly float64 `json:"price_quarterly"`
	PriceYearly    float64 `json:"price_yearly"`
	Features       string  `json:"features"`
	SortOrder      int     `json:"sort_order"`
}

func (h *PlanHandler) CreatePlan(c *gin.Context) {
	var req CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	plan := model.Plan{
		Name:           req.Name,
		Description:    req.Description,
		MaxProxies:     req.MaxProxies,
		MaxBandwidth:   req.MaxBandwidth,
		MaxTraffic:     req.MaxTraffic,
		MaxPorts:       req.MaxPorts,
		DurationDays:   req.DurationDays,
		PriceMonthly:   req.PriceMonthly,
		PriceQuarterly: req.PriceQuarterly,
		PriceYearly:    req.PriceYearly,
		Features:       req.Features,
		SortOrder:      req.SortOrder,
		Status:         "active",
	}

	if plan.MaxProxies == 0 {
		plan.MaxProxies = 5
	}
	if plan.MaxBandwidth == 0 {
		plan.MaxBandwidth = 10 * 1024 * 1024 // 10MB/s
	}
	if plan.MaxTraffic == 0 {
		plan.MaxTraffic = 100 * 1024 * 1024 * 1024 // 100GB
	}
	if plan.MaxPorts == 0 {
		plan.MaxPorts = 10
	}

	if err := h.db.Create(&plan).Error; err != nil {
		response.InternalError(c, "failed to create plan")
		return
	}

	response.SuccessWithMessage(c, "plan created", plan)
}

func (h *PlanHandler) ListPlans(c *gin.Context) {
	var plans []model.Plan
	h.db.Where("status = ?", "active").Order("sort_order asc, id asc").Find(&plans)
	response.Success(c, plans)
}

func (h *PlanHandler) GetPlan(c *gin.Context) {
	id := c.Param("id")
	var plan model.Plan
	if err := h.db.First(&plan, id).Error; err != nil {
		response.NotFound(c, "plan not found")
		return
	}
	response.Success(c, plan)
}

func (h *PlanHandler) UpdatePlan(c *gin.Context) {
	id := c.Param("id")

	var req CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updates := map[string]interface{}{
		"name":            req.Name,
		"description":     req.Description,
		"max_proxies":     req.MaxProxies,
		"max_bandwidth":   req.MaxBandwidth,
		"max_traffic":     req.MaxTraffic,
		"max_ports":       req.MaxPorts,
		"duration_days":   req.DurationDays,
		"price_monthly":   req.PriceMonthly,
		"price_quarterly": req.PriceQuarterly,
		"price_yearly":    req.PriceYearly,
		"features":        req.Features,
		"sort_order":      req.SortOrder,
	}

	if err := h.db.Model(&model.Plan{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.InternalError(c, "failed to update plan")
		return
	}

	response.SuccessWithMessage(c, "plan updated", nil)
}

func (h *PlanHandler) DeletePlan(c *gin.Context) {
	id := c.Param("id")

	// Check if plan is in use
	var count int64
	h.db.Model(&model.User{}).Where("plan_id = ?", id).Count(&count)
	if count > 0 {
		response.BadRequest(c, "plan is in use by users")
		return
	}

	if err := h.db.Unscoped().Delete(&model.Plan{}, id).Error; err != nil {
		response.InternalError(c, "failed to delete plan")
		return
	}
	response.SuccessWithMessage(c, "plan deleted", nil)
}

func (h *PlanHandler) TogglePlanStatus(c *gin.Context) {
	id := c.Param("id")
	var plan model.Plan
	if err := h.db.First(&plan, id).Error; err != nil {
		response.NotFound(c, "plan not found")
		return
	}

	newStatus := "archived"
	if plan.Status != "active" {
		newStatus = "active"
	}

	h.db.Model(&plan).Update("status", newStatus)
	response.SuccessWithMessage(c, "status updated", gin.H{"status": newStatus})
}

// Admin: List all plans (including archived)
func (h *PlanHandler) AdminListPlans(c *gin.Context) {
	var plans []model.Plan
	h.db.Order("sort_order asc, id asc").Find(&plans)
	response.Success(c, plans)
}
