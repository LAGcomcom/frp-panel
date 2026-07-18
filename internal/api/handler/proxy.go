package handler

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/accesscontrol"
	"github.com/frp-panel/frp-panel/internal/pkg/frpconfig"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// reservedPorts contains ports that must not be used as remote ports
var reservedPorts = map[int]bool{
	22: true, 25: true, 80: true, 443: true,
	3306: true, 5432: true, 6379: true, 8080: true, 8443: true,
	27017: true, 9090: true, 9100: true, 3000: true,
}

func validateRemotePort(db *gorm.DB, serverID uint, port int, excludeProxyID uint) error {
	if port < 1024 {
		return fmt.Errorf("端口 %d 为系统保留端口，不可使用", port)
	}
	if port > 65535 {
		return fmt.Errorf("端口号不能超过 65535")
	}
	if reservedPorts[port] {
		return fmt.Errorf("端口 %d 为常用服务端口，不可使用", port)
	}

	// Check against server's own ports
	var server model.Server
	if err := db.First(&server, serverID).Error; err == nil {
		if port == server.BindPort || port == server.DashboardPort ||
			port == server.VhostHTTPPort || port == server.VhostHTTPSPort {
			return fmt.Errorf("端口 %d 被服务器自身服务占用", port)
		}
	}

	// Check against other proxies on the same server
	var count int64
	query := db.Model(&model.Proxy{}).Where("server_id = ? AND remote_port = ?", serverID, port)
	if excludeProxyID > 0 {
		query = query.Where("id != ?", excludeProxyID)
	}
	query.Count(&count)
	if count > 0 {
		return fmt.Errorf("端口 %d 已被其他代理使用", port)
	}

	return nil
}

type ProxyHandler struct {
	db *gorm.DB
}

func NewProxyHandler(db *gorm.DB) *ProxyHandler {
	return &ProxyHandler{db: db}
}

type clientConfigAuth struct {
	Method string `json:"method"`
	Token  string `json:"token"`
}

type clientConfigTransport struct {
	TCPMux bool `json:"tcpMux"`
}

type clientConfigMetadata struct {
	APIKey   string `json:"apikey"`
	ServerID string `json:"server_id"`
}

type clientConfig struct {
	ServerID   uint                    `json:"serverId"`
	ServerName string                  `json:"serverName"`
	FRPVersion string                  `json:"frpVersion"`
	ServerAddr string                  `json:"serverAddr"`
	ServerPort int                     `json:"serverPort"`
	Auth       clientConfigAuth        `json:"auth"`
	Transport  clientConfigTransport   `json:"transport"`
	Metadatas  clientConfigMetadata    `json:"metadatas"`
	Proxies    []frpconfig.ProxyConfig `json:"proxies"`
}

func (h *ProxyHandler) GetClientConfigsByAPIKey(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		response.Unauthorized(c, "API key must be provided in a request header")
		return
	}

	apiKey := strings.TrimSpace(c.GetHeader("X-API-Key"))
	if apiKey == "" {
		fields := strings.Fields(c.GetHeader("Authorization"))
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			apiKey = fields[1]
		}
	}
	if apiKey == "" || len(apiKey) > 256 {
		response.Unauthorized(c, "invalid API key")
		return
	}

	user, err := accesscontrol.LoadUserByAPIKey(h.db, apiKey)
	if err != nil || user.Status != "active" {
		response.Unauthorized(c, "invalid API key")
		return
	}

	var proxies []model.Proxy
	if err := h.db.Where("user_id = ? AND enabled = ?", user.ID, true).
		Preload("Server").Order("server_id asc, id asc").Find(&proxies).Error; err != nil {
		response.InternalError(c, "failed to load client configs")
		return
	}

	grouped := make(map[uint][]model.Proxy)
	servers := make(map[uint]model.Server)
	serverOrder := make([]uint, 0)
	for _, proxy := range proxies {
		server := proxy.Server
		if server.ID == 0 || server.Status != "running" || !server.PluginAuthEnabled {
			continue
		}
		if _, exists := grouped[server.ID]; !exists {
			allowed, accessErr := accesscontrol.CanAccessServer(h.db, user, server.ID)
			if accessErr != nil {
				response.InternalError(c, "failed to check server access")
				return
			}
			if !allowed {
				continue
			}
			serverOrder = append(serverOrder, server.ID)
			servers[server.ID] = server
		}
		grouped[server.ID] = append(grouped[server.ID], proxy)
	}

	configs := make([]clientConfig, 0, len(serverOrder))
	for _, serverID := range serverOrder {
		server := servers[serverID]
		generated, buildErr := frpconfig.BuildFrpcConfig(&server, user, grouped[serverID])
		if buildErr != nil {
			response.InternalError(c, "failed to generate client config")
			return
		}
		configs = append(configs, clientConfig{
			ServerID: server.ID, ServerName: server.Name, FRPVersion: server.FrpVersion,
			ServerAddr: generated.ServerAddr, ServerPort: generated.ServerPort,
			Auth:      clientConfigAuth{Method: "token", Token: apiKey},
			Transport: clientConfigTransport{TCPMux: true},
			Metadatas: clientConfigMetadata{APIKey: apiKey, ServerID: strconv.FormatUint(uint64(server.ID), 10)},
			Proxies:   generated.Proxies,
		})
	}

	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	response.Success(c, gin.H{"generatedAt": time.Now().UTC(), "configs": configs})
}

func (h *ProxyHandler) requireServerAccess(c *gin.Context, userID, serverID uint) bool {
	user, err := accesscontrol.LoadUser(h.db, userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return false
	}
	allowed, err := accesscontrol.CanAccessServer(h.db, user, serverID)
	if err != nil {
		response.InternalError(c, "failed to check server access")
		return false
	}
	if !allowed {
		response.Forbidden(c, "当前用户组不能使用该节点")
		return false
	}
	return true
}

type CreateProxyRequest struct {
	ServerID       uint     `json:"server_id" binding:"required"`
	Name           string   `json:"name" binding:"required"`
	Type           string   `json:"type" binding:"required,oneof=tcp udp http https stcp xtcp"`
	LocalIP        string   `json:"local_ip"`
	LocalPort      int      `json:"local_port" binding:"required"`
	RemotePort     int      `json:"remote_port"`
	CustomDomains  []string `json:"custom_domains"`
	Subdomain      string   `json:"subdomain"`
	SecretKey      string   `json:"secret_key"`
	UseEncryption  bool     `json:"use_encryption"`
	UseCompression bool     `json:"use_compression"`
	BandwidthLimit int64    `json:"bandwidth_limit"`
}

func (h *ProxyHandler) CreateProxy(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req CreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.LocalIP == "" {
		req.LocalIP = "127.0.0.1"
	}

	// Check server exists and is running
	var server model.Server
	if err := h.db.First(&server, req.ServerID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.BadRequest(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}
	if server.Status != "running" {
		response.BadRequest(c, "server is not running")
		return
	}

	// Check user quota
	user, err := accesscontrol.LoadUser(h.db, userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}
	allowed, err := accesscontrol.CanAccessServer(h.db, user, req.ServerID)
	if err != nil {
		response.InternalError(c, "failed to check server access")
		return
	}
	if !allowed {
		response.Forbidden(c, "当前用户组不能使用该节点")
		return
	}
	maxBandwidth := accesscontrol.EffectiveBandwidth(h.db, user)
	if user.PlanID != nil {
		var plan model.Plan
		h.db.First(&plan, *user.PlanID)
		var currentProxies int64
		h.db.Model(&model.Proxy{}).Where("user_id = ?", userID).Count(&currentProxies)
		if int(currentProxies) >= plan.MaxProxies {
			response.BadRequest(c, fmt.Sprintf("proxy limit reached (%d/%d)", currentProxies, plan.MaxProxies))
			return
		}
	}

	// Validate bandwidth limit against plan maximum
	if req.BandwidthLimit > 0 && maxBandwidth > 0 && req.BandwidthLimit > maxBandwidth {
		response.BadRequest(c, fmt.Sprintf("带宽限制超过套餐上限（最大 %d KB/s）", maxBandwidth/1024))
		return
	}

	// Check name uniqueness on this server
	var count int64
	h.db.Model(&model.Proxy{}).Where("server_id = ? AND name = ?", req.ServerID, req.Name).Count(&count)
	if count > 0 {
		response.BadRequest(c, "proxy name already exists on this server")
		return
	}

	// Validate type-specific fields
	switch req.Type {
	case "tcp", "udp":
		if req.RemotePort == 0 {
			response.BadRequest(c, "remote_port is required for tcp/udp proxies")
			return
		}
		if err := validateRemotePort(h.db, req.ServerID, req.RemotePort, 0); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	case "http", "https":
		if len(req.CustomDomains) == 0 && req.Subdomain == "" {
			response.BadRequest(c, "custom_domains or subdomain is required for http/https proxies")
			return
		}
	case "stcp", "xtcp":
		if req.SecretKey == "" {
			response.BadRequest(c, "secret_key is required for stcp/xtcp proxies")
			return
		}
	}

	proxyName := fmt.Sprintf("%d_%s", userID, req.Name)

	customDomainsJSON := ""
	if len(req.CustomDomains) > 0 {
		b, _ := json.Marshal(req.CustomDomains)
		customDomainsJSON = string(b)
	}

	proxy := model.Proxy{
		UserID:         userID,
		ServerID:       req.ServerID,
		Name:           proxyName,
		Type:           req.Type,
		LocalIP:        req.LocalIP,
		LocalPort:      req.LocalPort,
		RemotePort:     req.RemotePort,
		CustomDomains:  customDomainsJSON,
		Subdomain:      req.Subdomain,
		SecretKey:      req.SecretKey,
		UseEncryption:  req.UseEncryption,
		UseCompression: req.UseCompression,
		BandwidthLimit: req.BandwidthLimit,
		Status:         "pending",
	}

	if err := h.db.Create(&proxy).Error; err != nil {
		response.InternalError(c, "failed to create proxy")
		return
	}

	// Proxy status is synced by the Poller from frps dashboard.
	// Users connect via Frpc-Desktop with their API Key; Poller matches by user_id + remote_port.

	response.SuccessWithMessage(c, "proxy created", proxy)
}

func (h *ProxyHandler) ListProxies(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	page := c.DefaultQuery("page", "1")
	size := c.DefaultQuery("size", "20")
	serverID := c.DefaultQuery("server_id", "")
	proxyType := c.DefaultQuery("type", "")
	status := c.DefaultQuery("status", "")

	var proxies []model.Proxy
	var total int64

	query := h.db.Model(&model.Proxy{}).Where("user_id = ?", userID)
	user, err := accesscontrol.LoadUser(h.db, userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}
	if accesscontrol.IsPlanGroupExpired(user) {
		query = query.Where("1 = 0")
	} else if user.GroupID != nil {
		query = query.Where("server_id IN (?)", h.db.Table("user_group_servers AS ugs").
			Select("ugs.server_id").Joins("JOIN servers AS s ON s.id = ugs.server_id AND s.deleted_at IS NULL").
			Where("ugs.user_group_id = ? AND s.plugin_auth_enabled = ?", *user.GroupID, true))
	}
	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}
	if proxyType != "" {
		query = query.Where("type = ?", proxyType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)

	offset := (parseInt(page) - 1) * parseInt(size)
	query.Offset(offset).Limit(parseInt(size)).Order("id desc").Preload("Server").Find(&proxies)

	response.Page(c, proxies, total, parseInt(page), parseInt(size))
}

func (h *ProxyHandler) GetProxy(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	proxyID := c.Param("id")

	var proxy model.Proxy
	if err := h.db.Where("id = ? AND user_id = ?", proxyID, userID).Preload("Server").First(&proxy).Error; err != nil {
		response.NotFound(c, "proxy not found")
		return
	}
	if !h.requireServerAccess(c, userID, proxy.ServerID) {
		return
	}

	response.Success(c, proxy)
}

func (h *ProxyHandler) UpdateProxy(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	proxyID := c.Param("id")

	var proxy model.Proxy
	if err := h.db.Where("id = ? AND user_id = ?", proxyID, userID).First(&proxy).Error; err != nil {
		response.NotFound(c, "proxy not found")
		return
	}
	if !h.requireServerAccess(c, userID, proxy.ServerID) {
		return
	}

	var req struct {
		LocalIP        string   `json:"local_ip"`
		LocalPort      int      `json:"local_port"`
		RemotePort     int      `json:"remote_port"`
		CustomDomains  []string `json:"custom_domains"`
		Subdomain      string   `json:"subdomain"`
		SecretKey      string   `json:"secret_key"`
		UseEncryption  *bool    `json:"use_encryption"`
		UseCompression *bool    `json:"use_compression"`
		BandwidthLimit int64    `json:"bandwidth_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.LocalIP != "" {
		updates["local_ip"] = req.LocalIP
	}
	if req.LocalPort > 0 {
		updates["local_port"] = req.LocalPort
	}
	if req.RemotePort > 0 {
		if err := validateRemotePort(h.db, proxy.ServerID, req.RemotePort, proxy.ID); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		updates["remote_port"] = req.RemotePort
	}
	if len(req.CustomDomains) > 0 {
		b, _ := json.Marshal(req.CustomDomains)
		updates["custom_domains"] = string(b)
	}
	if req.Subdomain != "" {
		updates["subdomain"] = req.Subdomain
	}
	if req.SecretKey != "" {
		updates["secret_key"] = req.SecretKey
	}
	if req.UseEncryption != nil {
		updates["use_encryption"] = *req.UseEncryption
	}
	if req.UseCompression != nil {
		updates["use_compression"] = *req.UseCompression
	}
	if req.BandwidthLimit > 0 {
		// Validate bandwidth against plan
		proxyUser, err := accesscontrol.LoadUser(h.db, proxy.UserID)
		if err != nil {
			response.NotFound(c, "user not found")
			return
		}
		maxBandwidth := accesscontrol.EffectiveBandwidth(h.db, proxyUser)
		if maxBandwidth > 0 && req.BandwidthLimit > maxBandwidth {
			response.BadRequest(c, fmt.Sprintf("带宽限制超过套餐上限（最大 %d KB/s）", maxBandwidth/1024))
			return
		}
		updates["bandwidth_limit"] = req.BandwidthLimit
	}

	if err := h.db.Model(&proxy).Updates(updates).Error; err != nil {
		response.InternalError(c, "failed to update proxy")
		return
	}

	response.SuccessWithMessage(c, "proxy updated", nil)
}

func (h *ProxyHandler) DeleteProxy(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	proxyID := c.Param("id")

	var proxy model.Proxy
	if err := h.db.Where("id = ? AND user_id = ?", proxyID, userID).First(&proxy).Error; err != nil {
		response.NotFound(c, "proxy not found")
		return
	}
	if !h.requireServerAccess(c, userID, proxy.ServerID) {
		return
	}

	if err := h.db.Delete(&proxy).Error; err != nil {
		response.InternalError(c, "failed to delete proxy")
		return
	}

	response.SuccessWithMessage(c, "proxy deleted", nil)
}

func (h *ProxyHandler) EnableProxy(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	proxyID := c.Param("id")

	var proxy model.Proxy
	if err := h.db.Where("id = ? AND user_id = ?", proxyID, userID).First(&proxy).Error; err != nil {
		response.NotFound(c, "proxy not found")
		return
	}
	if !h.requireServerAccess(c, userID, proxy.ServerID) {
		return
	}

	h.db.Model(&proxy).Update("enabled", true)
	response.SuccessWithMessage(c, "proxy enabled", nil)
}

func (h *ProxyHandler) DisableProxy(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	proxyID := c.Param("id")

	var proxy model.Proxy
	if err := h.db.Where("id = ? AND user_id = ?", proxyID, userID).First(&proxy).Error; err != nil {
		response.NotFound(c, "proxy not found")
		return
	}
	if !h.requireServerAccess(c, userID, proxy.ServerID) {
		return
	}

	h.db.Model(&proxy).Update("enabled", false)
	response.SuccessWithMessage(c, "proxy disabled", nil)
}

// GetFrpcConfig returns the frpc configuration for a specific server
func (h *ProxyHandler) GetFrpcConfig(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	serverID := c.Param("server_id")

	// Get server
	var server model.Server
	if err := h.db.First(&server, serverID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}
	if !server.PluginAuthEnabled {
		response.BadRequest(c, "该节点需要由管理员重新部署后才能生成安全配置")
		return
	}

	user, err := accesscontrol.LoadUser(h.db, userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}
	allowed, err := accesscontrol.CanAccessServer(h.db, user, server.ID)
	if err != nil {
		response.InternalError(c, "failed to check server access")
		return
	}
	if !allowed {
		response.Forbidden(c, "当前用户组不能使用该节点")
		return
	}

	// Get user's proxies on this server
	var proxies []model.Proxy
	h.db.Where("user_id = ? AND server_id = ? AND enabled = ?", userID, serverID, true).Find(&proxies)

	if len(proxies) == 0 {
		response.BadRequest(c, "no proxies found on this server")
		return
	}

	// Generate frpc config
	config, err := frpconfig.GenerateFrpcConfig(&server, user, proxies)
	if err != nil {
		response.InternalError(c, "failed to generate config")
		return
	}

	response.Success(c, gin.H{
		"config":    config,
		"server_ip": server.IP,
		"bind_port": server.BindPort,
	})
}

// Admin: List all proxies
func (h *ProxyHandler) AdminListProxies(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	size := c.DefaultQuery("size", "20")
	serverID := c.DefaultQuery("server_id", "")
	userID := c.DefaultQuery("user_id", "")
	proxyType := c.DefaultQuery("type", "")
	status := c.DefaultQuery("status", "")

	var proxies []model.Proxy
	var total int64

	query := h.db.Model(&model.Proxy{})
	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if proxyType != "" {
		query = query.Where("type = ?", proxyType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)

	offset := (parseInt(page) - 1) * parseInt(size)
	query.Offset(offset).Limit(parseInt(size)).Order("id desc").Preload("User").Preload("Server").Find(&proxies)

	response.Page(c, proxies, total, parseInt(page), parseInt(size))
}

func (h *ProxyHandler) AdminEnableProxy(c *gin.Context) {
	proxy, ok := h.loadAdminProxy(c)
	if !ok {
		return
	}
	if err := h.db.Model(proxy).Update("enabled", true).Error; err != nil {
		response.InternalError(c, "failed to enable proxy")
		return
	}
	response.SuccessWithMessage(c, "代理已启用", nil)
}

func (h *ProxyHandler) AdminDisableProxy(c *gin.Context) {
	proxy, ok := h.loadAdminProxy(c)
	if !ok {
		return
	}
	if err := h.db.Model(proxy).Update("enabled", false).Error; err != nil {
		response.InternalError(c, "failed to disable proxy")
		return
	}
	response.SuccessWithMessage(c, "代理已禁用", nil)
}

func (h *ProxyHandler) AdminDeleteProxy(c *gin.Context) {
	proxy, ok := h.loadAdminProxy(c)
	if !ok {
		return
	}
	if err := h.db.Delete(proxy).Error; err != nil {
		response.InternalError(c, "failed to delete proxy")
		return
	}
	response.SuccessWithMessage(c, "代理已删除", nil)
}

func (h *ProxyHandler) loadAdminProxy(c *gin.Context) (*model.Proxy, bool) {
	var proxy model.Proxy
	if err := h.db.Preload("User").First(&proxy, c.Param("id")).Error; err != nil {
		response.NotFound(c, "proxy not found")
		return nil, false
	}
	if !authorizeManageableUser(c, &proxy.User) {
		return nil, false
	}
	return &proxy, true
}

// GetServerPorts returns used ports and server's own ports for a given server
func (h *ProxyHandler) GetServerPorts(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	serverID := c.Param("server_id")

	var server model.Server
	if err := h.db.First(&server, serverID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "server not found")
		} else {
			response.InternalError(c, fmt.Sprintf("database error: %v", err))
		}
		return
	}
	user, err := accesscontrol.LoadUser(h.db, userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}
	allowed, err := accesscontrol.CanAccessServer(h.db, user, server.ID)
	if err != nil {
		response.InternalError(c, "failed to check server access")
		return
	}
	if !allowed {
		response.Forbidden(c, "当前用户组不能使用该节点")
		return
	}

	// Get all used remote ports on this server
	var proxies []model.Proxy
	h.db.Where("server_id = ? AND remote_port > 0", serverID).Select("remote_port").Find(&proxies)

	usedPorts := make([]int, 0, len(proxies))
	for _, p := range proxies {
		usedPorts = append(usedPorts, p.RemotePort)
	}

	response.Success(c, gin.H{
		"used_ports":     usedPorts,
		"server_ports":   []int{server.BindPort, server.DashboardPort, server.VhostHTTPPort, server.VhostHTTPSPort},
		"min_port":       1024,
		"max_port":       65535,
		"reserved_ports": []int{22, 25, 80, 443, 3306, 5432, 6379, 8080, 8443, 27017, 9090, 9100, 3000},
	})
}
