package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBackfillsPublicUpdateDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("update:\n  center_url: \"\"\n  control_public_key: \"\"\n  panel_version: v1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Update.CenterURL != DefaultUpdateCenterURL || cfg.Update.ControlPublicKey != DefaultUpdateControlPublicKey {
		t.Fatalf("update defaults were not restored: %#v", cfg.Update)
	}
	if cfg.Update.HeartbeatEnabled {
		t.Fatal("backfilling update checks must not enable heartbeat reporting")
	}
}
