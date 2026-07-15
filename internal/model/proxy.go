package model

import (
	"time"

	"gorm.io/gorm"
)

type Proxy struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	UserID          uint           `gorm:"not null;index" json:"user_id"`
	ServerID        uint           `gorm:"not null;index" json:"server_id"`
	Name            string         `gorm:"size:255;not null" json:"name"`
	Type            string         `gorm:"size:20;not null" json:"type"` // tcp, udp, http, https, stcp, xtcp
	LocalIP         string         `gorm:"size:45;default:127.0.0.1" json:"local_ip"`
	LocalPort       int            `gorm:"not null" json:"local_port"`
	RemotePort      int            `json:"remote_port"`               // tcp/udp
	CustomDomains   string         `gorm:"type:text" json:"custom_domains"` // JSON array, http/https
	Subdomain       string         `gorm:"size:255" json:"subdomain"`       // http/https
	SecretKey       string         `gorm:"size:255" json:"secret_key"`      // stcp/xtcp
	UseEncryption   bool           `gorm:"default:false" json:"use_encryption"`
	UseCompression  bool           `gorm:"default:false" json:"use_compression"`
	BandwidthLimit  int64          `gorm:"default:0" json:"bandwidth_limit"`
	Status          string         `gorm:"size:20;default:pending;not null" json:"status"` // pending, running, stopped, error (from frps)
	Enabled         bool           `gorm:"default:false;not null" json:"enabled"`            // user intent: enabled or disabled
	RemoteAddr      string         `gorm:"size:255" json:"remote_addr"`
	ErrorMsg        string         `gorm:"type:text" json:"error_msg"`
	TrafficIn       int64          `gorm:"default:0" json:"traffic_in"`
	TrafficOut      int64          `gorm:"default:0" json:"traffic_out"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	User   User   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Server Server `gorm:"foreignKey:ServerID" json:"server,omitempty"`
}

func (Proxy) TableName() string {
	return "proxies"
}
