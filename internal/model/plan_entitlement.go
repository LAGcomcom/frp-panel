package model

import "time"

const (
	PlanEntitlementActive   = "active"
	PlanEntitlementExtended = "extended"
	PlanEntitlementQueued   = "queued"
	PlanEntitlementExpired  = "expired"
)

// PlanEntitlement records how one paid order is delivered without overwriting
// another paid period.
type PlanEntitlement struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserID       uint       `gorm:"not null;index" json:"user_id"`
	PlanID       uint       `gorm:"not null;index" json:"plan_id"`
	OrderID      uint       `gorm:"not null;uniqueIndex" json:"order_id"`
	DurationDays int        `gorm:"not null" json:"duration_days"`
	Status       string     `gorm:"size:20;not null;index" json:"status"`
	StartsAt     *time.Time `json:"starts_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	ActivatedAt  *time.Time `json:"activated_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	Plan Plan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
}

func (PlanEntitlement) TableName() string {
	return "plan_entitlements"
}
