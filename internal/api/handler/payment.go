package handler

import (
	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/service/billing"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PaymentHandler struct {
	db           *gorm.DB
	orderHandler *OrderHandler
}

func NewPaymentHandler(db *gorm.DB, orderHandler *OrderHandler) *PaymentHandler {
	return &PaymentHandler{db: db, orderHandler: orderHandler}
}

var validPayTypes = map[string]bool{
	"alipay": true,
	"usdt":   true,
	"epay":   true,
}

// GetEnabledPaymentMethods returns enabled payment configs for user-facing display.
func (h *PaymentHandler) GetEnabledPaymentMethods(c *gin.Context) {
	var configs []model.PaymentConfig
	h.db.Where("enabled = ?", true).Order("sort_order asc, id asc").Find(&configs)

	methods := make([]billing.SafePaymentMethod, 0, len(configs))
	for _, cfg := range configs {
		methods = append(methods, billing.SafePaymentMethod{
			ID:        cfg.ID,
			Name:      cfg.Name,
			Type:      cfg.Type,
			SortOrder: cfg.SortOrder,
		})
	}
	response.Success(c, methods)
}

// PayNotify handles payment gateway callbacks for all types.
func (h *PaymentHandler) PayNotify(c *gin.Context) {
	payType := c.Param("type")
	if !validPayTypes[payType] {
		c.String(400, "invalid payment type")
		return
	}

	// Collect all callback parameters
	params := make(map[string]string)
	if c.Request.Method == "GET" {
		for k, v := range c.Request.URL.Query() {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	} else {
		c.Request.ParseForm()
		for k, v := range c.Request.PostForm {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
		// Also check query params for gateways that mix methods
		for k, v := range c.Request.URL.Query() {
			if _, exists := params[k]; !exists && len(v) > 0 {
				params[k] = v[0]
			}
		}
	}

	// Find the payment config for this type
	var config model.PaymentConfig
	if err := h.db.Where("type = ? AND enabled = ?", payType, true).First(&config).Error; err != nil {
		c.String(500, "payment config not found")
		return
	}

	provider, err := billing.NewProvider(&config)
	if err != nil {
		c.String(500, "provider init failed")
		return
	}

	result, err := provider.VerifyCallback(params)
	if err != nil || !result.Verified {
		c.String(400, "verification failed")
		return
	}

	if result.OrderNo == "" {
		c.String(200, "success")
		return
	}

	// Find the order
	var order model.Order
	if err := h.db.Where("order_no = ?", result.OrderNo).First(&order).Error; err != nil {
		c.String(200, "success")
		return
	}

	// Idempotency: skip if already paid
	if order.PayStatus == "paid" {
		c.String(200, "success")
		return
	}

	// Only process pending orders
	if order.PayStatus != "pending" {
		c.String(200, "success")
		return
	}

	// Verify amount matches
	if result.Amount > 0 && result.Amount != order.Amount {
		c.String(400, "amount mismatch")
		return
	}

	// Claiming the pending order and applying all entitlements happens in one
	// transaction, so concurrent duplicate callbacks cannot credit twice.
	if _, err := h.orderHandler.confirmOrderPayment(&order, order.UserID, result.TradeNo); err != nil {
		c.String(500, "payment confirmation failed")
		return
	}

	c.String(200, "success")
}

// --- Admin CRUD ---

func (h *PaymentHandler) ListPaymentConfigs(c *gin.Context) {
	var configs []model.PaymentConfig
	h.db.Order("sort_order asc, id asc").Find(&configs)
	response.Success(c, configs)
}

type CreatePaymentConfigRequest struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type" binding:"required"`
	Config    string `json:"config"`
	Enabled   *bool  `json:"enabled"`
	SortOrder *int   `json:"sort_order"`
}

func (h *PaymentHandler) CreatePaymentConfig(c *gin.Context) {
	var req CreatePaymentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if !validPayTypes[req.Type] {
		response.BadRequest(c, "invalid payment type, must be one of: alipay, usdt, epay")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	config := model.PaymentConfig{
		Name:      req.Name,
		Type:      req.Type,
		Config:    req.Config,
		Enabled:   enabled,
		SortOrder: sortOrder,
	}

	if err := h.db.Create(&config).Error; err != nil {
		response.InternalError(c, "failed to create payment config")
		return
	}

	response.SuccessWithMessage(c, "创建成功", config)
}

func (h *PaymentHandler) GetPaymentConfig(c *gin.Context) {
	id := c.Param("id")

	var config model.PaymentConfig
	if err := h.db.First(&config, id).Error; err != nil {
		response.NotFound(c, "payment config not found")
		return
	}

	response.Success(c, config)
}

type UpdatePaymentConfigRequest struct {
	Name      *string `json:"name"`
	Type      *string `json:"type"`
	Config    *string `json:"config"`
	Enabled   *bool   `json:"enabled"`
	SortOrder *int    `json:"sort_order"`
}

func (h *PaymentHandler) UpdatePaymentConfig(c *gin.Context) {
	id := c.Param("id")

	var config model.PaymentConfig
	if err := h.db.First(&config, id).Error; err != nil {
		response.NotFound(c, "payment config not found")
		return
	}

	var req UpdatePaymentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Type != nil && !validPayTypes[*req.Type] {
		response.BadRequest(c, "invalid payment type")
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Type != nil {
		updates["type"] = *req.Type
	}
	if req.Config != nil {
		updates["config"] = *req.Config
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}

	if err := h.db.Model(&config).Updates(updates).Error; err != nil {
		response.InternalError(c, "更新失败")
		return
	}

	h.db.First(&config, config.ID)
	response.SuccessWithMessage(c, "更新成功", config)
}

func (h *PaymentHandler) DeletePaymentConfig(c *gin.Context) {
	id := c.Param("id")

	var config model.PaymentConfig
	if err := h.db.First(&config, id).Error; err != nil {
		response.NotFound(c, "payment config not found")
		return
	}

	if err := h.db.Delete(&config).Error; err != nil {
		response.InternalError(c, "删除失败")
		return
	}

	response.SuccessWithMessage(c, "删除成功", nil)
}

func (h *PaymentHandler) TogglePaymentConfig(c *gin.Context) {
	id := c.Param("id")

	var config model.PaymentConfig
	if err := h.db.First(&config, id).Error; err != nil {
		response.NotFound(c, "payment config not found")
		return
	}

	newState := !config.Enabled
	if err := h.db.Model(&config).Update("enabled", newState).Error; err != nil {
		response.InternalError(c, "操作失败")
		return
	}

	msg := "已禁用"
	if newState {
		msg = "已启用"
	}
	response.SuccessWithMessage(c, msg, gin.H{"enabled": newState})
}
