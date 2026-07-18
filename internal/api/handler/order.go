package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/accesscontrol"
	"github.com/frp-panel/frp-panel/internal/service/billing"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type OrderHandler struct {
	db *gorm.DB
}

var (
	errInsufficientBalance = errors.New("insufficient balance")
	errOrderAlreadyPaid    = errors.New("order already paid")
)

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
	if err := h.db.Where("id = ? AND status = ?", req.PlanID, "active").First(&plan).Error; err != nil {
		response.BadRequest(c, "plan not found")
		return
	}

	var amount float64
	switch req.DurationType {
	case "monthly":
		amount = plan.PriceMonthly
	case "quarterly":
		amount = plan.PriceQuarterly
	case "yearly":
		amount = plan.PriceYearly
	}
	days := planDurationDays(&plan, req.DurationType)

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
		UserID:         userID,
		PlanID:         0,
		OrderNo:        fmt.Sprintf("RCH%s%06d", time.Now().In(beijingTZ).Format("20060102150405"), userID),
		OrderType:      "recharge",
		Amount:         req.Amount,
		OriginalAmount: req.Amount,
		DurationType:   "recharge",
		PayMethod:      req.PayMethod,
		PayStatus:      "pending",
		ExpiresAt:      timePtr(time.Now().In(beijingTZ).Add(30 * time.Minute)),
	}

	if err := h.db.Create(&order).Error; err != nil {
		response.InternalError(c, "failed to create order")
		return
	}

	h.processExternalPayment(c, &order)
}

// processBalancePayment handles balance payment (immediate deduction + plan assignment).
func (h *OrderHandler) processBalancePayment(c *gin.Context, order *model.Order, userID uint) {
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if order.Amount > 0 {
			result := tx.Model(&model.User{}).
				Where("id = ? AND balance >= ?", userID, order.Amount).
				Update("balance", gorm.Expr("balance - ?", order.Amount))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errInsufficientBalance
			}
		}

		processed, err := h.confirmOrderPaymentTx(tx, order, userID, "")
		if err != nil {
			return err
		}
		if !processed {
			return errOrderAlreadyPaid
		}
		return nil
	})
	if errors.Is(err, errInsufficientBalance) {
		h.db.Where("id = ? AND pay_status = ?", order.ID, "pending").Delete(&model.Order{})
		response.BadRequest(c, "insufficient balance")
		return
	}
	if err != nil {
		response.InternalError(c, "failed to process balance payment")
		return
	}
	order.PayStatus = "paid"
	if err := h.db.Preload("Entitlement").First(order, order.ID).Error; err != nil {
		response.InternalError(c, "failed to load paid order")
		return
	}
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

// confirmOrderPayment atomically claims a pending order and applies its side effects.
func (h *OrderHandler) confirmOrderPayment(order *model.Order, userID uint, tradeNo string) (bool, error) {
	processed := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var err error
		processed, err = h.confirmOrderPaymentTx(tx, order, userID, tradeNo)
		return err
	})
	return processed, err
}

func (h *OrderHandler) confirmOrderPaymentTx(tx *gorm.DB, order *model.Order, userID uint, tradeNo string) (bool, error) {
	now := time.Now().In(beijingTZ)
	updates := map[string]interface{}{
		"pay_status": "paid",
		"paid_at":    &now,
	}
	if tradeNo != "" {
		updates["trade_no"] = tradeNo
	}
	claim := tx.Model(&model.Order{}).
		Where("id = ? AND pay_status = ?", order.ID, "pending").
		Updates(updates)
	if claim.Error != nil {
		return false, claim.Error
	}
	if claim.RowsAffected != 1 {
		return false, nil
	}

	// Increment coupon usage
	if order.CouponCode != "" {
		if err := tx.Model(&model.Coupon{}).Where("code = ?", order.CouponCode).
			Update("used_count", gorm.Expr("used_count + 1")).Error; err != nil {
			return false, err
		}
		if err := tx.Model(&model.Coupon{}).Where("code = ? AND creator_type = ?", order.CouponCode, "user").
			Update("refund_status", "used").Error; err != nil {
			return false, err
		}
	}

	// Referral rebate
	if order.Amount > 0 {
		if err := h.processReferralRebate(tx, order, userID); err != nil {
			return false, err
		}
	}

	// Assign plan (only for plan orders)
	if order.OrderType == "plan" && order.PlanID > 0 {
		if err := h.assignPlan(tx, userID, order.ID, order.PlanID, order.DurationType); err != nil {
			return false, err
		}
	}

	// Credit balance (only for recharge orders)
	if order.OrderType == "recharge" {
		result := tx.Model(&model.User{}).Where("id = ?", userID).
			Update("balance", gorm.Expr("balance + ?", order.Amount))
		if result.Error != nil {
			return false, result.Error
		}
		if result.RowsAffected != 1 {
			return false, gorm.ErrRecordNotFound
		}
	}
	return true, nil
}

func (h *OrderHandler) processReferralRebate(tx *gorm.DB, order *model.Order, userID uint) error {
	var referredUser model.User
	if err := tx.Select("id", "invited_by").First(&referredUser, userID).Error; err != nil {
		return err
	}
	if referredUser.InvitedBy == nil {
		return nil
	}

	level1ID := *referredUser.InvitedBy
	settings := h.getSettingsMap(tx)
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
	if err := tx.Select("id", "invited_by").First(&inviter, level1ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if inviter.InvitedBy != nil {
		var level2 model.User
		if err := tx.Select("id").First(&level2, *inviter.InvitedBy).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			level2ID = &level2.ID
			level2Rebate = order.Amount * (level2Pct / 100)
		}
	}

	if err := tx.Model(&model.User{}).Where("id = ?", level1ID).
		Update("balance", gorm.Expr("balance + ?", level1Rebate)).Error; err != nil {
		return err
	}
	if level2ID != nil && level2Rebate > 0 {
		if err := tx.Model(&model.User{}).Where("id = ?", *level2ID).
			Update("balance", gorm.Expr("balance + ?", level2Rebate)).Error; err != nil {
			return err
		}
	}
	return tx.Create(&model.ReferralRebate{
		ReferredUserID: userID,
		OrderID:        order.ID,
		Level1UserID:   level1ID,
		Level2UserID:   level2ID,
		Level1Amount:   level1Rebate,
		Level2Amount:   level2Rebate,
	}).Error
}

func (h *OrderHandler) assignPlan(tx *gorm.DB, userID, orderID, planID uint, durationType string) error {
	var plan model.Plan
	if err := tx.First(&plan, planID).Error; err != nil {
		return err
	}
	days := planDurationDays(&plan, durationType)
	if days <= 0 {
		return fmt.Errorf("invalid plan duration")
	}

	_, err := accesscontrol.GrantPurchasedPlan(tx, userID, orderID, planID, days, time.Now().In(beijingTZ))
	return err
}

func planDurationDays(plan *model.Plan, durationType string) int {
	baseDays := plan.DurationDays
	if baseDays <= 0 {
		baseDays = 30
	}
	switch durationType {
	case "monthly":
		return baseDays
	case "quarterly":
		return baseDays * 3
	case "yearly":
		return baseDays * 12
	default:
		return 0
	}
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
	query.Offset(offset).Limit(parseInt(size)).Order("id desc").Preload("Plan").Preload("Entitlement").Find(&orders)

	response.Page(c, orders, total, parseInt(page), parseInt(size))
}

func (h *OrderHandler) GetOrder(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	orderID := c.Param("id")

	var order model.Order
	if err := h.db.Where("id = ? AND user_id = ?", orderID, userID).Preload("Plan").Preload("Entitlement").First(&order).Error; err != nil {
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
	query.Offset(offset).Limit(parseInt(size)).Order("id desc").Preload("User").Preload("Plan").Preload("Entitlement").Find(&orders)

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

	if order.PayMethod != "balance" {
		response.BadRequest(c, "该订单使用外部支付，面板暂不支持自动原路退款，请先在支付渠道完成退款")
		return
	}
	response.BadRequest(c, "余额支付退款需要同时回收套餐、优惠券和返利权益，当前暂不支持自动退款")
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
	if !authorizeManageableUser(c, &user) {
		return
	}

	now := time.Now().In(beijingTZ)
	order := model.Order{
		UserID:         req.UserID,
		PlanID:         0,
		OrderNo:        fmt.Sprintf("RCH%s%06d", now.Format("20060102150405.000000000"), req.UserID),
		OrderType:      "recharge",
		Amount:         req.Amount,
		OriginalAmount: req.Amount,
		DurationType:   "recharge",
		PayMethod:      "admin",
		PayStatus:      "paid",
		PaidAt:         &now,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		result := tx.Model(&model.User{}).Where("id = ?", req.UserID).
			Update("balance", gorm.Expr("balance + ?", req.Amount))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}); err != nil {
		response.InternalError(c, "failed to recharge balance")
		return
	}
	if err := h.db.First(&user, req.UserID).Error; err != nil {
		response.InternalError(c, "failed to load updated balance")
		return
	}

	response.SuccessWithMessage(c, "balance recharged", gin.H{
		"new_balance": user.Balance,
	})
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func (h *OrderHandler) getSettingsMap(db *gorm.DB) map[string]string {
	var settings []model.Setting
	db.Find(&settings)
	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result
}
