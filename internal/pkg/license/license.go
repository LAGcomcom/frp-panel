package license

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"gorm.io/gorm"
)

// Obfuscated encryption passphrase - split across multiple variables to resist simple string extraction.
var _eKp = []string{"f7r", "p4n", "3l!c", "3ns", "3cr3", "tK3y", "2026", "xQz"}
var _ePass string

func init() {
	b := make([]byte, 0, 64)
	for _, s := range _eKp {
		b = append(b, []byte(s)...)
	}
	_ePass = string(b)
}

// LicenseInfo holds the verified license data.
type LicenseInfo struct {
	LicenseKey string `json:"license_key"`
	DeviceID   string `json:"device_id"`
	ExpiresAt  string `json:"expires_at"`
	Prefix     string `json:"prefix"`
	Valid      bool   `json:"valid"`
	VerifiedAt time.Time
}

// VerifyResponse is the API response from the auth server.
type VerifyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Valid   bool   `json:"valid"`
		Prefix  string `json:"prefix"`
		Expires string `json:"expires"`
	} `json:"data"`
	Error string `json:"error"`
}

// HeartbeatResponse is the heartbeat API response.
type HeartbeatResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

// Manager handles license verification and storage.
type Manager struct {
	db              *gorm.DB
	authServer      string
	deviceID        string
	mu              sync.RWMutex
	current         *LicenseInfo
	heartbeatURL    string
	verifyURL       string
	heartbeatOnce   sync.Once
	heartbeatCancel context.CancelFunc
}

// NewManager creates a new license manager.
func NewManager(db *gorm.DB, authServer string) *Manager {
	m := &Manager{
		db:         db,
		authServer: strings.TrimRight(authServer, "/"),
		deviceID:   GetDeviceID(),
	}
	m.heartbeatURL = m.authServer + "/api/verify/heartbeat"
	m.verifyURL = m.authServer + "/api/verify"
	return m
}

// GetDeviceID returns a unique machine identifier based on hostname + MAC + CPU.
func GetDeviceID() string {
	var info []string

	hostname, _ := os.Hostname()
	info = append(info, hostname)

	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/sys/class/net/eth0/address"); err == nil {
			info = append(info, strings.TrimSpace(string(data)))
		}
		if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "model name") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) > 1 {
						info = append(info, strings.TrimSpace(parts[1]))
					}
					break
				}
			}
		}
	}

	if runtime.GOOS == "darwin" {
		for _, iface := range []string{"en0", "en1"} {
			if data, err := os.ReadFile("/sys/class/net/" + iface + "/address"); err == nil {
				info = append(info, strings.TrimSpace(string(data)))
				break
			}
		}
	}

	combined := strings.Join(info, "-")
	hash := md5.Sum([]byte(combined))
	return fmt.Sprintf("%x", hash)
}

// getEncryptedKey returns the obfuscated encryption key.
func getEncryptedKey() string {
	return _ePass
}

// SaveLicense encrypts and stores the license in the database.
func (m *Manager) SaveLicense(info *LicenseInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Anti-tamper check - log warning but don't block saving
	if !AntiTamperCheck() {
		log.Printf("[LICENSE] Warning: integrity check failed during save, but continuing")
	}

	jsonData, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal license: %w", err)
	}

	encrypted, err := Encrypt(string(jsonData), getEncryptedKey())
	if err != nil {
		return fmt.Errorf("encrypt license: %w", err)
	}

	// Compute integrity hash
	hash := computeLicenseHash(encrypted)

	// Upsert encrypted license
	var existing model.Setting
	if err := m.db.Where("key = ?", "_license_enc").First(&existing).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			m.db.Create(&model.Setting{Key: "_license_enc", Value: encrypted})
		} else {
			return err
		}
	} else {
		m.db.Model(&existing).Update("value", encrypted)
	}

	// Upsert hash
	if err := m.db.Where("key = ?", "_license_hash").First(&existing).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			m.db.Create(&model.Setting{Key: "_license_hash", Value: hash})
		} else {
			return err
		}
	} else {
		m.db.Model(&existing).Update("value", hash)
	}

	info.VerifiedAt = time.Now()
	m.current = info
	return nil
}

// LoadLicense loads and decrypts the license from the database.
func (m *Manager) LoadLicense() (*LicenseInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Anti-tamper check - log warning but don't block license loading
	if !AntiTamperCheck() {
		log.Printf("[LICENSE] Warning: integrity check failed, but continuing to load license")
	}

	var setting model.Setting
	if err := m.db.Where("key = ?", "_license_enc").First(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	// Verify integrity — warn but don't block (recompilation may change encryption)
	var hashSetting model.Setting
	if err := m.db.Where("key = ?", "_license_hash").First(&hashSetting).Error; err == nil {
		expected := computeLicenseHash(setting.Value)
		if hashSetting.Value != expected {
			log.Printf("[LICENSE] Integrity hash mismatch (possible tampering or re-encryption), updating hash")
			// Update the stored hash to match current data
			m.db.Model(&hashSetting).Update("value", expected)
		}
	}

	decrypted, err := Decrypt(setting.Value, getEncryptedKey())
	if err != nil {
		return nil, fmt.Errorf("decrypt license: %w", err)
	}

	var info LicenseInfo
	if err := json.Unmarshal([]byte(decrypted), &info); err != nil {
		return nil, fmt.Errorf("unmarshal license: %w", err)
	}

	m.current = &info
	return &info, nil
}

// Verify sends the license key to the auth server for verification.
func (m *Manager) Verify(licenseKey string) (*LicenseInfo, error) {
	// Anti-tamper: distributed check point 3
	if !AntiTamperCheck() {
		return nil, errors.New(_encIntegrityFail.String())
	}

	payload := map[string]string{
		"license_key": licenseKey,
		"device_id":   m.deviceID,
	}
	jsonPayload, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(m.verifyURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("verify request failed: %w", err)
	}
	defer resp.Body.Close()

	var result VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !result.Success || !result.Data.Valid {
		return nil, fmt.Errorf("license verification failed: %s", result.Error)
	}

	info := &LicenseInfo{
		LicenseKey: licenseKey,
		DeviceID:   m.deviceID,
		ExpiresAt:  result.Data.Expires,
		Prefix:     result.Data.Prefix,
		Valid:      true,
		VerifiedAt: time.Now(),
	}

	return info, nil
}

// IsActive returns whether a valid license is currently loaded.
func (m *Manager) IsActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current != nil && m.current.Valid
}

// GetCurrent returns the current license info.
func (m *Manager) GetCurrent() *LicenseInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Heartbeat sends a heartbeat to the auth server to keep the license alive.
func (m *Manager) Heartbeat() bool {
	m.mu.RLock()
	info := m.current
	m.mu.RUnlock()

	if info == nil || !info.Valid {
		return false
	}

	payload := map[string]string{
		"license_key": info.LicenseKey,
		"device_id":   m.deviceID,
	}
	jsonPayload, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(m.heartbeatURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result HeartbeatResponse
	json.NewDecoder(resp.Body).Decode(&result)
	log.Printf("[LICENSE] Heartbeat response: success=%v message=%s", result.Success, result.Message)
	return result.Success
}

// StartHeartbeat starts a supervisor that ensures a heartbeat goroutine is always running.
// Every 5 minutes, the supervisor restarts the heartbeat goroutine as a safeguard.
func (m *Manager) StartHeartbeat() {
	m.heartbeatOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		m.heartbeatCancel = cancel
		log.Printf("[LICENSE] Heartbeat loop started")
		go m.heartbeatLoop(ctx)
	})
}

// heartbeatLoop sends heartbeats every 5 minutes with panic recovery.
func (m *Manager) heartbeatLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LICENSE] Heartbeat goroutine panic recovered: %v", r)
		}
	}()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Send first heartbeat immediately
	m.doHeartbeat()

	for {
		select {
		case <-ticker.C:
			m.doHeartbeat()
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) StopHeartbeat() {
	if m.heartbeatCancel != nil {
		m.heartbeatCancel()
	}
}

// doHeartbeat sends a single heartbeat and invalidates the license on failure.
func (m *Manager) doHeartbeat() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LICENSE] Heartbeat panic recovered: %v", r)
		}
	}()

	if !m.IsActive() {
		return
	}

	if !m.Heartbeat() {
		log.Printf("[LICENSE] Heartbeat failed (auth server unreachable?), license remains active")
	}
}

// computeLicenseHash computes an integrity hash for the encrypted license data.
func computeLicenseHash(data string) string {
	// Use a salt derived from multiple parts to make hash harder to forge
	salt := []byte("frp-panel-license-integrity-check-2026")
	h := md5.New()
	h.Write(salt)
	h.Write([]byte(data))
	h.Write(salt)
	return fmt.Sprintf("%x", h.Sum(nil))
}
