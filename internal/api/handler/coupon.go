package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/hash"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var beijingTZ = time.FixedZone("CST", 8*3600)

type CouponHandler struct {
	db *gorm.DB
}

func NewCouponHandler(db *gorm.DB) *CouponHandler {
	return &CouponHandler{db: db}
}

type CreateCouponRequest struct {
	Code          string  `json:"code" binding:"required"`
	DiscountType  string  `json:"discount_type" binding:"required,oneof=percent fixed"`
	DiscountValue float64 `json:"discount_value" binding:"required"`
	MaxUses       int     `json:"max_uses"`
	PlanIDs       []uint  `json:"plan_ids"`
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time"`
}

func (h *CouponHandler) ListCoupons(c *gin.Context) {
	var coupons []model.Coupon
	h.db.Order("id desc").Find(&coupons)
	response.Success(c, coupons)
}

func (h *CouponHandler) CreateCoupon(c *gin.Context) {
	var req CreateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Check code uniqueness
	var count int64
	h.db.Model(&model.Coupon{}).Where("code = ?", req.Code).Count(&count)
	if count > 0 {
		response.BadRequest(c, "coupon code already exists")
		return
	}

	// Validate discount value
	if req.DiscountType == "percent" && (req.DiscountValue <= 0 || req.DiscountValue > 100) {
		response.BadRequest(c, "percent discount must be between 0 and 100")
		return
	}
	if req.DiscountType == "fixed" && req.DiscountValue <= 0 {
		response.BadRequest(c, "fixed discount must be greater than 0")
		return
	}

	// Parse times
	var startTime, endTime *time.Time
	if req.StartTime != "" {
		t, err := time.ParseInLocation("2006-01-02", req.StartTime, beijingTZ)
		if err != nil {
			response.BadRequest(c, "invalid start_time format, use YYYY-MM-DD")
			return
		}
		startTime = &t
	}
	if req.EndTime != "" {
		t, err := time.ParseInLocation("2006-01-02", req.EndTime, beijingTZ)
		if err != nil {
			response.BadRequest(c, "invalid end_time format, use YYYY-MM-DD")
			return
		}
		// End of day: add 23:59:59
		end := t.Add(24*time.Hour - time.Second)
		endTime = &end
	}

	// Serialize plan_ids
	planIDsJSON := ""
	if len(req.PlanIDs) > 0 {
		b, _ := json.Marshal(req.PlanIDs)
		planIDsJSON = string(b)
	}

	coupon := model.Coupon{
		Code:          req.Code,
		DiscountType:  req.DiscountType,
		DiscountValue: req.DiscountValue,
		MaxUses:       req.MaxUses,
		PlanIDs:       planIDsJSON,
		StartTime:     startTime,
		EndTime:       endTime,
		Status:        "active",
	}

	if err := h.db.Create(&coupon).Error; err != nil {
		response.InternalError(c, "failed to create coupon")
		return
	}

	response.SuccessWithMessage(c, "coupon created", coupon)
}

func (h *CouponHandler) UpdateCoupon(c *gin.Context) {
	id := c.Param("id")

	var coupon model.Coupon
	if err := h.db.First(&coupon, id).Error; err != nil {
		response.NotFound(c, "coupon not found")
		return
	}

	var req struct {
		Code          string  `json:"code"`
		DiscountType  string  `json:"discount_type"`
		DiscountValue float64 `json:"discount_value"`
		MaxUses       int     `json:"max_uses"`
		PlanIDs       []uint  `json:"plan_ids"`
		StartTime     string  `json:"start_time"`
		EndTime       string  `json:"end_time"`
		Status        string  `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Code != "" {
		// Check uniqueness
		var count int64
		h.db.Model(&model.Coupon{}).Where("code = ? AND id != ?", req.Code, id).Count(&count)
		if count > 0 {
			response.BadRequest(c, "coupon code already exists")
			return
		}
		updates["code"] = req.Code
	}
	if req.DiscountType != "" {
		updates["discount_type"] = req.DiscountType
	}
	if req.DiscountValue > 0 {
		updates["discount_value"] = req.DiscountValue
	}
	if req.MaxUses >= 0 {
		updates["max_uses"] = req.MaxUses
	}
	if req.PlanIDs != nil {
		b, _ := json.Marshal(req.PlanIDs)
		updates["plan_ids"] = string(b)
	}
	if req.StartTime != "" {
		t, err := time.ParseInLocation("2006-01-02", req.StartTime, beijingTZ)
		if err != nil {
			response.BadRequest(c, "invalid start_time format")
			return
		}
		updates["start_time"] = t
	}
	if req.EndTime != "" {
		t, err := time.ParseInLocation("2006-01-02", req.EndTime, beijingTZ)
		if err != nil {
			response.BadRequest(c, "invalid end_time format")
			return
		}
		end := t.Add(24*time.Hour - time.Second)
		updates["end_time"] = end
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}

	if len(updates) == 0 {
		response.BadRequest(c, "no fields to update")
		return
	}

	if err := h.db.Model(&coupon).Updates(updates).Error; err != nil {
		response.InternalError(c, "failed to update coupon")
		return
	}

	response.SuccessWithMessage(c, "coupon updated", nil)
}

func (h *CouponHandler) DeleteCoupon(c *gin.Context) {
	id := c.Param("id")

	if err := h.db.Delete(&model.Coupon{}, id).Error; err != nil {
		response.InternalError(c, "failed to delete coupon")
		return
	}

	response.SuccessWithMessage(c, "coupon deleted", nil)
}

// VerifyCoupon checks if a coupon is valid and returns discount info
func (h *CouponHandler) VerifyCoupon(c *gin.Context) {
	code := c.Query("code")
	planID := c.Query("plan_id")
	durationType := c.DefaultQuery("duration_type", "monthly")

	if code == "" || planID == "" {
		response.BadRequest(c, "code and plan_id are required")
		return
	}

	var coupon model.Coupon
	if err := h.db.Where("code = ? AND status = ?", code, "active").First(&coupon).Error; err != nil {
		response.BadRequest(c, "优惠码无效")
		return
	}

	// Check time validity
	now := time.Now().In(beijingTZ)
	if coupon.StartTime != nil && now.Before(*coupon.StartTime) {
		response.BadRequest(c, "优惠码尚未生效")
		return
	}
	if coupon.EndTime != nil && now.After(*coupon.EndTime) {
		response.BadRequest(c, "优惠码已过期")
		return
	}

	// Check usage limit
	if coupon.MaxUses > 0 && coupon.UsedCount >= coupon.MaxUses {
		response.BadRequest(c, "优惠码已达使用上限")
		return
	}

	// Check plan applicability
	var planIDUint uint
	fmt.Sscanf(planID, "%d", &planIDUint)
	if coupon.PlanIDs != "" {
		var planIDs []uint
		json.Unmarshal([]byte(coupon.PlanIDs), &planIDs)
		applicable := false
		for _, pid := range planIDs {
			if pid == planIDUint {
				applicable = true
				break
			}
		}
		if !applicable {
			response.BadRequest(c, "优惠码不适用于此套餐")
			return
		}
	}

	// Get plan price
	var plan model.Plan
	if err := h.db.First(&plan, planIDUint).Error; err != nil {
		response.BadRequest(c, "plan not found")
		return
	}

	var originalPrice float64
	switch durationType {
	case "monthly":
		originalPrice = plan.PriceMonthly
	case "quarterly":
		originalPrice = plan.PriceQuarterly
	case "yearly":
		originalPrice = plan.PriceYearly
	default:
		originalPrice = plan.PriceMonthly
	}

	// Calculate discount
	var discount float64
	if coupon.DiscountType == "percent" {
		discount = originalPrice * (coupon.DiscountValue / 100)
	} else {
		discount = coupon.DiscountValue
	}
	if discount > originalPrice {
		discount = originalPrice
	}

	response.Success(c, gin.H{
		"code":           coupon.Code,
		"discount_type":  coupon.DiscountType,
		"discount_value": coupon.DiscountValue,
		"original_price": originalPrice,
		"discount":       discount,
		"final_price":    originalPrice - discount,
	})
}

func (h *CouponHandler) ToggleCoupon(c *gin.Context) {
	id := c.Param("id")

	var coupon model.Coupon
	if err := h.db.First(&coupon, id).Error; err != nil {
		response.NotFound(c, "coupon not found")
		return
	}

	newStatus := "active"
	if coupon.Status == "active" {
		newStatus = "disabled"
	}

	h.db.Model(&coupon).Update("status", newStatus)
	response.SuccessWithMessage(c, "coupon status updated", gin.H{"status": newStatus})
}

// --- User coupon functions ---

type CreateUserCouponRequest struct {
	AssignedTo uint    `json:"assigned_to" binding:"required"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	EndTime    string  `json:"end_time" binding:"required"` // YYYY-MM-DD
}

func (h *CouponHandler) CreateUserCoupon(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req CreateUserCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Verify target is a direct referral
	var target model.User
	if err := h.db.Where("id = ? AND invited_by = ?", req.AssignedTo, userID).First(&target).Error; err != nil {
		response.BadRequest(c, "只能给自己的直推下级发优惠券")
		return
	}

	// Check creator balance
	var creator model.User
	h.db.First(&creator, userID)
	if creator.Balance < req.Amount {
		response.BadRequest(c, "余额不足")
		return
	}

	// Parse end time
	t, err := time.ParseInLocation("2006-01-02", req.EndTime, beijingTZ)
	if err != nil {
		response.BadRequest(c, "日期格式错误，请使用 YYYY-MM-DD")
		return
	}
	endTime := t.Add(24*time.Hour - time.Second)

	// Generate coupon code
	code := fmt.Sprintf("REF%s", hash.RandomString(8))

	// Transaction: deduct balance + create coupon
	err = h.db.Transaction(func(tx *gorm.DB) error {
		// Deduct balance with race condition guard
		result := tx.Model(&model.User{}).Where("id = ? AND balance >= ?", userID, req.Amount).
			Update("balance", gorm.Expr("balance - ?", req.Amount))
		if result.RowsAffected == 0 {
			return fmt.Errorf("余额不足")
		}

		coupon := model.Coupon{
			Code:           code,
			DiscountType:   "fixed",
			DiscountValue:  req.Amount,
			MaxUses:        1,
			Status:         "active",
			EndTime:        &endTime,
			CreatorType:    "user",
			CreatedBy:      &userID,
			AssignedTo:     &req.AssignedTo,
			DeductedAmount: req.Amount,
			RefundStatus:   "none",
		}
		return tx.Create(&coupon).Error
	})

	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "优惠券已创建", gin.H{
		"code":   code,
		"amount": req.Amount,
	})
}

func (h *CouponHandler) ListMyCoupons(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	// On-access refund check for expired user coupons
	refundExpiredCoupons(h.db)

	var coupons []model.Coupon
	h.db.Where("created_by = ?", userID).Order("id desc").Find(&coupons)
	response.Success(c, coupons)
}

func (h *CouponHandler) ListMyAvailableCoupons(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	// On-access refund check
	refundExpiredCoupons(h.db)

	now := time.Now().In(beijingTZ)
	var coupons []model.Coupon
	h.db.Where("assigned_to = ? AND status = ? AND refund_status = ? AND (end_time IS NULL OR end_time > ?)",
		userID, "active", "none", now).Order("id desc").Find(&coupons)
	response.Success(c, coupons)
}

func refundExpiredCoupons(db *gorm.DB) {
	now := time.Now().In(beijingTZ)
	var coupons []model.Coupon
	db.Where("creator_type = ? AND refund_status = ? AND end_time IS NOT NULL AND end_time < ?",
		"user", "none", now).Find(&coupons)

	for _, coupon := range coupons {
		if coupon.CreatedBy == nil || coupon.DeductedAmount <= 0 {
			continue
		}
		db.Transaction(func(tx *gorm.DB) error {
			tx.Model(&model.User{}).Where("id = ?", *coupon.CreatedBy).
				Update("balance", gorm.Expr("balance + ?", coupon.DeductedAmount))
			tx.Model(&model.Coupon{}).Where("id = ?", coupon.ID).
				Update("refund_status", "refunded")
			return nil
		})
	}
}
