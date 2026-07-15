package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/service/monitor"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type WSMessage struct {
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
}

type WebSocketHandler struct {
	db         *gorm.DB
	clients    map[*websocket.Conn]bool
	userConns  map[uint]map[*websocket.Conn]bool // userID -> connections
	mu         sync.RWMutex
	stopCh     chan struct{}
}

func NewWebSocketHandler(db *gorm.DB) *WebSocketHandler {
	h := &WebSocketHandler{
		db:        db,
		clients:   make(map[*websocket.Conn]bool),
		userConns: make(map[uint]map[*websocket.Conn]bool),
		stopCh:    make(chan struct{}),
	}
	go h.broadcastLoop()
	return h
}

// HandleWebSocket handles admin WebSocket connections
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Register client
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	log.Printf("[WS] Client connected, total: %d", len(h.clients))

	// Send initial data
	h.sendInitialData(conn)

	// Read messages (keep connection alive)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

	// Unregister client
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()

	log.Printf("[WS] Client disconnected, total: %d", len(h.clients))
}

func (h *WebSocketHandler) sendInitialData(conn *websocket.Conn) {
	// Send server list
	var servers []model.Server
	h.db.Find(&servers)

	// Mask sensitive fields
	for i := range servers {
		servers[i].Token = "***"
		servers[i].PluginSecret = "***"
		servers[i].SSHPassword = ""
		servers[i].SSHPrivateKey = ""
		servers[i].DashboardPassword = "***"
	}

	conn.WriteJSON(WSMessage{
		Type: "servers",
		Data: servers,
	})

	// Send stats
	h.sendStats(conn)
}

func (h *WebSocketHandler) sendStats(conn *websocket.Conn) {
	var serverCount, runningServers int64
	h.db.Model(&model.Server{}).Count(&serverCount)
	h.db.Model(&model.Server{}).Where("status = ?", "running").Count(&runningServers)

	var userCount int64
	h.db.Model(&model.User{}).Count(&userCount)

	var proxyCount, runningProxies int64
	h.db.Model(&model.Proxy{}).Count(&proxyCount)
	h.db.Model(&model.Proxy{}).Where("status = ?", "running").Count(&runningProxies)

	conn.WriteJSON(WSMessage{
		Type: "stats",
		Data: gin.H{
			"servers":        serverCount,
			"running_servers": runningServers,
			"users":          userCount,
			"proxies":        proxyCount,
			"running_proxies": runningProxies,
		},
	})
}

func (h *WebSocketHandler) broadcastLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.broadcast()
		case <-h.stopCh:
			return
		}
	}
}

func (h *WebSocketHandler) broadcast() {
	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for conn := range h.clients {
		clients = append(clients, conn)
	}
	h.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	// Prepare stats message
	var serverCount, runningServers int64
	h.db.Model(&model.Server{}).Count(&serverCount)
	h.db.Model(&model.Server{}).Where("status = ?", "running").Count(&runningServers)

	var userCount int64
	h.db.Model(&model.User{}).Count(&userCount)

	var proxyCount, runningProxies int64
	h.db.Model(&model.Proxy{}).Count(&proxyCount)
	h.db.Model(&model.Proxy{}).Where("status = ?", "running").Count(&runningProxies)

	statsMsg := WSMessage{
		Type: "stats",
		Data: gin.H{
			"servers":         serverCount,
			"running_servers": runningServers,
			"users":           userCount,
			"proxies":         proxyCount,
			"running_proxies": runningProxies,
			"timestamp":       time.Now().Unix(),
		},
	}

	data, _ := json.Marshal(statsMsg)

	// Broadcast to all clients
	for _, conn := range clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
		}
	}
}

// Stop stops the broadcast loop
func (h *WebSocketHandler) Stop() {
	close(h.stopCh)
}

// HandleUserWebSocket handles user WebSocket connections for notifications
func (h *WebSocketHandler) HandleUserWebSocket(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	uid := userID.(uint)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] User upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Register user connection
	h.mu.Lock()
	if h.userConns[uid] == nil {
		h.userConns[uid] = make(map[*websocket.Conn]bool)
	}
	h.userConns[uid][conn] = true
	h.mu.Unlock()

	log.Printf("[WS] User %d connected, total user conns: %d", uid, len(h.userConns[uid]))

	// Send unread count on connect
	var unreadCount int64
	h.db.Model(&monitor.Alert{}).Where("user_id = ? AND is_read = false", uid).Count(&unreadCount)
	conn.WriteJSON(WSMessage{
		Type: "unread_count",
		Data: gin.H{"count": unreadCount},
	})

	// Read messages (keep connection alive)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

	// Unregister
	h.mu.Lock()
	delete(h.userConns[uid], conn)
	if len(h.userConns[uid]) == 0 {
		delete(h.userConns, uid)
	}
	h.mu.Unlock()

	log.Printf("[WS] User %d disconnected", uid)
}

// NotifyUser sends a notification to a specific user via WebSocket
func (h *WebSocketHandler) NotifyUser(userID uint, alert monitor.Alert) {
	h.mu.RLock()
	conns := h.userConns[userID]
	h.mu.RUnlock()

	if len(conns) == 0 {
		return
	}

	msg := WSMessage{
		Type: "notification",
		Data: alert,
	}
	data, _ := json.Marshal(msg)

	h.mu.RLock()
	for conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			h.mu.Lock()
			delete(conns, conn)
			h.mu.Unlock()
		}
	}
	h.mu.RUnlock()
}

// NotifyAllUsers sends a notification to all connected users
func (h *WebSocketHandler) NotifyAllUsers(alert monitor.Alert) {
	h.mu.RLock()
	var allConns []*websocket.Conn
	for _, conns := range h.userConns {
		for conn := range conns {
			allConns = append(allConns, conn)
		}
	}
	h.mu.RUnlock()

	msg := WSMessage{
		Type: "notification",
		Data: alert,
	}
	data, _ := json.Marshal(msg)

	for _, conn := range allConns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
		}
	}
}
