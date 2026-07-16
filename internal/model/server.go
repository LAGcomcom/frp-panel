package model

import (
	"time"

	"gorm.io/gorm"
)

type Server struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	Name              string         `gorm:"size:255;not null" json:"name"`
	IP                string         `gorm:"size:45;not null" json:"ip"`
	SSHPort           int            `gorm:"default:22" json:"ssh_port"`
	SSHUser           string         `gorm:"size:64;default:root" json:"ssh_user"`
	SSHAuthType       string         `gorm:"size:20" json:"ssh_auth_type"` // password, key
	SSHPassword       string         `gorm:"size:255" json:"-"`
	SSHPrivateKey     string         `gorm:"type:text" json:"-"`
	FrpVersion        string         `gorm:"size:20" json:"frp_version"`
	BindPort          int            `gorm:"default:7000" json:"bind_port"`
	DashboardPort     int            `json:"dashboard_port"`
	DashboardUser     string         `gorm:"size:64" json:"dashboard_user"`
	DashboardPassword string         `gorm:"size:255" json:"-"`
	VhostHTTPPort     int            `json:"vhost_http_port"`
	VhostHTTPSPort    int            `json:"vhost_https_port"`
	Token             string         `gorm:"size:255" json:"-"`
	PluginSecret      string         `gorm:"size:64" json:"-"`
	PluginAuthEnabled bool           `gorm:"default:false" json:"plugin_auth_enabled"`
	Region            string         `gorm:"size:64" json:"region"`
	Status            string         `gorm:"size:20;default:pending;not null" json:"status"` // pending, installing, running, stopped, error
	AgentInstalled    bool           `gorm:"default:false" json:"agent_installed"`
	AgentGRPCAddr     string         `gorm:"column:agent_grpc_addr;size:255" json:"agent_grpc_addr"`
	MaxUsers          int            `gorm:"default:0" json:"max_users"`
	CPUUsage          float64        `json:"cpu_usage"`
	MemoryUsage       float64        `json:"memory_usage"`
	DiskUsage         float64        `json:"disk_usage"`
	BandwidthUsage    int64          `json:"bandwidth_usage"`
	ClientCount       int            `gorm:"default:0" json:"client_count"`
	ProxyCount        int            `gorm:"default:0" json:"proxy_count"`
	LastHeartbeat     *time.Time     `json:"last_heartbeat"`
	Latency           int64          `gorm:"-" json:"latency"` // ms, measured on-the-fly
	ErrorMsg          string         `gorm:"type:text" json:"error_msg"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`

	Proxies []Proxy `gorm:"foreignKey:ServerID" json:"proxies,omitempty"`
}

func (Server) TableName() string {
	return "servers"
}

type ServerMetricsHistory struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  uint      `gorm:"not null;index" json:"server_id"`
	Timestamp time.Time `gorm:"not null" json:"timestamp"`
	Data      string    `gorm:"type:text" json:"data"`
}

func (ServerMetricsHistory) TableName() string {
	return "server_metrics_histories"
}
