package monitor

import (
	"log"
	"strconv"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"gorm.io/gorm"
)

type AlertLevel string

const (
	AlertLevelInfo    AlertLevel = "info"
	AlertLevelWarning AlertLevel = "warning"
	AlertLevelError   AlertLevel = "error"
)

type Alert struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Level     AlertLevel `gorm:"size:20;not null" json:"level"`
	Type      string     `gorm:"size:50;not null" json:"type"` // server_down, traffic_exceeded, plan_expired, admin_message
	Title     string     `gorm:"size:200" json:"title"`
	UserID    *uint      `json:"user_id"`
	ServerID  *uint      `json:"server_id"`
	Message   string     `gorm:"size:500;not null" json:"message"`
	IsRead    bool       `gorm:"default:false" json:"is_read"`
	CreatedAt time.Time  `json:"created_at"`
}

func (Alert) TableName() string {
	return "alerts"
}

type NotifyFunc func(userID uint, alert Alert)

type AlertManager struct {
	db       *gorm.DB
	notifyFn NotifyFunc
}

func NewAlertManager(db *gorm.DB) *AlertManager {
	return &AlertManager{db: db}
}

func (m *AlertManager) SetNotifyFunc(fn NotifyFunc) {
	m.notifyFn = fn
}

// CheckServerHealth checks if any servers are down
func (m *AlertManager) CheckServerHealth() {
	var servers []model.Server
	m.db.Where("status = ?", "running").Find(&servers)

	for _, server := range servers {
		// Check last heartbeat
		if server.LastHeartbeat != nil {
			since := time.Since(*server.LastHeartbeat)
			if since > 5*time.Minute {
				m.createAlert(AlertLevelError, "server_down", nil, &server.ID,
					"Server "+server.Name+" ("+server.IP+") has been unreachable for over 5 minutes")
			}
		}
	}
}

// CheckTrafficLimits checks if users have exceeded their traffic limits
func (m *AlertManager) CheckTrafficLimits() {
	var users []model.User
	m.db.Find(&users)

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	freeMaxTraffic := loadFreeTrafficLimit(m.db)

	for _, user := range users {
		// Calculate monthly traffic
		var monthlyTraffic int64
		m.db.Model(&model.TrafficDaily{}).
			Where("user_id = ? AND date >= ?", user.ID, monthStart.Format("2006-01-02")).
			Select("COALESCE(SUM(traffic_in + traffic_out), 0)").
			Scan(&monthlyTraffic)

		var maxTraffic int64
		if user.PlanID != nil {
			var plan model.Plan
			if err := m.db.First(&plan, user.PlanID).Error; err != nil {
				continue
			}
			maxTraffic = plan.MaxTraffic
		} else {
			maxTraffic = freeMaxTraffic
		}

		if maxTraffic > 0 {
			usagePercent := float64(monthlyTraffic) / float64(maxTraffic) * 100
			if usagePercent >= 100 {
				m.createAlert(AlertLevelError, "traffic_exceeded", &user.ID, nil,
					"Traffic limit exceeded: used "+formatBytes(monthlyTraffic)+" of "+formatBytes(maxTraffic))
				// Auto-disable all user's enabled proxies
				m.db.Model(&model.Proxy{}).Where("user_id = ? AND enabled = ?", user.ID, true).Update("enabled", false)
			} else if usagePercent >= 80 {
				m.createAlert(AlertLevelWarning, "traffic_warning", &user.ID, nil,
					"Traffic usage at "+formatPercent(usagePercent)+": used "+formatBytes(monthlyTraffic)+" of "+formatBytes(maxTraffic))
			}
		}
	}
}

const defaultFreeTraffic = int64(10 * 1024 * 1024 * 1024)

func loadFreeTrafficLimit(db *gorm.DB) int64 {
	var setting model.Setting
	if err := db.Where("key = ?", "free_max_traffic_gb").Limit(1).Find(&setting).Error; err != nil || setting.ID == 0 {
		return defaultFreeTraffic
	}

	gb, err := strconv.ParseFloat(setting.Value, 64)
	if err != nil || gb < 0 {
		return defaultFreeTraffic
	}
	return int64(gb * 1024 * 1024 * 1024)
}

// CheckPlanExpiry checks if user plans are about to expire
func (m *AlertManager) CheckPlanExpiry() {
	var users []model.User
	m.db.Where("plan_id IS NOT NULL AND plan_expires_at IS NOT NULL").Find(&users)

	for _, user := range users {
		if user.PlanExpiresAt == nil {
			continue
		}

		daysUntilExpiry := time.Until(*user.PlanExpiresAt).Hours() / 24

		if daysUntilExpiry < 0 {
			m.createAlert(AlertLevelError, "plan_expired", &user.ID, nil,
				"Plan has expired")
		} else if daysUntilExpiry < 3 {
			m.createAlert(AlertLevelWarning, "plan_expiring", &user.ID, nil,
				"Plan expires in "+formatDays(int(daysUntilExpiry)))
		}
	}
}

func (m *AlertManager) createAlert(level AlertLevel, alertType string, userID, serverID *uint, message string) {
	// Check if similar alert exists in last hour
	var count int64
	query := m.db.Model(&Alert{}).Where("type = ? AND message = ? AND created_at > ?",
		alertType, message, time.Now().Add(-1*time.Hour))
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if serverID != nil {
		query = query.Where("server_id = ?", *serverID)
	}
	query.Count(&count)

	if count > 0 {
		return // Don't create duplicate alerts
	}

	alert := Alert{
		Level:    level,
		Type:     alertType,
		UserID:   userID,
		ServerID: serverID,
		Message:  message,
	}
	m.db.Create(&alert)
	log.Printf("[Alert] %s: %s", level, message)

	// Notify via WebSocket
	if m.notifyFn != nil && userID != nil {
		m.notifyFn(*userID, alert)
	}
}

// GetAlerts returns alerts for a user
func (m *AlertManager) GetAlerts(userID uint, unreadOnly bool) []Alert {
	var alerts []Alert
	query := m.db.Where("user_id = ? OR user_id IS NULL", userID)
	if unreadOnly {
		query = query.Where("is_read = ?", false)
	}
	query.Order("created_at desc").Limit(50).Find(&alerts)
	return alerts
}

// MarkAlertRead marks an alert as read
func (m *AlertManager) MarkAlertRead(alertID, userID uint) {
	m.db.Model(&Alert{}).Where("id = ? AND (user_id = ? OR user_id IS NULL)", alertID, userID).
		Update("is_read", true)
}

// GetAllAlerts returns all alerts (admin)
func (m *AlertManager) GetAllAlerts(page, size int) ([]Alert, int64) {
	var alerts []Alert
	var total int64

	m.db.Model(&Alert{}).Count(&total)
	m.db.Order("created_at desc").Offset((page - 1) * size).Limit(size).Find(&alerts)

	return alerts, total
}

// SendNotification sends a notification from admin to a user (or broadcast)
func (m *AlertManager) SendNotification(userID *uint, title, message string) {
	alert := Alert{
		Level:   AlertLevelInfo,
		Type:    "admin_message",
		Title:   title,
		UserID:  userID,
		Message: message,
	}
	m.db.Create(&alert)
	log.Printf("[Alert] Admin notification sent: %s", title)

	// Notify via WebSocket
	if m.notifyFn != nil && userID != nil {
		m.notifyFn(*userID, alert)
	}
}

// StartPeriodicChecks starts periodic alert checks
func (m *AlertManager) StartPeriodicChecks() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			m.CheckServerHealth()
			m.CheckTrafficLimits()
			m.CheckPlanExpiry()
		}
	}()

	log.Println("[Alert] Periodic checks started")
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return formatFloat(float64(bytes)/float64(TB)) + " TB"
	case bytes >= GB:
		return formatFloat(float64(bytes)/float64(GB)) + " GB"
	case bytes >= MB:
		return formatFloat(float64(bytes)/float64(MB)) + " MB"
	case bytes >= KB:
		return formatFloat(float64(bytes)/float64(KB)) + " KB"
	default:
		return formatFloat(float64(bytes)) + " B"
	}
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}

func formatPercent(f float64) string {
	return strconv.FormatFloat(f, 'f', 1, 64) + "%"
}

func formatDays(days int) string {
	if days == 0 {
		return "today"
	} else if days == 1 {
		return "1 day"
	}
	return strconv.Itoa(days) + " days"
}
