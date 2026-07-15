package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/frp-panel/frp-panel/internal/config"
)

type Client struct {
	cfg        config.UpdateConfig
	instanceID string
	http       *http.Client
	mu         sync.RWMutex
	lease      *Lease
	baseURL    string
	startOnce  sync.Once
	cancel     context.CancelFunc
}

type Release struct {
	Version     string    `json:"version"`
	Title       string    `json:"title"`
	Notes       string    `json:"notes"`
	DownloadURL string    `json:"download_url"`
	SHA256      string    `json:"sha256"`
	Mandatory   bool      `json:"mandatory"`
	PublishedAt time.Time `json:"published_at"`
}

type CheckResult struct {
	Enabled         bool     `json:"enabled"`
	CurrentVersion  string   `json:"current_version"`
	UpdateAvailable bool     `json:"update_available"`
	Release         *Release `json:"release,omitempty"`
}

func NewClient(cfg config.UpdateConfig, instanceID string) *Client {
	return &Client{cfg: cfg, instanceID: instanceID, baseURL: strings.TrimRight(cfg.CenterURL, "/"), http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) Check(ctx context.Context) (*CheckResult, error) {
	result := &CheckResult{Enabled: c.FeatureAvailable("private_updates"), CurrentVersion: c.cfg.PanelVersion}
	if c.baseURL == "" {
		return result, nil
	}
	if c.cfg.PanelVersion == "" || c.cfg.PanelDomain == "" {
		return nil, fmt.Errorf("update is enabled but center_url, instance_key, panel_version or panel_domain is empty")
	}
	if c.cfg.AnonymousStatistics {
		if err := c.post(ctx, "/api/v1/heartbeat", map[string]string{
			"id": c.instanceID, "domain": c.cfg.PanelDomain, "version": c.cfg.PanelVersion,
			"os": runtime.GOOS, "arch": runtime.GOARCH,
		}, nil); err != nil {
			return nil, fmt.Errorf("report update heartbeat: %w", err)
		}
	}
	var response struct {
		UpdateAvailable bool      `json:"update_available"`
		Release         *envelope `json:"release"`
	}
	if err := c.post(ctx, "/api/v1/check", map[string]string{"version": c.cfg.PanelVersion}, &response); err != nil {
		return nil, fmt.Errorf("check update: %w", err)
	}
	result.UpdateAvailable = response.UpdateAvailable
	if response.Release != nil {
		var release Release
		if err := c.verifyEnvelope(*response.Release, &release); err != nil {
			return nil, fmt.Errorf("verify release: %w", err)
		}
		result.Release = &release
	}
	return result, nil
}

func (c *Client) post(ctx context.Context, path string, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Instance-Key", c.cfg.InstanceKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("update center returned HTTP %d", resp.StatusCode)
	}
	if output != nil {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	return nil
}
