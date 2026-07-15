package model

import (
	"time"

	"gorm.io/gorm"
)

type Plan struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	Name           string         `gorm:"size:255;not null" json:"name"`
	Description    string         `gorm:"type:text" json:"description"`
	MaxProxies     int            `gorm:"default:5" json:"max_proxies"`
	MaxBandwidth   int64          `gorm:"default:10485760" json:"max_bandwidth"`  // 10MB/s
	MaxTraffic     int64          `gorm:"default:107374182400" json:"max_traffic"` // 100GB
	MaxPorts       int            `gorm:"default:10" json:"max_ports"`
	DurationDays   int            `gorm:"default:30" json:"duration_days"`
	PriceMonthly   float64        `gorm:"default:0" json:"price_monthly"`
	PriceQuarterly float64        `gorm:"default:0" json:"price_quarterly"`
	PriceYearly    float64        `gorm:"default:0" json:"price_yearly"`
	Features       string         `gorm:"type:text" json:"features"` // JSON
	SortOrder      int            `gorm:"default:0" json:"sort_order"`
	Status         string         `gorm:"size:20;default:active;not null" json:"status"` // active, archived
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Plan) TableName() string {
	return "plans"
}
