package handler

import (
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	db *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrow := todayStart.AddDate(0, 0, 1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	nextMonth := monthStart.AddDate(0, 1, 0)

	// Server stats
	var totalServers, onlineServers int64
	h.db.Model(&model.Server{}).Count(&totalServers)
	h.db.Model(&model.Server{}).Where("status = ?", "running").Count(&onlineServers)

	// User stats
	var totalUsers, todayUsers, activeUsers int64
	h.db.Model(&model.User{}).Count(&totalUsers)
	h.db.Model(&model.User{}).Where("created_at >= ? AND created_at < ?", todayStart, tomorrow).Count(&todayUsers)
	h.db.Model(&model.User{}).Where("status = ?", "active").Count(&activeUsers)

	// Proxy stats
	var totalProxies, runningProxies int64
	h.db.Model(&model.Proxy{}).Count(&totalProxies)
	h.db.Model(&model.Proxy{}).Where("status = ?", "running").Count(&runningProxies)

	// Proxy type distribution
	type TypeCount struct {
		Type  string `json:"type"`
		Count int64  `json:"count"`
	}
	var proxyTypes []TypeCount
	h.db.Model(&model.Proxy{}).Select("type, count(*) as count").Group("type").Scan(&proxyTypes)

	// Traffic stats (today)
	var todayTrafficIn, todayTrafficOut int64
	h.db.Model(&model.TrafficDaily{}).Where("date = ?", todayStart.Format("2006-01-02")).
		Select("COALESCE(SUM(traffic_in), 0), COALESCE(SUM(traffic_out), 0)").
		Row().Scan(&todayTrafficIn, &todayTrafficOut)

	// Revenue stats
	var todayRevenue, monthRevenue, totalRevenue float64
	h.db.Model(&model.Order{}).Where("pay_status = ? AND paid_at >= ? AND paid_at < ?", "paid", todayStart, tomorrow).
		Select("COALESCE(SUM(amount), 0)").Row().Scan(&todayRevenue)
	h.db.Model(&model.Order{}).Where("pay_status = ? AND paid_at >= ? AND paid_at < ?", "paid", monthStart, nextMonth).
		Select("COALESCE(SUM(amount), 0)").Row().Scan(&monthRevenue)
	h.db.Model(&model.Order{}).Where("pay_status = ?", "paid").
		Select("COALESCE(SUM(amount), 0)").Row().Scan(&totalRevenue)

	// Recent orders
	var recentOrders []model.Order
	h.db.Order("id desc").Limit(10).Preload("User").Preload("Plan").Find(&recentOrders)

	// Recent alerts (errors)
	var errorServers []model.Server
	h.db.Where("status = ?", "error").Limit(10).Find(&errorServers)

	response.Success(c, gin.H{
		"servers": gin.H{
			"total":  totalServers,
			"online": onlineServers,
		},
		"users": gin.H{
			"total":  totalUsers,
			"today":  todayUsers,
			"active": activeUsers,
		},
		"proxies": gin.H{
			"total":   totalProxies,
			"running": runningProxies,
			"by_type": proxyTypes,
		},
		"traffic": gin.H{
			"today_in":  todayTrafficIn,
			"today_out": todayTrafficOut,
		},
		"revenue": gin.H{
			"today": todayRevenue,
			"month": monthRevenue,
			"total": totalRevenue,
		},
		"recent_orders": recentOrders,
		"error_servers": errorServers,
	})
}
