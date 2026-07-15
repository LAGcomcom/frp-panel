package model

import (
	"time"

	"gorm.io/gorm"
)

type Coupon struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	Code           string         `gorm:"uniqueIndex;size:64;not null" json:"code"`
	DiscountType   string         `gorm:"size:20;not null" json:"discount_type"` // percent, fixed
	DiscountValue  float64        `gorm:"not null" json:"discount_value"`        // percent: 80=8折, fixed: 减免金额
	MaxUses        int            `gorm:"default:0" json:"max_uses"`             // 0=不限
	UsedCount      int            `gorm:"default:0" json:"used_count"`
	PlanIDs        string         `gorm:"type:text" json:"plan_ids"`             // JSON array, empty=all plans
	StartTime      *time.Time     `json:"start_time"`
	EndTime        *time.Time     `json:"end_time"`
	Status         string         `gorm:"size:20;default:active;not null" json:"status"` // active, disabled
	CreatorType    string         `gorm:"size:20;default:admin;not null" json:"creator_type"` // admin, user
	CreatedBy      *uint          `gorm:"index" json:"created_by"`
	AssignedTo     *uint          `gorm:"index" json:"assigned_to"`
	DeductedAmount float64        `gorm:"default:0" json:"deducted_amount"`
	RefundStatus   string         `gorm:"size:20;default:none;not null" json:"refund_status"` // none, refunded, used
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Coupon) TableName() string {
	return "coupons"
}
