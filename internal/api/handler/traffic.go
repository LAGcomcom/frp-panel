package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TrafficHandler struct {
	db *gorm.DB
}

func NewTrafficHandler(db *gorm.DB) *TrafficHandler {
	return &TrafficHandler{db: db}
}

// GetTrafficStats returns traffic statistics for the current user
func (h *TrafficHandler) GetTrafficStats(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	// Get user's API key
	var user model.User
	if err := h.db.First(&user, userID).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	// Get user's proxies
	var proxies []model.Proxy
	h.db.Where("user_id = ?", userID).Find(&proxies)

	// Fetch real-time traffic from frps servers
	var totalIn, totalOut int64
	type ProxyTraffic struct {
		ProxyID    uint   `json:"proxy_id"`
		ProxyName  string `json:"proxy_name"`
		TrafficIn  int64  `json:"traffic_in"`
		TrafficOut int64  `json:"traffic_out"`
	}
	var proxyTraffic []ProxyTraffic

	// Get all servers
	var servers []model.Server
	h.db.Find(&servers)

	for _, proxy := range proxies {
		proxyName := proxy.Name
		if idx := indexOf(proxy.Name, '_'); idx > 0 {
			proxyName = proxy.Name[idx+1:]
		}
		fullName := fmt.Sprintf("%s.%s", user.APIKey, proxyName)

		var proxyIn, proxyOut int64
		for _, server := range servers {
			if server.Status != "running" {
				continue
			}
			frpsProxies := fetchServerProxies(server.IP, server.DashboardPort, server.DashboardUser, server.DashboardPassword)
			for _, p := range frpsProxies {
				if p.Name == fullName {
					proxyIn += p.TodayTrafficIn
					proxyOut += p.TodayTrafficOut
					break
				}
			}
		}

		totalIn += proxyIn
		totalOut += proxyOut

		// Update proxy traffic in DB
		h.db.Model(&proxy).Updates(map[string]interface{}{
			"traffic_in":  proxyIn,
			"traffic_out": proxyOut,
		})

		proxyTraffic = append(proxyTraffic, ProxyTraffic{
			ProxyID:    proxy.ID,
			ProxyName:  proxy.Name,
			TrafficIn:  proxyIn,
			TrafficOut: proxyOut,
		})
	}

	// Get real monthly traffic from TrafficDaily
	var monthlyIn, monthlyOut int64
	monthStart := time.Now().Format("2006-01") + "-01"
	h.db.Model(&model.TrafficDaily{}).
		Where("user_id = ? AND date >= ?", userID, monthStart).
		Select("COALESCE(SUM(traffic_in), 0), COALESCE(SUM(traffic_out), 0)").
		Row().Scan(&monthlyIn, &monthlyOut)

	// Get total cumulative traffic
	var totalTrafficIn, totalTrafficOut int64
	h.db.Model(&model.TrafficDaily{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(traffic_in), 0), COALESCE(SUM(traffic_out), 0)").
		Row().Scan(&totalTrafficIn, &totalTrafficOut)

	// Get user plan limits
	var planInfo gin.H
	if user.PlanID != nil {
		var plan model.Plan
		if err := h.db.First(&plan, user.PlanID).Error; err == nil {
			planInfo = gin.H{
				"name":          plan.Name,
				"max_traffic":   plan.MaxTraffic,
				"max_bandwidth": plan.MaxBandwidth,
				"max_proxies":   plan.MaxProxies,
				"max_ports":     plan.MaxPorts,
				"expires_at":    user.PlanExpiresAt,
			}
		}
	} else {
		// Free plan limits from settings
		settings := h.getSettingsMap()
		maxProxies := 5
		maxBandwidth := int64(10 * 1024 * 1024)    // 10MB/s
		maxTraffic := int64(10 * 1024 * 1024 * 1024) // 10GB
		if v := settings["free_max_proxies"]; v != "" {
			fmt.Sscanf(v, "%d", &maxProxies)
		}
		if v := settings["free_max_bandwidth_mb"]; v != "" {
			var mb float64
			fmt.Sscanf(v, "%f", &mb)
			if mb > 0 {
				maxBandwidth = int64(mb * 1024 * 1024)
			}
		}
		if v := settings["free_max_traffic_gb"]; v != "" {
			var gb float64
			fmt.Sscanf(v, "%f", &gb)
			if gb > 0 {
				maxTraffic = int64(gb * 1024 * 1024 * 1024)
			}
		}
		planInfo = gin.H{
			"name":          "免费版",
			"max_traffic":   maxTraffic,
			"max_bandwidth": maxBandwidth,
			"max_proxies":   maxProxies,
		}
	}

	response.Success(c, gin.H{
		"monthly": gin.H{
			"traffic_in":  monthlyIn,
			"traffic_out": monthlyOut,
			"total":       monthlyIn + monthlyOut,
		},
		"total": gin.H{
			"traffic_in":  totalTrafficIn,
			"traffic_out": totalTrafficOut,
			"total":       totalTrafficIn + totalTrafficOut,
		},
		"per_proxy": proxyTraffic,
		"plan":      planInfo,
	})
}

// GetTrafficLogs returns traffic log history
func (h *TrafficHandler) GetTrafficLogs(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	page := c.DefaultQuery("page", "1")
	size := c.DefaultQuery("size", "20")
	proxyID := c.DefaultQuery("proxy_id", "")
	days := c.DefaultQuery("days", "30")

	var logs []model.TrafficDaily
	var total int64

	query := h.db.Model(&model.TrafficDaily{}).Where("user_id = ?", userID)

	if proxyID != "" {
		query = query.Where("proxy_id = ?", proxyID)
	}

	// Filter by date range
	since := time.Now().AddDate(0, 0, -parseInt(days))
	query = query.Where("date >= ?", since.Format("2006-01-02"))

	query.Count(&total)

	offset := (parseInt(page) - 1) * parseInt(size)
	query.Offset(offset).Limit(parseInt(size)).Order("date desc").Find(&logs)

	response.Page(c, logs, total, parseInt(page), parseInt(size))
}

// AdminGetTrafficStats returns traffic statistics for all users (admin)
func (h *TrafficHandler) AdminGetTrafficStats(c *gin.Context) {
	// Fetch real-time traffic from all frps servers
	var servers []model.Server
	h.db.Find(&servers)

	var totalTrafficIn, totalTrafficOut int64
	serverTraffic := make(map[uint][2]int64)
	userTraffic := make(map[uint][2]int64)

	for _, server := range servers {
		if server.Status != "running" {
			continue
		}
		proxies := fetchServerProxies(server.IP, server.DashboardPort, server.DashboardUser, server.DashboardPassword)
		for _, p := range proxies {
			totalTrafficIn += p.TodayTrafficIn
			totalTrafficOut += p.TodayTrafficOut
			serverTraffic[server.ID] = [2]int64{
				serverTraffic[server.ID][0] + p.TodayTrafficIn,
				serverTraffic[server.ID][1] + p.TodayTrafficOut,
			}
		}
	}

	// Map proxy traffic to users
	var allProxies []model.Proxy
	h.db.Find(&allProxies)
	for _, proxy := range allProxies {
		// Find matching frps proxy
		var user model.User
		if err := h.db.First(&user, proxy.UserID).Error; err != nil {
			continue
		}
		proxyName := proxy.Name
		if idx := indexOf(proxy.Name, '_'); idx > 0 {
			proxyName = proxy.Name[idx+1:]
		}
		fullName := fmt.Sprintf("%s.%s", user.APIKey, proxyName)

		for _, server := range servers {
			if server.Status != "running" {
				continue
			}
			proxies := fetchServerProxies(server.IP, server.DashboardPort, server.DashboardUser, server.DashboardPassword)
			for _, p := range proxies {
				if p.Name == fullName {
					userTraffic[proxy.UserID] = [2]int64{
						userTraffic[proxy.UserID][0] + p.TodayTrafficIn,
						userTraffic[proxy.UserID][1] + p.TodayTrafficOut,
					}
					// Update proxy traffic in DB
					h.db.Model(&proxy).Updates(map[string]interface{}{
						"traffic_in":  p.TodayTrafficIn,
						"traffic_out": p.TodayTrafficOut,
					})
					break
				}
			}
		}
	}

	// Build top users list
	type UserTrafficInfo struct {
		UserID     uint   `json:"user_id"`
		Email      string `json:"email"`
		TrafficIn  int64  `json:"traffic_in"`
		TrafficOut int64  `json:"traffic_out"`
	}
	var topUsers []UserTrafficInfo
	for userID, traffic := range userTraffic {
		var user model.User
		h.db.Select("email").First(&user, userID)
		topUsers = append(topUsers, UserTrafficInfo{
			UserID:     userID,
			Email:      user.Email,
			TrafficIn:  traffic[0],
			TrafficOut: traffic[1],
		})
	}

	// Build top servers list
	type ServerTrafficInfo struct {
		ServerID   uint   `json:"server_id"`
		ServerName string `json:"server_name"`
		TrafficIn  int64  `json:"traffic_in"`
		TrafficOut int64  `json:"traffic_out"`
	}
	var topServers []ServerTrafficInfo
	for serverID, traffic := range serverTraffic {
		var server model.Server
		h.db.Select("name").First(&server, serverID)
		topServers = append(topServers, ServerTrafficInfo{
			ServerID:   serverID,
			ServerName: server.Name,
			TrafficIn:  traffic[0],
			TrafficOut: traffic[1],
		})
	}

	// Get real monthly traffic from TrafficDaily
	var monthlyIn, monthlyOut int64
	monthStart := time.Now().Format("2006-01") + "-01"
	h.db.Model(&model.TrafficDaily{}).
		Where("date >= ?", monthStart).
		Select("COALESCE(SUM(traffic_in), 0), COALESCE(SUM(traffic_out), 0)").
		Row().Scan(&monthlyIn, &monthlyOut)

	response.Success(c, gin.H{
		"today": gin.H{
			"traffic_in":  totalTrafficIn,
			"traffic_out": totalTrafficOut,
			"total":       totalTrafficIn + totalTrafficOut,
		},
		"monthly": gin.H{
			"traffic_in":  monthlyIn,
			"traffic_out": monthlyOut,
			"total":       monthlyIn + monthlyOut,
		},
		"top_users":   topUsers,
		"top_servers": topServers,
	})
}

type serverProxy struct {
	Name            string `json:"name"`
	TodayTrafficIn  int64  `json:"todayTrafficIn"`
	TodayTrafficOut int64  `json:"todayTrafficOut"`
}

type serverProxyResponse struct {
	Proxies []serverProxy `json:"proxies"`
}

func fetchServerProxies(ip string, dashboardPort int, user, password string) []serverProxy {
	client := &http.Client{Timeout: 2 * time.Second}
	types := []string{"tcp", "udp", "http", "https", "stcp", "xtcp", "tcpmux"}
	var all []serverProxy

	for _, proxyType := range types {
		url := fmt.Sprintf("http://%s:%d/api/proxy/%s", ip, dashboardPort, proxyType)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}
		req.SetBasicAuth(user, password)
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}
		var result serverProxyResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil {
			continue
		}
		all = append(all, result.Proxies...)
	}
	return all
}

func (h *TrafficHandler) getSettingsMap() map[string]string {
	var settings []model.Setting
	h.db.Find(&settings)
	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result
}
