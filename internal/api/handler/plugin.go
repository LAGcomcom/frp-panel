package handler

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
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

type PluginHandler struct {
	db          *gorm.DB
	serverToken string
}

func NewPluginHandler(db *gorm.DB, serverToken string) *PluginHandler {
	return &PluginHandler{db: db, serverToken: serverToken}
}

// frps httpPlugin webhook payload
type PluginRequest struct {
	Version string      `json:"version"`
	Op      string      `json:"op"`
	Content interface{} `json:"content"`
}

type PluginResponse struct {
	Reject       bool        `json:"reject"`
	RejectReason string      `json:"reject_reason,omitempty"`
	Unchange     bool        `json:"unchange"`
	Content      interface{} `json:"content,omitempty"`
}

func (h *PluginHandler) HandleWebhook(c *gin.Context) {
	h.handleWebhook(c, nil)
}

func (h *PluginHandler) HandleServerWebhook(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("server_id"), 10, 64)
	if err != nil {
		c.JSON(404, gin.H{"message": "not found"})
		return
	}
	var server model.Server
	if err := h.db.First(&server, uint(serverID)).Error; err != nil ||
		subtle.ConstantTimeCompare([]byte(server.PluginSecret), []byte(c.Param("plugin_secret"))) != 1 {
		c.JSON(404, gin.H{"message": "not found"})
		return
	}
	if !server.PluginAuthEnabled {
		c.JSON(200, PluginResponse{Reject: true, RejectReason: "node authentication is not enabled"})
		return
	}
	h.handleWebhook(c, &server)
}

func (h *PluginHandler) handleWebhook(c *gin.Context, server *model.Server) {
	var req PluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if server != nil {
		log.Printf("[Plugin] op=%s server_id=%d", req.Op, server.ID)
	} else {
		log.Printf("[Plugin] op=%s legacy_webhook=true", req.Op)
	}

	switch req.Op {
	case "Login":
		h.handleLogin(c, req, server)
	case "NewProxy":
		h.handleNewProxy(c, req, server)
	case "CloseProxy":
		h.handleCloseProxy(c, req)
	case "Ping":
		h.handlePing(c, req, server)
	default:
		c.JSON(200, PluginResponse{Unchange: true})
	}
}

func (h *PluginHandler) handleLogin(c *gin.Context, req PluginRequest, server *model.Server) {
	content, ok := req.Content.(map[string]interface{})
	if !ok {
		c.JSON(200, PluginResponse{Unchange: true})
		return
	}

	timestamp := toInt64(content["timestamp"])

	// Try metadata first (apikey field), then fall back to token/user fields
	var apiKey string
	if metas, ok := content["metas"].(map[string]interface{}); ok {
		apiKey, _ = metas["apikey"].(string)
	}
	if apiKey == "" {
		apiKey, _ = content["token"].(string)
	}
	if apiKey == "" {
		apiKey, _ = content["login_user"].(string)
	}
	if apiKey == "" {
		apiKey, _ = content["user"].(string)
	}

	if apiKey == "" {
		rejectPlugin(c, "invalid API key")
		return
	}

	user, err := accesscontrol.LoadUserByAPIKey(h.db, apiKey)
	if err != nil {
		rejectPlugin(c, "invalid API key")
		return
	}

	if user.Status != "active" {
		rejectPlugin(c, "account is not active")
		return
	}

	if server != nil {
		allowed, err := accesscontrol.CanAccessServer(h.db, user, server.ID)
		if err != nil || !allowed {
			rejectPlugin(c, "node is not available for this user group")
			return
		}
		if timestamp <= 0 {
			rejectPlugin(c, "invalid login timestamp")
			return
		}
		content["privilege_key"] = makePrivilegeKey(server.Token, timestamp)
		content["user"] = "user_" + strconv.FormatUint(uint64(user.ID), 10)
		c.JSON(200, PluginResponse{Unchange: false, Content: content})
		return
	}

	log.Printf("[Plugin] legacy login authenticated user_id=%d", user.ID)
	c.JSON(200, PluginResponse{Unchange: true})
}

func makePrivilegeKey(key string, timestamp int64) string {
	hash := md5.Sum([]byte(key + strconv.FormatInt(timestamp, 10)))
	return hex.EncodeToString(hash[:])
}

func (h *PluginHandler) handleNewProxy(c *gin.Context, req PluginRequest, server *model.Server) {
	content, ok := req.Content.(map[string]interface{})
	if !ok {
		c.JSON(200, PluginResponse{Unchange: true})
		return
	}

	proxyName, _ := content["proxy_name"].(string)
	proxyType, _ := content["proxy_type"].(string)
	apiKey := apiKeyFromPluginContent(content)
	log.Printf("[Plugin] NewProxy: name=%s type=%s", proxyName, proxyType)

	// Try to find user by API Key
	var userObj *model.User
	if apiKey != "" {
		userObj, _ = accesscontrol.LoadUserByAPIKey(h.db, apiKey)
	}
	if userObj == nil {
		var proxy model.Proxy
		query := h.db.Where("name = ?", proxyName)
		if server != nil {
			query = query.Where("server_id = ?", server.ID)
		}
		if err := query.First(&proxy).Error; err == nil {
			userObj, _ = accesscontrol.LoadUser(h.db, proxy.UserID)
		}
	}
	if userObj == nil || userObj.Status != "active" {
		rejectPlugin(c, "invalid user")
		return
	}

	if server != nil {
		allowed, err := accesscontrol.CanAccessServer(h.db, userObj, server.ID)
		if err != nil || !allowed {
			rejectPlugin(c, "node is not available for this user group")
			return
		}
		var proxy model.Proxy
		if err := h.db.Where("name = ? AND user_id = ? AND server_id = ? AND enabled = ?",
			proxyName, userObj.ID, server.ID, true).First(&proxy).Error; err != nil {
			rejectPlugin(c, "proxy is not registered for this node")
			return
		}
		if proxy.Type != proxyType {
			rejectPlugin(c, "proxy configuration does not match the panel")
			return
		}
		enforceStoredProxyConfig(content, &proxy)
		bandwidth := accesscontrol.EffectiveBandwidth(h.db, userObj)
		if proxy.BandwidthLimit > 0 && proxy.BandwidthLimit < bandwidth {
			bandwidth = proxy.BandwidthLimit
		}
		if bandwidth > 0 {
			content["bandwidth_limit"] = frpconfig.FormatBandwidth(bandwidth)
			content["bandwidth_limit_mode"] = "server"
		}
	}

	// Check user's traffic quota
	if userObj.ID > 0 && userObj.PlanID != nil {
		if plan := userObj.Plan; plan != nil && plan.MaxTraffic > 0 {
			now := time.Now()
			monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			var monthlyTraffic int64
			h.db.Model(&model.TrafficDaily{}).
				Where("user_id = ? AND date >= ?", userObj.ID, monthStart.Format("2006-01-02")).
				Select("COALESCE(SUM(traffic_in + traffic_out), 0)").
				Scan(&monthlyTraffic)

			if monthlyTraffic >= plan.MaxTraffic {
				c.JSON(200, PluginResponse{
					Reject:       true,
					RejectReason: "traffic quota exceeded",
				})
				return
			}
		}
	}

	if server != nil {
		c.JSON(200, PluginResponse{Unchange: false, Content: content})
		return
	}
	c.JSON(200, PluginResponse{Unchange: true})
}

func enforceStoredProxyConfig(content map[string]interface{}, proxy *model.Proxy) {
	content["use_encryption"] = proxy.UseEncryption
	content["use_compression"] = proxy.UseCompression
	delete(content, "group")
	delete(content, "group_key")

	switch proxy.Type {
	case "tcp", "udp":
		content["remote_port"] = proxy.RemotePort
	case "http", "https":
		var domains []string
		if proxy.CustomDomains != "" {
			if err := json.Unmarshal([]byte(proxy.CustomDomains), &domains); err != nil {
				for _, domain := range strings.Split(proxy.CustomDomains, ",") {
					if domain = strings.TrimSpace(domain); domain != "" {
						domains = append(domains, domain)
					}
				}
			}
		}
		content["custom_domains"] = domains
		content["subdomain"] = proxy.Subdomain
	case "stcp", "xtcp":
		content["sk"] = proxy.SecretKey
	}
}

func apiKeyFromPluginContent(content map[string]interface{}) string {
	if metas, ok := content["metas"].(map[string]interface{}); ok {
		if apiKey, _ := metas["apikey"].(string); apiKey != "" {
			return apiKey
		}
	}
	if userInfo, ok := content["user"].(map[string]interface{}); ok {
		if metas, ok := userInfo["metas"].(map[string]interface{}); ok {
			if apiKey, _ := metas["apikey"].(string); apiKey != "" {
				return apiKey
			}
		}
	}
	if token, _ := content["token"].(string); token != "" {
		return token
	}
	return ""
}

func rejectPlugin(c *gin.Context, reason string) {
	c.JSON(200, PluginResponse{Reject: true, RejectReason: reason})
}

func (h *PluginHandler) handleCloseProxy(c *gin.Context, req PluginRequest) {
	content, ok := req.Content.(map[string]interface{})
	if !ok {
		c.JSON(200, PluginResponse{Unchange: true})
		return
	}

	proxyName, _ := content["proxy_name"].(string)
	log.Printf("[Plugin] CloseProxy: name=%s", proxyName)

	c.JSON(200, PluginResponse{Unchange: true})
}

func (h *PluginHandler) handlePing(c *gin.Context, req PluginRequest, server *model.Server) {
	content, ok := req.Content.(map[string]interface{})
	if !ok {
		c.JSON(200, PluginResponse{Unchange: true})
		return
	}
	if server != nil {
		apiKey := apiKeyFromPluginContent(content)
		if apiKey != "" {
			userObj, err := accesscontrol.LoadUserByAPIKey(h.db, apiKey)
			if err != nil || userObj.Status != "active" {
				rejectPlugin(c, "invalid user")
				return
			}
			allowed, err := accesscontrol.CanAccessServer(h.db, userObj, server.ID)
			if err != nil || !allowed {
				rejectPlugin(c, "node is not available for this user group")
				return
			}
		}
	}

	// Try token first, then user field
	user, _ := content["token"].(string)
	if user == "" {
		user, _ = content["user"].(string)
	}
	proxyName, _ := content["proxy_name"].(string)

	// Extract traffic data from ping
	trafficIn := toInt64(content["traffic_in"])
	trafficOut := toInt64(content["traffic_out"])
	curConns := toInt64(content["cur_conns"])

	if proxyName == "" || (trafficIn == 0 && trafficOut == 0) {
		c.JSON(200, PluginResponse{Unchange: true})
		return
	}

	// Find proxy in DB
	var proxy model.Proxy
	if err := h.db.Where("name = ?", proxyName).First(&proxy).Error; err != nil {
		// Try to find proxy by user's API Key
		if user != "" {
			var userObj model.User
			if err := h.db.Where("api_key = ?", user).First(&userObj).Error; err == nil {
				h.db.Where("user_id = ? AND status = ?", userObj.ID, "running").First(&proxy)
			}
		}
		if proxy.ID == 0 {
			c.JSON(200, PluginResponse{Unchange: true})
			return
		}
	}

	// Calculate delta
	deltaIn := trafficIn - proxy.TrafficIn
	deltaOut := trafficOut - proxy.TrafficOut
	if deltaIn < 0 {
		deltaIn = 0
	}
	if deltaOut < 0 {
		deltaOut = 0
	}

	if deltaIn > 0 || deltaOut > 0 {
		// Update proxy cumulative counters
		h.db.Model(&proxy).Updates(map[string]interface{}{
			"traffic_in":  trafficIn,
			"traffic_out": trafficOut,
		})

		// Write TrafficLog
		h.db.Create(&model.TrafficLog{
			ProxyID:    proxy.ID,
			UserID:     proxy.UserID,
			ServerID:   proxy.ServerID,
			TrafficIn:  deltaIn,
			TrafficOut: deltaOut,
			RecordedAt: time.Now(),
		})

		// Upsert TrafficDaily
		today := time.Now().Format("2006-01-02")
		var daily model.TrafficDaily
		if err := h.db.Where("proxy_id = ? AND user_id = ? AND date = ?",
			proxy.ID, proxy.UserID, today).First(&daily).Error; err == nil {
			h.db.Model(&daily).Updates(map[string]interface{}{
				"traffic_in":  daily.TrafficIn + deltaIn,
				"traffic_out": daily.TrafficOut + deltaOut,
			})
		} else {
			h.db.Create(&model.TrafficDaily{
				ProxyID:    proxy.ID,
				UserID:     proxy.UserID,
				Date:       today,
				TrafficIn:  deltaIn,
				TrafficOut: deltaOut,
			})
		}
	}

	log.Printf("[Plugin] Ping: user=%s proxy=%s traffic_in=%d traffic_out=%d cur_conns=%d",
		user, proxyName, trafficIn, trafficOut, curConns)

	c.JSON(200, PluginResponse{Unchange: true})
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
	}
	return 0
}
