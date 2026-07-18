//go:build !offline

package edition

import (
	"testing"

	"github.com/frp-panel/frp-panel/internal/config"
)

func TestOnlineApplyPreservesUpdateConfiguration(t *testing.T) {
	cfg := &config.Config{Update: config.UpdateConfig{CenterURL: "https://updates.example.com", HeartbeatEnabled: true}}
	Apply(cfg)
	if Offline || cfg.Update.CenterURL != "https://updates.example.com" || !cfg.Update.HeartbeatEnabled {
		t.Fatalf("online configuration changed: %#v", cfg.Update)
	}
}
