package handler

import (
	"net"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMeasureTCPLatencyNeverReturnsZeroForReachableServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	latency, reachable := measureTCPLatency(listener.Addr().String(), time.Second)
	if !reachable {
		t.Fatal("listener was reported unreachable")
	}
	if latency < 1 {
		t.Fatalf("latency = %d, want at least 1ms", latency)
	}
}

func TestAgentPanelAddressUsesRequestWhenWebhookIsNotConfigured(t *testing.T) {
	request := httptest.NewRequest("POST", "http://38.76.190.234:8080/api/admin/servers/1/install-agent", nil)
	request.Host = "38.76.190.234:8080"

	got, err := agentPanelAddress(request, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://38.76.190.234:8080" {
		t.Fatalf("address=%q", got)
	}
}

func TestAgentPanelAddressUsesForwardedHTTPSOrigin(t *testing.T) {
	request := httptest.NewRequest("POST", "http://127.0.0.1/api/admin/servers/1/install-agent", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "panel.example.com")

	got, err := agentPanelAddress(request, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://panel.example.com" {
		t.Fatalf("address=%q", got)
	}
}

func TestAgentPanelAddressIgnoresForwardedOriginFromRemoteClient(t *testing.T) {
	request := httptest.NewRequest("POST", "http://38.76.190.234:8080/api/admin/servers/1/install-agent", nil)
	request.Host = "38.76.190.234:8080"
	request.RemoteAddr = "203.0.113.8:12345"
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "attacker.example.com")

	got, err := agentPanelAddress(request, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://38.76.190.234:8080" {
		t.Fatalf("address=%q", got)
	}
}

func TestAgentPanelAddressPrefersConfiguredOrigin(t *testing.T) {
	request := httptest.NewRequest("POST", "http://127.0.0.1/api/admin/servers/1/install-agent", nil)

	got, err := agentPanelAddress(request, "https://panel.example.com/api/frps/auth?token=ignored")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://panel.example.com" {
		t.Fatalf("address=%q", got)
	}
}
