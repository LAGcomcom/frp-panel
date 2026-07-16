package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	Email          string         `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Password       string         `gorm:"size:255;not null" json:"-"`
	Role           string         `gorm:"size:20;default:user;not null" json:"role"` // super_admin, admin, user
	Balance        float64        `gorm:"default:0" json:"balance"`
	InviteCode     string         `gorm:"uniqueIndex;size:32" json:"invite_code"`
	InvitedBy      *uint          `json:"invited_by"`
	PlanID         *uint          `json:"plan_id"`
	PlanExpiresAt  *time.Time     `json:"plan_expires_at"`
	GroupID        *uint          `gorm:"index" json:"group_id"`
	GroupSource    string         `gorm:"size:20" json:"group_source"`
	BandwidthLimit int64          `gorm:"default:0" json:"bandwidth_limit"`
	Status         string         `gorm:"size:20;default:active;not null" json:"status"` // active, banned, pending
	APIKey         string         `gorm:"uniqueIndex;size:64" json:"api_key"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	Plan  *Plan      `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
	Group *UserGroup `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

func (User) TableName() string {
	return "users"
}
