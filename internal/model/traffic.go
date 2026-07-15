package model

import (
	"time"
)

type TrafficLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProxyID    uint      `gorm:"not null;index" json:"proxy_id"`
	UserID     uint      `gorm:"not null;index" json:"user_id"`
	ServerID   uint      `gorm:"not null;index" json:"server_id"`
	TrafficIn  int64     `gorm:"default:0" json:"traffic_in"`
	TrafficOut int64     `gorm:"default:0" json:"traffic_out"`
	RecordedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"recorded_at"`
}

func (TrafficLog) TableName() string {
	return "traffic_logs"
}

type TrafficDaily struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	ProxyID    uint   `gorm:"not null;index" json:"proxy_id"`
	UserID     uint   `gorm:"not null;index" json:"user_id"`
	Date       string `gorm:"size:10;not null" json:"date"` // YYYY-MM-DD
	TrafficIn  int64  `gorm:"default:0" json:"traffic_in"`
	TrafficOut int64  `gorm:"default:0" json:"traffic_out"`
}

func (TrafficDaily) TableName() string {
	return "traffic_daily"
}
