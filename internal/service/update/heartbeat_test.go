package update

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/config"
)

func TestHeartbeatIntervalDefaultsAndClampsToFiveSeconds(t *testing.T) {
	for _, interval := range []time.Duration{0, time.Second} {
		client := NewClient(config.UpdateConfig{HeartbeatInterval: interval}, "instance")
		if got := client.heartbeatInterval(); got != 5*time.Second {
			t.Fatalf("interval %s resolved to %s", interval, got)
		}
	}
}

func TestStartSendsSignedHeartbeatAndStops(t *testing.T) {
	var mu sync.Mutex
	var publicKey ed25519.PublicKey
	heartbeats := make(chan map[string]string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/api/v1/public/instances/register":
			raw, err := base64.StdEncoding.DecodeString(payload["publickey"])
			if err != nil {
				t.Errorf("decode public key: %v", err)
			}
			mu.Lock()
			publicKey = append(ed25519.PublicKey(nil), raw...)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		case "/api/v1/heartbeat":
			heartbeats <- payload
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(config.UpdateConfig{
		CenterURL:         server.URL,
		InstanceKey:       "enrollment-token",
		PanelVersion:      "1.2.3",
		PanelDomain:       "https://Panel.Example.com/admin",
		HeartbeatEnabled:  true,
		HeartbeatInterval: 5 * time.Second,
		IdentityKeyFile:   filepath.Join(t.TempDir(), "identity.key"),
	}, "instance-1")
	client.Start(context.Background())

	var payload map[string]string
	select {
	case payload = <-heartbeats:
	case <-time.After(2 * time.Second):
		client.Stop()
		t.Fatal("heartbeat was not sent immediately")
	}
	client.Stop()

	mu.Lock()
	pub := append(ed25519.PublicKey(nil), publicKey...)
	mu.Unlock()
	signature, err := base64.StdEncoding.DecodeString(payload["signature"])
	if err != nil {
		t.Fatal(err)
	}
	message := strings.Join([]string{payload["id"], payload["domain"], payload["version"], payload["os"], payload["arch"], payload["nonce"]}, "\n")
	if payload["domain"] != "panel.example.com" || !ed25519.Verify(pub, []byte(message), signature) {
		t.Fatalf("invalid signed heartbeat: %#v", payload)
	}
}

func TestStartDoesNothingWhenHeartbeatDisabled(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(config.UpdateConfig{CenterURL: server.URL, PanelVersion: "1.0.0", PanelDomain: "panel.example.com"}, "instance")
	client.Start(context.Background())
	defer client.Stop()
	select {
	case <-requests:
		t.Fatal("disabled heartbeat made a request")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPublicDownloadRegistrationUsesRequestHostWhenHeartbeatIsDisabled(t *testing.T) {
	registered := make(chan map[string]string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		registered <- payload
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(config.UpdateConfig{
		CenterURL:       server.URL,
		PanelVersion:    "1.0.0",
		IdentityKeyFile: filepath.Join(t.TempDir(), "identity.key"),
	}, "download-instance")
	privateKey, _, err := client.identity()
	if err != nil {
		t.Fatal(err)
	}
	if err = client.registerPublicDomain(context.Background(), privateKey, "panel.example.com:8080"); err != nil {
		t.Fatal(err)
	}
	payload := <-registered
	if payload["domain"] != "panel.example.com" {
		t.Fatalf("registered domain=%q", payload["domain"])
	}
}
