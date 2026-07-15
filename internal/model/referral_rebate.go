package model

import "time"

type ReferralRebate struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ReferredUserID uint      `gorm:"not null;index" json:"referred_user_id"`
	OrderID        uint      `gorm:"not null" json:"order_id"`
	Level1UserID   uint      `gorm:"not null" json:"level1_user_id"`
	Level2UserID   *uint     `json:"level2_user_id"`
	Level1Amount   float64   `gorm:"not null" json:"level1_amount"`
	Level2Amount   float64   `gorm:"not null" json:"level2_amount"`
	CreatedAt      time.Time `json:"created_at"`
}

func (ReferralRebate) TableName() string {
	return "referral_rebates"
}
