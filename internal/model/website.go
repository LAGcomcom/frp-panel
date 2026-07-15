package model

import (
	"time"

	"gorm.io/gorm"
)

type Website struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	Name          string         `gorm:"size:255;not null" json:"name"`
	Domain        string         `gorm:"size:255;not null" json:"domain"`
	Subdomain     string         `gorm:"size:255" json:"subdomain"`
	Type          string         `gorm:"size:20;not null" json:"type"` // http, https
	ServerID      uint           `gorm:"not null;index" json:"server_id"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	ProxyID       uint           `gorm:"index" json:"proxy_id"`
	BackendAddr   string         `gorm:"size:255;not null" json:"backend_addr"` // ip:port
	SSLStatus     string         `gorm:"size:20;default:none" json:"ssl_status"` // none, active, expired, error
	SSLExpiry     *time.Time     `json:"ssl_expiry"`
	Status        string         `gorm:"size:20;default:pending;not null" json:"status"` // pending, running, stopped, error
	MonthlyVisits int64          `gorm:"default:0" json:"monthly_visits"`
	TrafficIn     int64          `gorm:"default:0" json:"traffic_in"`
	TrafficOut    int64          `gorm:"default:0" json:"traffic_out"`
	ResponseTime  int            `gorm:"default:0" json:"response_time"` // ms
	ErrorMsg      string         `gorm:"type:text" json:"error_msg"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	Server *Server `gorm:"foreignKey:ServerID" json:"server,omitempty"`
	User   *User   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Proxy  *Proxy  `gorm:"foreignKey:ProxyID" json:"proxy,omitempty"`
}

func (Website) TableName() string {
	return "websites"
}
