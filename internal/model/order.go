package model

import (
	"time"

	"gorm.io/gorm"
)

type Order struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	UserID         uint           `gorm:"not null;index" json:"user_id"`
	PlanID         uint           `gorm:"default:0" json:"plan_id"`
	OrderNo        string         `gorm:"uniqueIndex;size:64;not null" json:"order_no"`
	OrderType      string         `gorm:"size:20;default:plan;not null" json:"order_type"` // plan, recharge
	Amount         float64        `gorm:"not null" json:"amount"`
	OriginalAmount float64        `gorm:"default:0" json:"original_amount"`
	Discount       float64        `gorm:"default:0" json:"discount"`
	CouponCode     string         `gorm:"size:64" json:"coupon_code"`
	DurationType   string         `gorm:"size:20" json:"duration_type"`                   // monthly, quarterly, yearly, recharge
	PayMethod      string         `gorm:"size:20" json:"pay_method"`                      // alipay, wechat, usdt, epay, balance, admin
	PayStatus      string         `gorm:"size:20;default:pending;not null" json:"pay_status"` // pending, paid, refunded, expired
	TradeNo        string         `gorm:"size:255" json:"trade_no"`
	PaidAt         *time.Time     `json:"paid_at"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Plan Plan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
}

func (Order) TableName() string {
	return "orders"
}
