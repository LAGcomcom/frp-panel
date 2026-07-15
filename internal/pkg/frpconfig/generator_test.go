package frpconfig

import (
	"testing"

	"github.com/frp-panel/frp-panel/internal/model"
)

func TestGenerateFrpcConfig(t *testing.T) {
	server := &model.Server{
		IP:       "1.2.3.4",
		BindPort: 7000,
		Token:    "test-token-123",
	}
	user := &model.User{
		ID: 1,
	}
	proxies := []model.Proxy{
		{
			Name:           "my-ssh",
			Type:           "tcp",
			LocalIP:        "127.0.0.1",
			LocalPort:      22,
			RemotePort:     60022,
			UseEncryption:  true,
			UseCompression: true,
		},
		{
			Name:          "my-web",
			Type:          "http",
			LocalIP:       "127.0.0.1",
			LocalPort:     8080,
			Subdomain:     "myapp",
			CustomDomains: "example.com,example2.com",
		},
		{
			Name:      "my-stcp",
			Type:      "stcp",
			LocalIP:   "127.0.0.1",
			LocalPort: 3389,
			SecretKey: "stcp-secret",
		},
	}

	config, err := GenerateFrpcConfig(server, user, proxies)
	if err != nil {
		t.Fatalf("GenerateFrpcConfig failed: %v", err)
	}

	t.Logf("Generated config:\n%s", config)

	if !contains(config, `serverAddr = "1.2.3.4"`) {
		t.Error("missing serverAddr")
	}
	if !contains(config, `serverPort = 7000`) {
		t.Error("missing serverPort")
	}
	if !contains(config, "[auth]") || !contains(config, `token = "test-token-123"`) {
		t.Error("missing auth token")
	}
	if !contains(config, `name = "my-ssh"`) {
		t.Error("missing ssh proxy name")
	}
	if !contains(config, `remotePort = 60022`) {
		t.Error("missing remote port")
	}
	if !contains(config, `transport.useEncryption = true`) {
		t.Error("missing encryption flag")
	}
	if !contains(config, `subdomain = "myapp"`) {
		t.Error("missing subdomain")
	}
	if !contains(config, `secretKey = "stcp-secret"`) {
		t.Error("missing secret key")
	}
}

func TestFormatBandwidth(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, ""},
		{512, "512 B"},
		{1024, "1 KB"},
		{1024 * 1024, "1 MB"},
		{100 * 1024 * 1024, "100 MB"},
	}
	for _, tt := range tests {
		result := FormatBandwidth(tt.input)
		if result != tt.expected {
			t.Errorf("FormatBandwidth(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
