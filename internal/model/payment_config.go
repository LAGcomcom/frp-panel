package model

import (
	"time"

	"gorm.io/gorm"
)

type PaymentConfig struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"size:100;not null" json:"name"`
	Type      string         `gorm:"size:20;not null" json:"type"` // alipay, wechat, usdt, epay
	Config    string         `gorm:"type:text" json:"config"`      // JSON config
	Enabled   bool           `gorm:"default:true" json:"enabled"`
	SortOrder int            `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (PaymentConfig) TableName() string {
	return "payment_configs"
}
