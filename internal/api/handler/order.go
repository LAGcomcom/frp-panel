package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/service/billing"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type OrderHandler struct {
	db *gorm.DB
}

func NewOrderHandler(db *gorm.DB) *OrderHandler {
	return &OrderHandler{db: db}
}

type CreateOrderRequest struct {
	PlanID       uint   `json:"plan_id" binding:"required"`
	DurationType string `json:"duration_type" binding:"required,oneof=monthly quarterly yearly"`
	PayMethod    string `json:"pay_method" binding:"required,oneof=balance alipay wechat usdt epay"`
	CouponCode   string `json:"coupon_code"`
}

type CreateRechargeOrderRequest struct {
	Amount    float64 `json:"amount" binding:"required,gt=0"`
	PayMethod string  `json:"pay_method" binding:"required,oneof=alipay wechat usdt epay"`
}

func (h *OrderHandler) CreateOrder(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var plan model.Plan
	if err := h.db.First(&plan, req.PlanID).Error; err != nil {
		response.BadRequest(c, "plan not found")
		return
	}

	var amount float64
	var days int
	switch req.DurationType {
	case "monthly":
		amount = plan.PriceMonthly
		days = 30
	case "quarterly":
		amount = plan.PriceQuarterly
		days = 90
	case "yearly":
		amount = plan.PriceYearly
		days = 365
	}

	if amount <= 0 {
		response.BadRequest(c, "invalid price for selected duration")
		return
	}

	// Apply coupon if provided
	originalAmount := amount
	var discount float64
	if req.CouponCode != "" {
		discount, err := h.validateCoupon(req.CouponCode, userID, req.PlanID, amount)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		amount = amount - discount
		_ = originalAmount // already set above
		_ = discount
	}

	order := model.Order{
		UserID:         userID,
		PlanID:         req.PlanID,
		OrderNo:        fmt.Sprintf("ORD%s%06d", time.Now().In(beijingTZ).Format("20060102150405"), userID),
		OrderType:      "plan",
		Amount:         amount,
		OriginalAmount: originalAmount,
		Discount:       discount,
		CouponCode:     req.CouponCode,
		DurationType:   req.DurationType,
		PayMethod:      req.PayMethod,
		PayStatus:      "pending",
		ExpiresAt:      timePtr(time.Now().In(beijingTZ).AddDate(0, 0, days)),
	}

	if err := h.db.Create(&order).Error; err != nil {
		response.InternalError(c, "failed to create order")
		return
	}

	// Balance payment: immediate
	if req.PayMethod == "balance" {
		h.processBalancePayment(c, &order, userID)
		return
	}

	// External payment: create payment at gateway
	h.processExternalPayment(c, &order)
}

func (h *OrderHandler) CreateRechargeOrder(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req CreateRechargeOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	order := model.Order{
		UserID:       userID,
		PlanID:       0,
		OrderNo:      fmt.Sprintf("RCH%s%06d", time.Now().In(beijingTZ).Format("20060102150405"), userID),
		OrderType:    "recharge",
		Amount:       req.Amount,
		OriginalAmount: req.Amount,
		DurationType: "recharge",
		PayMethod:    req.PayMethod,
		PayStatus:    "pending",
		ExpiresAt:    timePtr(time.Now().In(beijingTZ).Add(30 * time.Minute)),
	}

	if err := h.db.Create(&order).Error; err != nil {
		response.InternalError(c, "failed to create order")
		return
	}

	h.processExternalPayment(c, &order)
}

// processBalancePayment handles balance payment (immediate deduction + plan assignment).
func (h *OrderHandler) processBalancePayment(c *gin.Context, order *model.Order, userID uint) {
	var user model.User
	h.db.First(&user, userID)

	if order.Amount > 0 && user.Balance < order.Amount {
		// Rollback the pending order
		h.db.Delete(order)
		response.BadRequest(c, "insufficient balance")
		return
	}

	if order.Amount > 0 {
		h.db.Model(&user).Update("balance", gorm.Expr("balance - ?", order.Amount))
	}

	h.confirmOrderPayment(order, userID)
	response.SuccessWithMessage(c, "order paid", order)
}

// processExternalPayment creates a payment at the external gateway and returns payment info.
func (h *OrderHandler) processExternalPayment(c *gin.Context, order *model.Order) {
	// Find enabled payment config for the chosen method
	var config model.PaymentConfig
	if err := h.db.Where("type = ? AND enabled = ?", order.PayMethod, true).First(&config).Error; err != nil {
		// Rollback the pending order
		h.db.Delete(order)
		response.BadRequest(c, "payment method not available")
		return
	}

	provider, err := billing.NewProvider(&config)
	if err != nil {
		h.db.Delete(order)
		response.InternalError(c, "payment provider init failed")
		return
	}

	subject := fmt.Sprintf("Order %s", order.OrderNo)
	if order.OrderType == "plan" {
		var plan model.Plan
		if h.db.First(&plan, order.PlanID).Error == nil {
			subject = plan.Name
		}
	} else {
		subject = "Balance Recharge"
	}

	result, err := provider.CreatePayment(&billing.CreatePaymentRequest{
		OrderNo: order.OrderNo,
		Amount:  order.Amount,
		Subject: subject,
	})
	if err != nil {
		h.db.Delete(order)
		response.InternalError(c, "failed to create payment: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"order_id": order.ID,
		"order_no": order.OrderNo,
		"pay_url":  result.PayURL,
		"qr_code":  result.QRCode,
	})
}

// confirmOrderPayment marks order as paid and processes post-payment logic.
func (h *OrderHandler) confirmOrderPayment(order *model.Order, userID uint) {
	now := time.Now().In(beijingTZ)
	h.db.Model(order).Updates(map[string]interface{}{
		"pay_status": "paid",
		"paid_at":    &now,
	})

	// Increment coupon usage
	if order.CouponCode != "" {
		h.db.Model(&model.Coupon{}).Where("code = ?", order.CouponCode).Update("used_count", gorm.Expr("used_count + 1"))
		h.db.Model(&model.Coupon{}).Where("code = ? AND creator_type = ?", order.CouponCode, "user").Update("refund_status", "used")
	}

	// Referral rebate
	if order.Amount > 0 {
		h.processReferralRebate(order, userID)
	}

	// Assign plan (only for plan orders)
	if order.OrderType == "plan" && order.PlanID > 0 {
		h.assignPlan(userID, order.PlanID, order.DurationType, order.ExpiresAt)
	}

	// Credit balance (only for recharge orders)
	if order.OrderType == "recharge" {
		h.db.Model(&model.User{}).Where("id = ?", userID).Update("balance", gorm.Expr("balance + ?", order.Amount))
	}
}

func (h *OrderHandler) processReferralRebate(order *model.Order, userID uint) {
	var referredUser model.User
	h.db.Select("id", "invited_by").First(&referredUser, userID)
	if referredUser.InvitedBy == nil {
		return
	}

	level1ID := *referredUser.InvitedBy
	settings := h.getSettingsMap()
	level1Pct := 10.0
	level2Pct := 5.0
	if v := settings["invite_rebate_level1_percent"]; v != "" {
		fmt.Sscanf(v, "%f", &level1Pct)
	}
	if v := settings["invite_rebate_level2_percent"]; v != "" {
		fmt.Sscanf(v, "%f", &level2Pct)
	}

	level1Rebate := order.Amount * (level1Pct / 100)
	var level2Rebate float64
	var level2ID *uint

	var inviter model.User
	h.db.Select("id", "invited_by").First(&inviter, level1ID)
	if inviter.InvitedBy != nil {
		level2ID = inviter.InvitedBy
		level2Rebate = order.Amount * (level2Pct / 100)
	}

	h.db.Transaction(func(tx *gorm.DB) error {
		tx.Model(&model.User{}).Where("id = ?", level1ID).Update("balance", gorm.Expr("balance + ?", level1Rebate))
		if level2ID != nil && level2Rebate > 0 {
			tx.Model(&model.User{}).Where("id = ?", *level2ID).Update("balance", gorm.Expr("balance + ?", level2Rebate))
		}
		tx.Create(&model.ReferralRebate{
			ReferredUserID: userID,
			OrderID:        order.ID,
			Level1UserID:   level1ID,
			Level2UserID:   level2ID,
			Level1Amount:   level1Rebate,
			Level2Amount:   level2Rebate,
		})
		return nil
	})
}

func (h *OrderHandler) assignPlan(userID uint, planID uint, durationType string, expiresAt *time.Time) {
	var days int
	switch durationType {
	case "monthly":
		days = 30
	case "quarterly":
		days = 90
	case "yearly":
		days = 365
	default:
		return
	}

	var existingUser model.User
	h.db.First(&existingUser, userID)
	var newExpiresAt time.Time
	if existingUser.PlanID != nil && *existingUser.PlanID == planID &&
		existingUser.PlanExpiresAt != nil && existingUser.PlanExpiresAt.After(time.Now().In(beijingTZ)) {
		newExpiresAt = existingUser.PlanExpiresAt.AddDate(0, 0, days)
	} else if expiresAt != nil {
		newExpiresAt = *expiresAt
	} else {
		newExpiresAt = time.Now().In(beijingTZ).AddDate(0, 0, days)
	}

	h.db.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"plan_id":         planID,
		"plan_expires_at": newExpiresAt,
	})
}

// validateCoupon validates a coupon and returns the discount amount.
func (h *OrderHandler) validateCoupon(couponCode string, userID uint, planID uint, amount float64) (float64, error) {
	var coupon model.Coupon
	if err := h.db.Where("code = ? AND status = ?", couponCode, "active").First(&coupon).Error; err != nil {
		return 0, fmt.Errorf("invalid coupon code")
	}

	now := time.Now().In(beijingTZ)
	if coupon.StartTime != nil && now.Before(*coupon.StartTime) {
		return 0, fmt.Errorf("coupon is not yet valid")
	}
	if coupon.EndTime != nil && now.After(*coupon.EndTime) {
		return 0, fmt.Errorf("coupon has expired")
	}
	if coupon.MaxUses > 0 && coupon.UsedCount >= coupon.MaxUses {
		return 0, fmt.Errorf("coupon usage limit reached")
	}

	if coupon.PlanIDs != "" {
		var planIDs []uint
		json.Unmarshal([]byte(coupon.PlanIDs), &planIDs)
		applicable := false
		for _, pid := range planIDs {
			if pid == planID {
				applicable = true
				break
			}
		}
		if !applicable {
			return 0, fmt.Errorf("coupon is not applicable to this plan")
		}
	}

	if coupon.AssignedTo != nil && *coupon.AssignedTo != userID {
		return 0, fmt.Errorf("此优惠码不属于您")
	}

	var discount float64
	if coupon.DiscountType == "percent" {
		discount = amount * (coupon.DiscountValue / 100)
	} else {
		discount = coupon.DiscountValue
	}
	if discount > amount {
		discount = amount
	}
	return discount, nil
}

func (h *OrderHandler) ListOrders(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	page := c.DefaultQuery("page", "1")
	size := c.DefaultQuery("size", "20")

	var orders []model.Order
	var total int64

	query := h.db.Model(&model.Order{}).Where("user_id = ?", userID)
	query.Count(&total)

	offset := (parseInt(page) - 1) * parseInt(size)
	query.Offset(offset).Limit(parseInt(size)).Order("id desc").Preload("Plan").Find(&orders)

	response.Page(c, orders, total, parseInt(page), parseInt(size))
}

func (h *OrderHandler) GetOrder(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	orderID := c.Param("id")

	var order model.Order
	if err := h.db.Where("id = ? AND user_id = ?", orderID, userID).Preload("Plan").First(&order).Error; err != nil {
		response.NotFound(c, "order not found")
		return
	}

	response.Success(c, order)
}

// Admin: List all orders
func (h *OrderHandler) AdminListOrders(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	size := c.DefaultQuery("size", "20")
	userID := c.DefaultQuery("user_id", "")
	status := c.DefaultQuery("status", "")

	var orders []model.Order
	var total int64

	query := h.db.Model(&model.Order{})
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if status != "" {
		query = query.Where("pay_status = ?", status)
	}

	query.Count(&total)

	offset := (parseInt(page) - 1) * parseInt(size)
	query.Offset(offset).Limit(parseInt(size)).Order("id desc").Preload("User").Preload("Plan").Find(&orders)

	response.Page(c, orders, total, parseInt(page), parseInt(size))
}

// Admin: Refund order
func (h *OrderHandler) RefundOrder(c *gin.Context) {
	orderID := c.Param("id")

	var order model.Order
	if err := h.db.First(&order, orderID).Error; err != nil {
		response.NotFound(c, "order not found")
		return
	}

	if order.PayStatus != "paid" {
		response.BadRequest(c, "only paid orders can be refunded")
		return
	}

	// Refund to balance
	h.db.Model(&model.User{}).Where("id = ?", order.UserID).Update("balance", gorm.Expr("balance + ?", order.Amount))

	h.db.Model(&order).Update("pay_status", "refunded")
	response.SuccessWithMessage(c, "order refunded", nil)
}

// RechargeBalance adds balance to user account (admin only)
func (h *OrderHandler) RechargeBalance(c *gin.Context) {
	var req struct {
		UserID uint    `json:"user_id" binding:"required"`
		Amount float64 `json:"amount" binding:"required,gt=0"`
		Remark string  `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var user model.User
	if err := h.db.First(&user, req.UserID).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	if err := h.db.Model(&user).Update("balance", gorm.Expr("balance + ?", req.Amount)).Error; err != nil {
		response.InternalError(c, "failed to recharge balance")
		return
	}

	// Create a record order
	order := model.Order{
		UserID:       req.UserID,
		PlanID:       0,
		OrderNo:      fmt.Sprintf("RCH%s%06d", time.Now().In(beijingTZ).Format("20060102150405"), req.UserID),
		OrderType:    "recharge",
		Amount:       req.Amount,
		DurationType: "recharge",
		PayMethod:    "admin",
		PayStatus:    "paid",
	}
	h.db.Create(&order)

	response.SuccessWithMessage(c, "balance recharged", gin.H{
		"new_balance": user.Balance + req.Amount,
	})
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func (h *OrderHandler) getSettingsMap() map[string]string {
	var settings []model.Setting
	h.db.Find(&settings)
	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result
}
