package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"gorm.io/gorm"
)

type Poller struct {
	db       *gorm.DB
	client   *http.Client
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
	polling  atomic.Bool
	pollFn   func(*model.Server)
}

func NewPoller(db *gorm.DB, interval time.Duration) *Poller {
	p := &Poller{
		db: db,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		interval: interval,
		stopCh:   make(chan struct{}),
	}
	p.pollFn = p.pollServer
	return p
}

func (p *Poller) Start() {
	log.Printf("[Poller] Starting with interval %s", p.interval)
	p.wg.Add(1)
	go p.loop()
}

func (p *Poller) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	log.Printf("[Poller] Stopped")
}

func (p *Poller) loop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Run immediately on start
	p.pollAll()

	for {
		select {
		case <-ticker.C:
			p.pollAll()
		case <-p.stopCh:
			return
		}
	}
}

func (p *Poller) pollAll() {
	if !p.polling.CompareAndSwap(false, true) {
		log.Printf("[Poller] Previous poll is still running; skipping this interval")
		return
	}
	defer p.polling.Store(false)

	var servers []model.Server
	p.db.Where("status = ?", "running").Find(&servers)

	const maxConcurrentPolls = 8
	sem := make(chan struct{}, maxConcurrentPolls)
	var wg sync.WaitGroup
	for i := range servers {
		server := &servers[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			p.pollFn(server)
		}()
	}
	wg.Wait()
}

func (p *Poller) pollServer(server *model.Server) {
	baseURL := fmt.Sprintf("http://%s:%d", server.IP, server.DashboardPort)

	// Poll server info
	info, err := p.fetchServerInfo(baseURL, server.DashboardUser, server.DashboardPassword)
	if err != nil {
		log.Printf("[Poller] Failed to poll server %s: %v", server.Name, err)
		// Check if server is actually down
		_, healthErr := p.fetchHealthz(baseURL)
		if healthErr != nil {
			p.db.Model(server).Updates(map[string]interface{}{
				"status":    "error",
				"error_msg": fmt.Sprintf("unreachable: %v", healthErr),
			})
		}
		return
	}

	// Update server stats
	updates := map[string]interface{}{
		"client_count":   info.ClientCounts,
		"proxy_count":    info.ProxyTypeCounts,
		"last_heartbeat": time.Now(),
		"error_msg":      "",
		"status":         "running",
	}

	p.db.Model(server).Updates(updates)

	// Poll traffic data for each proxy
	p.pollTraffic(baseURL, server, server.DashboardUser, server.DashboardPassword)
}

type ServerInfoResp struct {
	Version         string         `json:"version"`
	ClientCounts    int            `json:"client_counts"`
	ProxyTypeCounts int            `json:"proxy_type_counts"`
	TodayTrafficIn  int64          `json:"today_traffic_in"`
	TodayTrafficOut int64          `json:"today_traffic_out"`
	CurConns        int            `json:"cur_conns"`
	ProxyTypeCount  map[string]int `json:"proxy_type_count"`
}

func (p *Poller) fetchServerInfo(baseURL, user, password string) (*ServerInfoResp, error) {
	req, err := http.NewRequest("GET", baseURL+"/api/serverinfo", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, password)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info ServerInfoResp
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

func (p *Poller) pollTraffic(baseURL string, server *model.Server, user, password string) {
	// Get all proxy types
	types := []string{"tcp", "udp", "http", "https", "stcp", "xtcp", "tcpmux"}

	// Track all proxy names found in frps
	foundInFrps := make(map[string]bool)

	for _, proxyType := range types {
		proxies, err := p.fetchProxies(baseURL, proxyType, user, password)
		if err != nil {
			continue
		}

		for _, proxyInfo := range proxies {
			foundInFrps[proxyInfo.Name] = true

			// Update proxy status in DB - try exact match first, then fallback
			var proxy model.Proxy
			matched := false

			// 1. Exact match
			if err := p.db.Where("server_id = ? AND name = ?", server.ID, proxyInfo.Name).First(&proxy).Error; err == nil {
				matched = true
			}

			// 2. frps name is {api_key}.{proxy_name}, strip prefix and try {userID}_{proxy_name}
			if !matched && proxyInfo.User != "" {
				var user model.User
				if err := p.db.Where("api_key = ?", proxyInfo.User).First(&user).Error; err == nil {
					prefix := proxyInfo.User + "."
					bareName := proxyInfo.Name
					if len(bareName) > len(prefix) && bareName[:len(prefix)] == prefix {
						bareName = bareName[len(prefix):]
					}
					// Try {userID}_{bareName}
					panelName := fmt.Sprintf("%d_%s", user.ID, bareName)
					if err := p.db.Where("server_id = ? AND name = ?", server.ID, panelName).First(&proxy).Error; err == nil {
						matched = true
						foundInFrps[panelName] = true
					}
					// Try bareName directly
					if !matched {
						if err := p.db.Where("server_id = ? AND name = ?", server.ID, bareName).First(&proxy).Error; err == nil {
							matched = true
							foundInFrps[bareName] = true
						}
					}
					// 3. Match by user's proxy + remote port (for Frpc-Desktop clients)
					if !matched && proxyInfo.Conf.RemotePort > 0 {
						if err := p.db.Where("server_id = ? AND user_id = ? AND remote_port = ?",
							server.ID, user.ID, proxyInfo.Conf.RemotePort).First(&proxy).Error; err == nil {
							matched = true
							foundInFrps[proxy.Name] = true
						}
					}
				}
			}

			if !matched {
				continue
			}

			status := "running"
			if proxyInfo.Status != "running" && proxyInfo.Status != "online" {
				status = proxyInfo.Status
			}

			// Construct remote address from server IP and remote port
			remoteAddr := ""
			if proxyInfo.Conf.RemotePort > 0 {
				remoteAddr = fmt.Sprintf("%s:%d", server.IP, proxyInfo.Conf.RemotePort)
			}

			updates := map[string]interface{}{
				"status":      status,
				"remote_addr": remoteAddr,
			}

			// Calculate traffic delta and persist
			deltaIn := proxyInfo.TodayTrafficIn - proxy.TrafficIn
			deltaOut := proxyInfo.TodayTrafficOut - proxy.TrafficOut

			// Detect counter reset (frps restarted): frps values are much smaller than stored values
			if deltaIn < 0 || deltaOut < 0 {
				// Reset stored values to current frps baseline so next poll works correctly
				updates["traffic_in"] = proxyInfo.TodayTrafficIn
				updates["traffic_out"] = proxyInfo.TodayTrafficOut
				log.Printf("[Poller] Proxy %s traffic counter reset detected (frps: %d/%d, db: %d/%d), updating baseline",
					proxy.Name, proxyInfo.TodayTrafficIn, proxyInfo.TodayTrafficOut, proxy.TrafficIn, proxy.TrafficOut)
				// Still write updates, just with delta=0
				p.db.Model(&proxy).Updates(updates)
				continue
			}

			if deltaIn > 0 || deltaOut > 0 {
				updates["traffic_in"] = proxyInfo.TodayTrafficIn
				updates["traffic_out"] = proxyInfo.TodayTrafficOut

				// Write TrafficLog
				p.db.Create(&model.TrafficLog{
					ProxyID:    proxy.ID,
					UserID:     proxy.UserID,
					ServerID:   server.ID,
					TrafficIn:  deltaIn,
					TrafficOut: deltaOut,
					RecordedAt: time.Now(),
				})

				// Upsert TrafficDaily
				today := time.Now().Format("2006-01-02")
				var daily model.TrafficDaily
				if err := p.db.Where("proxy_id = ? AND user_id = ? AND date = ?",
					proxy.ID, proxy.UserID, today).First(&daily).Error; err == nil {
					p.db.Model(&daily).Updates(map[string]interface{}{
						"traffic_in":  daily.TrafficIn + deltaIn,
						"traffic_out": daily.TrafficOut + deltaOut,
					})
				} else {
					p.db.Create(&model.TrafficDaily{
						ProxyID:    proxy.ID,
						UserID:     proxy.UserID,
						Date:       today,
						TrafficIn:  deltaIn,
						TrafficOut: deltaOut,
					})
				}
			}

			p.db.Model(&proxy).Updates(updates)
		}
	}

	// Mark DB proxies that are "running" but not found in frps as "stopped"
	var dbProxies []model.Proxy
	p.db.Where("server_id = ? AND status = ?", server.ID, "running").Find(&dbProxies)
	for _, proxy := range dbProxies {
		if !foundInFrps[proxy.Name] {
			p.db.Model(&proxy).Updates(map[string]interface{}{
				"status":      "stopped",
				"remote_addr": "",
			})
			log.Printf("[Poller] Proxy %s not found in frps, marked as stopped", proxy.Name)
		}
	}
}

type ProxyInfo struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	User            string `json:"user"`
	TodayTrafficIn  int64  `json:"todayTrafficIn"`
	TodayTrafficOut int64  `json:"todayTrafficOut"`
	Conf            struct {
		RemotePort int `json:"remotePort"`
	} `json:"conf"`
}

func (p *Poller) fetchProxies(baseURL, proxyType, user, password string) ([]ProxyInfo, error) {
	req, err := http.NewRequest("GET", baseURL+"/api/proxy/"+proxyType, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, password)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// The response format is: {"proxies": [...]}
	var result struct {
		Proxies []ProxyInfo `json:"proxies"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// Try direct array format
		var proxies []ProxyInfo
		if err2 := json.Unmarshal(body, &proxies); err2 != nil {
			return nil, err
		}
		return proxies, nil
	}

	return result.Proxies, nil
}

func (p *Poller) fetchHealthz(baseURL string) (bool, error) {
	resp, err := p.client.Get(baseURL + "/healthz")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200, nil
}
