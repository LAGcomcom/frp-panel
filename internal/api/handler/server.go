package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/accesscontrol"
	"github.com/frp-panel/frp-panel/internal/pkg/hash"
	"github.com/frp-panel/frp-panel/internal/service/deployer"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ServerHandler struct {
	db          *gorm.DB
	deployer    *deployer.Deployer
	serverToken string
}

func NewServerHandler(db *gorm.DB, d *deployer.Deployer, serverToken string) *ServerHandler {
	return &ServerHandler{db: db, deployer: d, serverToken: serverToken}
}

func (h *ServerHandler) markServerPluginStatus(server *model.Server) {
	if h.deployer == nil {
		if server.PluginAuthEnabled {
			server.PluginAuthStatus = "ready"
			server.PluginAuthMessage = "安全模式"
		} else {
			server.PluginAuthStatus = "redeploy_required"
			server.PluginAuthMessage = "需重新部署"
		}
		return
	}
	if ok, reason := h.deployer.PluginEndpointMatches("", server); ok {
		server.PluginAuthStatus = "ready"
		server.PluginAuthMessage = "安全模式"
	} else {
		server.PluginAuthStatus = "redeploy_required"
		server.PluginAuthMessage = reason
	}
}

func (h *ServerHandler) serverPluginReady(server *model.Server) bool {
	h.markServerPluginStatus(server)
	return server.PluginAuthStatus == "ready"
}

type CreateServerRequest struct {
	Name          string `json:"name" binding:"required"`
	IP            string `json:"ip" binding:"required"`
	SSHPort       int    `json:"ssh_port"`
	SSHUser       string `json:"ssh_user"`
	SSHAuthType   string `json:"ssh_auth_type" binding:"required,oneof=password key"`
	SSHPassword   string `json:"ssh_password"`
	SSHPrivateKey string `json:"ssh_private_key"`
	Region        string `json:"region"`
	MaxUsers      int    `json:"max_users"`
}

func (h *ServerHandler) CreateServer(c *gin.Context) {
	var req CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}

	// Use configured server token or generate one
	token := h.serverToken
	if token == "" {
		token = hash.RandomString(32)
	}

	server := model.Server{
		Name:              req.Name,
		IP:                req.IP,
		SSHPort:           req.SSHPort,
		SSHUser:           req.SSHUser,
		SSHAuthType:       req.SSHAuthType,
		SSHPassword:       req.SSHPassword,
		SSHPrivateKey:     req.SSHPrivateKey,
		Region:            req.Region,
		MaxUsers:          req.MaxUsers,
		Status:            "pending",
		Token:             token,
		PluginSecret:      hash.RandomString(32),
		BindPort:          7000,
		DashboardPort:     7500,
		DashboardUser:     "admin",
		DashboardPassword: hash.RandomString(12),
	}

	if err := h.db.Create(&server).Error; err != nil {
		response.InternalError(c, "failed to create server")
		return
	}

	response.SuccessWithMessage(c, "server created", server)
}

func (h *ServerHandler) ListServers(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	size := c.DefaultQuery("size", "20")
	status := strings.TrimSpace(c.Query("status"))
	region := strings.TrimSpace(c.Query("region"))
	keyword := strings.TrimSpace(c.Query("keyword"))

	var servers []model.Server
	var total int64

	query := h.db.Model(&model.Server{})
	if keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("(name LIKE ? OR ip LIKE ? OR region LIKE ? OR frp_version LIKE ?)", pattern, pattern, pattern, pattern)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if region != "" {
		query = query.Where("region = ?", region)
	}

	if err := query.Count(&total).Error; err != nil {
		response.InternalError(c, "服务器列表加载失败，请稍后重试")
		return
	}

	offset := (parseInt(page) - 1) * parseInt(size)
	if err := query.Offset(offset).Limit(parseInt(size)).Order("id desc").Find(&servers).Error; err != nil {
		response.InternalError(c, "服务器列表加载失败，请稍后重试")
		return
	}

	latencyLimit := make(chan struct{}, 8)
	var latencyWG sync.WaitGroup
	for i := range servers {
		if servers[i].Status == "running" {
			latencyWG.Add(1)
			go func(index int, address string) {
				defer latencyWG.Done()
				latencyLimit <- struct{}{}
				defer func() { <-latencyLimit }()
				latency, _ := measureTCPLatency(address, 2*time.Second)
				servers[index].Latency = latency
			}(i, fmt.Sprintf("%s:%d", servers[i].IP, servers[i].BindPort))
		}
	}
	latencyWG.Wait()

	// Mask sensitive fields
	for i := range servers {
		h.markServerPluginStatus(&servers[i])
		servers[i].Token = "***"
		servers[i].PluginSecret = "***"
		servers[i].SSHPassword = ""
		servers[i].SSHPrivateKey = ""
		servers[i].DashboardPassword = "***"
	}

	response.Page(c, servers, total, parseInt(page), parseInt(size))
}

// ListAvailableServers returns running servers with basic info for users
func (h *ServerHandler) ListAvailableServers(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	user, err := accesscontrol.LoadUser(h.db, userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}
	if accesscontrol.IsPlanGroupExpired(user) {
		response.Success(c, []model.Server{})
		return
	}
	var servers []model.Server
	query := h.db.Where("status = ?", "running")
	if user.GroupID != nil {
		query = query.Where("plugin_auth_enabled = ? AND id IN (?)", true, h.db.Model(&model.UserGroupServer{}).
			Select("server_id").Where("user_group_id = ?", *user.GroupID))
	}
	query.Order("id asc").Find(&servers)
	filteredServers := servers[:0]
	for i := range servers {
		if h.serverPluginReady(&servers[i]) {
			filteredServers = append(filteredServers, servers[i])
		}
	}
	servers = filteredServers

	type ServerMetrics struct {
		CPUUsage    float64 `json:"cpu_usage"`
		MemoryUsed  int64   `json:"memory_used"`
		MemoryTotal int64   `json:"memory_total"`
		DiskUsed    int64   `json:"disk_used"`
		DiskTotal   int64   `json:"disk_total"`
		NetIn       int64   `json:"net_in"`
		NetOut      int64   `json:"net_out"`
		LoadAvg1    float64 `json:"load_avg_1"`
		LoadAvg5    float64 `json:"load_avg_5"`
		LoadAvg15   float64 `json:"load_avg_15"`
		Connections int     `json:"connections"`
		Uptime      int64   `json:"uptime"`
	}

	type ServerInfo struct {
		ID             uint           `json:"id"`
		Name           string         `json:"name"`
		IP             string         `json:"ip"`
		Region         string         `json:"region"`
		FrpVersion     string         `json:"frp_version"`
		BindPort       int            `json:"bind_port"`
		VhostHTTPPort  int            `json:"vhost_http_port"`
		VhostHTTPSPort int            `json:"vhost_https_port"`
		ClientCount    int            `json:"client_count"`
		ProxyCount     int            `json:"proxy_count"`
		Latency        int64          `json:"latency"`
		AgentInstalled bool           `json:"agent_installed"`
		Metrics        *ServerMetrics `json:"metrics,omitempty"`
	}

	result := make([]ServerInfo, len(servers))
	latencyLimit := make(chan struct{}, 8)
	var latencyWG sync.WaitGroup
	for i, s := range servers {
		result[i] = ServerInfo{
			ID:             s.ID,
			Name:           s.Name,
			IP:             s.IP,
			Region:         s.Region,
			FrpVersion:     s.FrpVersion,
			BindPort:       s.BindPort,
			VhostHTTPPort:  s.VhostHTTPPort,
			VhostHTTPSPort: s.VhostHTTPSPort,
			ClientCount:    s.ClientCount,
			ProxyCount:     s.ProxyCount,
			AgentInstalled: s.AgentInstalled,
		}

		// Fetch latest metrics if agent is installed
		if s.AgentInstalled {
			var history model.ServerMetricsHistory
			if err := h.db.Where("server_id = ?", s.ID).Order("timestamp desc").First(&history).Error; err == nil {
				var m ServerMetrics
				if err := json.Unmarshal([]byte(history.Data), &m); err == nil {
					result[i].Metrics = &m
				}
			}
		}

		latencyWG.Add(1)
		go func(index int, address string) {
			defer latencyWG.Done()
			latencyLimit <- struct{}{}
			defer func() { <-latencyLimit }()
			latency, _ := measureTCPLatency(address, 2*time.Second)
			result[index].Latency = latency
		}(i, fmt.Sprintf("%s:%d", s.IP, s.BindPort))
	}
	latencyWG.Wait()

	response.Success(c, result)
}

type frpsServerInfo struct {
	Version        string         `json:"version"`
	ClientCounts   int            `json:"clientCounts"`
	ProxyTypeCount map[string]int `json:"proxyTypeCount"`
}

func measureTCPLatency(address string, timeout time.Duration) (int64, bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return 0, false
	}
	conn.Close()
	latency := time.Since(start).Milliseconds()
	if latency < 1 {
		latency = 1
	}
	return latency, true
}

func fetchFrpsInfo(ip string, dashboardPort int, user, password string) *frpsServerInfo {
	url := fmt.Sprintf("http://%s:%d/api/serverinfo", ip, dashboardPort)
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	req.SetBasicAuth(user, password)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	var info frpsServerInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil
	}
	return &info
}

func (h *ServerHandler) GetServer(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	// Fetch real-time data from frps dashboard
	if server.Status == "running" {
		// Measure latency
		latency, reachable := measureTCPLatency(fmt.Sprintf("%s:%d", server.IP, server.BindPort), 2*time.Second)
		if reachable {
			server.Latency = latency
		}

		info := fetchFrpsInfo(server.IP, server.DashboardPort, server.DashboardUser, server.DashboardPassword)
		if info != nil {
			if info.Version != "" {
				server.FrpVersion = info.Version
			}
			server.ClientCount = info.ClientCounts
			total := 0
			for _, count := range info.ProxyTypeCount {
				total += count
			}
			server.ProxyCount = total
			updates := map[string]interface{}{
				"client_count": info.ClientCounts,
				"proxy_count":  total,
			}
			if info.Version != "" {
				updates["frp_version"] = info.Version
			}
			h.db.Model(&server).Updates(updates)
		}
	}

	// Mask sensitive fields
	h.markServerPluginStatus(&server)
	server.SSHPassword = ""
	server.SSHPrivateKey = ""

	// Get proxy count from DB
	var proxyCount int64
	h.db.Model(&model.Proxy{}).Where("server_id = ?", server.ID).Count(&proxyCount)

	response.Success(c, gin.H{
		"server":      server,
		"proxy_count": proxyCount,
	})
}

func (h *ServerHandler) UpdateServer(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Name              string `json:"name"`
		Region            string `json:"region"`
		MaxUsers          *int   `json:"max_users"`
		BindPort          int    `json:"bind_port"`
		DashboardPort     int    `json:"dashboard_port"`
		DashboardUser     string `json:"dashboard_user"`
		DashboardPassword string `json:"dashboard_password"`
		VhostHTTPPort     int    `json:"vhost_http_port"`
		VhostHTTPSPort    int    `json:"vhost_https_port"`
		Token             string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Region != "" {
		updates["region"] = req.Region
	}
	if req.MaxUsers != nil {
		if *req.MaxUsers < 0 {
			response.BadRequest(c, "max users cannot be negative")
			return
		}
		updates["max_users"] = *req.MaxUsers
	}
	if req.BindPort > 0 {
		updates["bind_port"] = req.BindPort
	}
	if req.DashboardPort > 0 {
		updates["dashboard_port"] = req.DashboardPort
	}
	if req.DashboardUser != "" {
		updates["dashboard_user"] = req.DashboardUser
	}
	if req.DashboardPassword != "" {
		updates["dashboard_password"] = req.DashboardPassword
	}
	if req.VhostHTTPPort > 0 {
		updates["vhost_http_port"] = req.VhostHTTPPort
	}
	if req.VhostHTTPSPort > 0 {
		updates["vhost_https_port"] = req.VhostHTTPSPort
	}
	if req.Token != "" {
		updates["token"] = req.Token
	}

	if err := h.db.Model(&model.Server{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.InternalError(c, "failed to update server")
		return
	}

	response.SuccessWithMessage(c, "server updated", nil)
}

func (h *ServerHandler) DeleteServer(c *gin.Context) {
	id := c.Param("id")

	// Check if server has active proxies
	var count int64
	h.db.Model(&model.Proxy{}).Where("server_id = ? AND status = ?", id, "running").Count(&count)
	if count > 0 {
		response.BadRequest(c, "server has active proxies, please remove them first")
		return
	}

	if err := h.db.Delete(&model.Server{}, id).Error; err != nil {
		response.InternalError(c, "failed to delete server")
		return
	}

	response.SuccessWithMessage(c, "server deleted", nil)
}

// Deploy frps on remote server via SSH
func (h *ServerHandler) DeployServer(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	if server.Status == "installing" {
		response.BadRequest(c, "server is already being installed")
		return
	}

	panelAddr := c.Query("panel_addr")
	if panelAddr == "" {
		panelAddr = h.deployer.GetPluginWebhookURL()
	}
	if panelAddr == "" {
		panelAddr = fmt.Sprintf("http://localhost:%d", h.deployer.GetPanelPort())
	}
	go h.deployer.Deploy(&server, panelAddr)

	response.SuccessWithMessage(c, "deployment started", gin.H{
		"status": "installing",
	})
}

// Restart frps on remote server
func (h *ServerHandler) RestartServer(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	if server.Status != "running" && server.Status != "stopped" {
		response.BadRequest(c, fmt.Sprintf("cannot restart server in %s status", server.Status))
		return
	}

	if err := h.deployer.Restart(&server); err != nil {
		response.InternalError(c, fmt.Sprintf("restart failed: %v", err))
		return
	}

	response.SuccessWithMessage(c, "server restarted", nil)
}

// Stop frps on remote server
func (h *ServerHandler) StopServer(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	if err := h.deployer.Stop(&server); err != nil {
		response.InternalError(c, fmt.Sprintf("stop failed: %v", err))
		return
	}

	response.SuccessWithMessage(c, "server stopped", nil)
}

// Get frps config from remote server
func (h *ServerHandler) GetServerConfig(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	config, err := h.deployer.GetFrpsConfig(&server)
	if err != nil {
		response.InternalError(c, fmt.Sprintf("get config failed: %v", err))
		return
	}

	response.Success(c, gin.H{"config": config})
}

// Update frps config on remote server
func (h *ServerHandler) UpdateServerConfig(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Config string `json:"config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	panelAddr := c.Query("panel_addr")
	if panelAddr == "" {
		panelAddr = h.deployer.GetPluginWebhookURL()
	}
	if panelAddr == "" {
		panelAddr = fmt.Sprintf("http://localhost:%d", h.deployer.GetPanelPort())
	}

	if err := h.deployer.UpdateConfig(&server, req.Config, panelAddr); err != nil {
		response.InternalError(c, fmt.Sprintf("update config failed: %v", err))
		return
	}

	response.SuccessWithMessage(c, "config updated", nil)
}

// Get frps logs from remote server
func (h *ServerHandler) GetServerLogs(c *gin.Context) {
	id := c.Param("id")
	lines, err := strconv.Atoi(c.DefaultQuery("lines", "100"))
	if err != nil || lines < 1 || lines > 2000 {
		response.BadRequest(c, "log lines must be between 1 and 2000")
		return
	}

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	logs, err := h.deployer.GetFrpsLogs(&server, lines)
	if err != nil {
		response.InternalError(c, fmt.Sprintf("get logs failed: %v", err))
		return
	}

	response.Success(c, gin.H{"logs": logs})
}

// Uninstall frps from remote server
func (h *ServerHandler) UninstallServer(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	if err := h.deployer.Uninstall(&server); err != nil {
		response.InternalError(c, fmt.Sprintf("uninstall failed: %v", err))
		return
	}

	response.SuccessWithMessage(c, "frps uninstalled", nil)
}

// Get server's connected clients via frps Dashboard API
func (h *ServerHandler) GetServerClients(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	// Fetch clients from frps Dashboard API
	clients := fetchFrpsClients(server.IP, server.DashboardPort, server.DashboardUser, server.DashboardPassword)
	response.Success(c, gin.H{
		"clients": clients,
	})
}

type frpsClient struct {
	Key             string `json:"key"`
	User            string `json:"user"`
	ClientID        string `json:"clientID"`
	Version         string `json:"version"`
	Hostname        string `json:"hostname"`
	ClientIP        string `json:"clientIP"`
	Online          bool   `json:"online"`
	LastConnectedAt int64  `json:"lastConnectedAt"`
}

func fetchFrpsClients(ip string, dashboardPort int, user, password string) []frpsClient {
	url := fmt.Sprintf("http://%s:%d/api/clients", ip, dashboardPort)
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []frpsClient{}
	}
	req.SetBasicAuth(user, password)
	resp, err := client.Do(req)
	if err != nil {
		return []frpsClient{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return []frpsClient{}
	}
	var clients []frpsClient
	if err := json.NewDecoder(resp.Body).Decode(&clients); err != nil {
		return []frpsClient{}
	}
	return clients
}

// Get server's proxies via frps Dashboard API
func (h *ServerHandler) GetServerProxies(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	// Get proxies from DB
	var proxies []model.Proxy
	h.db.Where("server_id = ?", server.ID).Find(&proxies)

	// Fetch real-time traffic from frps dashboard
	if server.Status == "running" {
		frpsProxies := fetchFrpsProxies(server.IP, server.DashboardPort, server.DashboardUser, server.DashboardPassword)
		// Match frps proxies with DB proxies by name
		// frps proxy name format: "{api_key}.{proxy_name}"
		// DB proxy name format: "{user_id}_{proxy_name}"
		for i := range proxies {
			// Get user's API key
			var user model.User
			if err := h.db.First(&user, proxies[i].UserID).Error; err != nil {
				continue
			}
			// Extract proxy name from DB name (remove user_id prefix)
			proxyName := proxies[i].Name
			if idx := indexOf(proxies[i].Name, '_'); idx > 0 {
				proxyName = proxies[i].Name[idx+1:]
			}
			fullName := fmt.Sprintf("%s.%s", user.APIKey, proxyName)
			for _, fp := range frpsProxies {
				if fp.Name == fullName {
					proxies[i].TrafficIn = fp.TodayTrafficIn
					proxies[i].TrafficOut = fp.TodayTrafficOut
					// Update in DB
					h.db.Model(&proxies[i]).Updates(map[string]interface{}{
						"traffic_in":  fp.TodayTrafficIn,
						"traffic_out": fp.TodayTrafficOut,
					})
					break
				}
			}
		}
	}

	response.Success(c, proxies)
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

type frpsProxy struct {
	Name            string `json:"name"`
	TodayTrafficIn  int64  `json:"todayTrafficIn"`
	TodayTrafficOut int64  `json:"todayTrafficOut"`
	CurConns        int    `json:"curConns"`
	Status          string `json:"status"`
}

type frpsProxyResponse struct {
	Proxies []frpsProxy `json:"proxies"`
}

func fetchFrpsProxies(ip string, dashboardPort int, user, password string) []frpsProxy {
	proxyTypes := []string{"tcp", "udp", "http", "https", "stcp", "xtcp", "tcpmux"}
	client := &http.Client{Timeout: 2 * time.Second}
	var allProxies []frpsProxy

	for _, pType := range proxyTypes {
		url := fmt.Sprintf("http://%s:%d/api/proxy/%s", ip, dashboardPort, pType)
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
		var result frpsProxyResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		allProxies = append(allProxies, result.Proxies...)
	}
	return allProxies
}

// ReportMetrics receives metrics from the agent
func (h *ServerHandler) ReportMetrics(c *gin.Context) {
	id := c.Param("id")

	// Validate agent API key
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		response.Unauthorized(c, "missing authorization header")
		return
	}
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	// Validate API key matches server's agent key
	if server.AgentGRPCAddr == "" || server.AgentGRPCAddr != apiKey {
		response.Unauthorized(c, "invalid API key")
		return
	}

	var req struct {
		Timestamp   int64   `json:"timestamp"`
		CPUUsage    float64 `json:"cpu_usage"`
		MemoryUsed  int64   `json:"memory_used"`
		MemoryTotal int64   `json:"memory_total"`
		DiskUsed    int64   `json:"disk_used"`
		DiskTotal   int64   `json:"disk_total"`
		NetIn       int64   `json:"net_in"`
		NetOut      int64   `json:"net_out"`
		LoadAvg1    float64 `json:"load_avg_1"`
		LoadAvg5    float64 `json:"load_avg_5"`
		LoadAvg15   float64 `json:"load_avg_15"`
		Connections int     `json:"connections"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Update server metrics in database
	h.db.Model(&server).Updates(map[string]interface{}{
		"cpu_usage":       req.CPUUsage,
		"memory_usage":    req.MemoryUsed,
		"disk_usage":      req.DiskUsed,
		"bandwidth_usage": req.NetIn + req.NetOut,
		"last_heartbeat":  time.Now(),
		"status":          "running",
	})

	// Store metrics history (keep last 1000 entries per server)
	metricsJSON, _ := json.Marshal(req)
	metricsRecord := model.ServerMetricsHistory{
		ServerID:  server.ID,
		Timestamp: time.Unix(req.Timestamp, 0),
		Data:      string(metricsJSON),
	}
	h.db.Create(&metricsRecord)

	// Clean old metrics (keep last 1000)
	h.db.Exec("DELETE FROM server_metrics_histories WHERE server_id = ? AND id NOT IN (SELECT id FROM server_metrics_histories WHERE server_id = ? ORDER BY id DESC LIMIT 1000)", server.ID, server.ID)

	response.SuccessWithMessage(c, "metrics received", nil)
}

// GetServerMetrics returns server metrics history
func (h *ServerHandler) GetServerMetrics(c *gin.Context) {
	id := c.Param("id")
	hours := c.DefaultQuery("hours", "1")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	// Get history
	since := time.Now().Add(-time.Duration(parseInt(hours)) * time.Hour)
	var history []model.ServerMetricsHistory
	h.db.Where("server_id = ? AND timestamp >= ?", server.ID, since).
		Order("timestamp ASC").
		Find(&history)

	// Get current metrics from the latest history entry
	current := gin.H{
		"cpu_usage":       server.CPUUsage,
		"memory_usage":    server.MemoryUsage,
		"disk_usage":      server.DiskUsage,
		"bandwidth_usage": server.BandwidthUsage,
		"last_heartbeat":  server.LastHeartbeat,
	}

	// Parse latest history for detailed metrics
	if len(history) > 0 {
		var latest struct {
			CPUUsage    float64 `json:"cpu_usage"`
			MemoryUsed  int64   `json:"memory_used"`
			MemoryTotal int64   `json:"memory_total"`
			DiskUsed    int64   `json:"disk_used"`
			DiskTotal   int64   `json:"disk_total"`
			NetIn       int64   `json:"net_in"`
			NetOut      int64   `json:"net_out"`
			LoadAvg1    float64 `json:"load_avg_1"`
			LoadAvg5    float64 `json:"load_avg_5"`
			LoadAvg15   float64 `json:"load_avg_15"`
			Connections int     `json:"connections"`
		}
		if err := json.Unmarshal([]byte(history[len(history)-1].Data), &latest); err == nil {
			current = gin.H{
				"cpu_usage":      latest.CPUUsage,
				"memory_usage":   latest.MemoryUsed,
				"memory_total":   latest.MemoryTotal,
				"disk_usage":     latest.DiskUsed,
				"disk_total":     latest.DiskTotal,
				"net_in":         latest.NetIn,
				"net_out":        latest.NetOut,
				"load_avg_1":     latest.LoadAvg1,
				"load_avg_5":     latest.LoadAvg5,
				"load_avg_15":    latest.LoadAvg15,
				"connections":    latest.Connections,
				"last_heartbeat": server.LastHeartbeat,
			}
		}
	}

	response.Success(c, gin.H{
		"current": current,
		"history": history,
	})
}

// InstallAgent installs the agent on the server
func (h *ServerHandler) InstallAgent(c *gin.Context) {
	id := c.Param("id")

	var server model.Server
	if err := h.db.First(&server, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	if server.Status != "running" {
		response.BadRequest(c, "server is not running")
		return
	}

	panelAddr, err := agentPanelAddress(c.Request, h.deployer.GetPluginWebhookURL())
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Generate API key for agent
	apiKey := fmt.Sprintf("agent-%s-%d", server.IP, time.Now().Unix())

	// Install agent async to avoid frontend timeout
	go func() {
		if err := h.deployer.InstallAgent(&server, panelAddr, apiKey); err != nil {
			log.Printf("[InstallAgent] Failed for server %s: %v", server.Name, err)
			h.db.Model(&server).Updates(map[string]interface{}{
				"agent_installed": false,
				"error_msg":       fmt.Sprintf("[Agent] installation failed: %v", err),
			})
		} else {
			log.Printf("[InstallAgent] Success for server %s", server.Name)
			h.db.Model(&server).Update("error_msg", "")
		}
	}()

	response.SuccessWithMessage(c, "agent installation started", gin.H{"api_key": apiKey})
}

func agentPanelAddress(r *http.Request, configured string) (string, error) {
	candidate := strings.TrimSpace(configured)
	if candidate == "" {
		host := strings.TrimSpace(r.Host)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if requestFromLocalProxy(r) {
			if forwardedHost := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwardedHost != "" {
				host = forwardedHost
			}
			if forwardedProto := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])); forwardedProto == "http" || forwardedProto == "https" {
				scheme = forwardedProto
			}
		}
		if host == "" {
			return "", fmt.Errorf("panel address is unavailable")
		}
		candidate = scheme + "://" + host
	}

	parsed, err := url.Parse(candidate)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return "", fmt.Errorf("invalid panel address")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func requestFromLocalProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}
