package handler

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
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
	var req PluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	contentJSON, _ := json.Marshal(req.Content)
	log.Printf("[Plugin] op=%s content=%s", req.Op, string(contentJSON))

	switch req.Op {
	case "Login":
		h.handleLogin(c, req)
	case "NewProxy":
		h.handleNewProxy(c, req)
	case "CloseProxy":
		h.handleCloseProxy(c, req)
	case "Ping":
		h.handlePing(c, req)
	default:
		c.JSON(200, PluginResponse{Unchange: true})
	}
}

func (h *PluginHandler) handleLogin(c *gin.Context, req PluginRequest) {
	content, ok := req.Content.(map[string]interface{})
	if !ok {
		c.JSON(200, PluginResponse{Unchange: true})
		return
	}

	contentJSON, _ := json.Marshal(content)
	log.Printf("[Plugin] Login content: %s", string(contentJSON))

	privilegeKey, _ := content["privilege_key"].(string)
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

	// If no API key in fields, try to verify via privilege_key
	if apiKey == "" && privilegeKey != "" && timestamp > 0 {
		apiKey = h.verifyPrivilegeKey(privilegeKey, timestamp)
	}

	log.Printf("[Plugin] Login: apiKey=%s privilege_key=%s", apiKey, privilegeKey)

	if apiKey == "" {
		c.JSON(200, PluginResponse{
			Reject:       true,
			RejectReason: "invalid API key",
		})
		return
	}

	// Validate API Key
	var user model.User
	if err := h.db.Where("api_key = ?", apiKey).First(&user).Error; err != nil {
		c.JSON(200, PluginResponse{
			Reject:       true,
			RejectReason: "invalid API key",
		})
		return
	}

	if user.Status == "banned" {
		c.JSON(200, PluginResponse{
			Reject:       true,
			RejectReason: "account banned",
		})
		return
	}

	log.Printf("[Plugin] Login: user %s authenticated", apiKey)
	c.JSON(200, PluginResponse{Unchange: true})
}

// verifyPrivilegeKey checks the privilege_key against all stored API keys
// privilege_key = MD5(apiKey + timestamp)
func (h *PluginHandler) verifyPrivilegeKey(privilegeKey string, timestamp int64) string {
	var users []model.User
	h.db.Select("api_key").Where("api_key != '' AND status != 'banned'").Find(&users)

	timestampStr := strconv.FormatInt(timestamp, 10)
	for _, user := range users {
		if verifyPrivilege(user.APIKey, timestampStr, privilegeKey) {
			return user.APIKey
		}
	}
	return ""
}

func verifyPrivilege(key, timestamp, expectedKey string) bool {
	hash := md5.Sum([]byte(key + timestamp))
	computed := hex.EncodeToString(hash[:])
	return computed == expectedKey
}

func (h *PluginHandler) handleNewProxy(c *gin.Context, req PluginRequest) {
	content, ok := req.Content.(map[string]interface{})
	if !ok {
		c.JSON(200, PluginResponse{Unchange: true})
		return
	}

	proxyName, _ := content["proxy_name"].(string)
	proxyType, _ := content["proxy_type"].(string)
	// Try token first, then user field
	userLogin, _ := content["token"].(string)
	if userLogin == "" {
		userLogin, _ = content["user"].(string)
	}

	log.Printf("[Plugin] NewProxy: name=%s type=%s user=%s", proxyName, proxyType, userLogin)

	// Try to find user by API Key
	var userObj model.User
	if userLogin != "" {
		if err := h.db.Where("api_key = ?", userLogin).First(&userObj).Error; err != nil {
			// Fallback: try by proxy name to find owner
			var proxy model.Proxy
			if err := h.db.Where("name = ?", proxyName).First(&proxy).Error; err == nil {
				h.db.First(&userObj, proxy.UserID)
			}
		}
	}

	// Check user's traffic quota
	if userObj.ID > 0 && userObj.PlanID != nil {
		var plan model.Plan
		if err := h.db.First(&plan, userObj.PlanID).Error; err == nil && plan.MaxTraffic > 0 {
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

	c.JSON(200, PluginResponse{Unchange: true})
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

func (h *PluginHandler) handlePing(c *gin.Context, req PluginRequest) {
	content, ok := req.Content.(map[string]interface{})
	if !ok {
		c.JSON(200, PluginResponse{Unchange: true})
		return
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
