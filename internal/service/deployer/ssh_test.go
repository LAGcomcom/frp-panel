package deployer

import (
	"strings"
	"testing"

	"github.com/frp-panel/frp-panel/internal/model"
)

func TestBuildDownloadURLs(t *testing.T) {
	got := buildDownloadURLs("https://github.com")
	if len(got) != 1 || got[0] != "https://github.com/fatedier/frp" {
		t.Fatalf("default GitHub URL = %#v", got)
	}

	got = buildDownloadURLs("https://mirror.example.com/")
	if len(got) != 2 || got[0] != "https://mirror.example.com/https://github.com/fatedier/frp" || got[1] != "https://github.com/fatedier/frp" {
		t.Fatalf("mirror fallback URLs = %#v", got)
	}
}

func TestBuildPluginEndpointUsesNodeSpecificSecretPath(t *testing.T) {
	addr, path, err := buildPluginEndpoint("https://panel.example.com/api/plugin/webhook", 42, "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if addr != "https://panel.example.com" || path != "/api/plugin/webhook/42/secret-value" {
		t.Fatalf("plugin endpoint = %q %q", addr, path)
	}
}

func TestBuildPluginEndpointAcceptsPanelHTTPForDirectIPDeployments(t *testing.T) {
	addr, path, err := buildPluginEndpoint("http://38.76.190.234:8080/api/plugin/webhook", 1, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if addr != "http://38.76.190.234:8080" || path != "/api/plugin/webhook/1/secret" {
		t.Fatalf("public HTTP webhook = %q %q", addr, path)
	}

	addr, _, err = buildPluginEndpoint("http://127.0.0.1:8080/api/plugin/webhook", 1, "secret")
	if err != nil || addr != "http://127.0.0.1:8080" {
		t.Fatalf("loopback HTTP webhook = %q err=%v", addr, err)
	}
	addr, _, err = buildPluginEndpoint("panel.example.com/api/plugin/webhook", 1, "secret")
	if err != nil || addr != "https://panel.example.com" {
		t.Fatalf("scheme-less public webhook = %q err=%v", addr, err)
	}
	if !strings.Contains(frpsTomlTemplate, "tlsVerify = true") {
		t.Fatal("FRPS plugin TLS certificate verification is not enabled")
	}
}

func TestPluginEndpointMatchesDetectsChangedPanelAddress(t *testing.T) {
	server := model.Server{ID: 7, PluginSecret: "plugin-secret", PluginAuthEnabled: true}
	deployerA := New(nil, "https://panel-a.example.com/api/plugin/webhook", "", 8080)
	addr, path, err := deployerA.ExpectedPluginEndpoint("", &server)
	if err != nil {
		t.Fatal(err)
	}
	server.PluginWebhookAddr = addr
	server.PluginWebhookPath = path
	if ok, reason := deployerA.PluginEndpointMatches("", &server); !ok {
		t.Fatalf("matching endpoint rejected: %s", reason)
	}

	deployerB := New(nil, "https://panel-b.example.com/api/plugin/webhook", "", 8080)
	if ok, reason := deployerB.PluginEndpointMatches("", &server); ok || !strings.Contains(reason, "面板访问地址已变化") {
		t.Fatalf("changed endpoint not detected: ok=%v reason=%q", ok, reason)
	}
}

func TestRenderInstallScriptIsNonDestructiveUntilVerified(t *testing.T) {
	script, err := renderInstallScript("0.68.0", "https://mirror.example.com")
	if err != nil {
		t.Fatalf("render script: %v", err)
	}
	for _, want := range []string{"command -v curl", "command -v wget", "python3", "apt-get", "dnf", "yum", "apk", "all FRP download sources failed", "tar -tzf", "mv -f /opt/frp/frps.new /opt/frp/frps"} {
		if !strings.Contains(script, want) {
			t.Errorf("script does not contain %q", want)
		}
	}
	if !strings.HasPrefix(script, "#!/bin/sh\nset -eu") {
		t.Fatal("install script is not POSIX sh")
	}
	if !strings.Contains(script, "command -v rc-service") {
		t.Fatal("install script does not accept OpenRC")
	}
	if strings.Contains(script, "pkill -9 frps") || strings.Contains(script, "systemctl stop frps") {
		t.Fatal("script stops the working FRPS before the replacement is verified")
	}
	for _, forbidden := range []string{"set -Eeuo pipefail", "URLS=(", "${URLS[@]}", "local source_url"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("script contains non-POSIX shell syntax %q", forbidden)
		}
	}
}

func TestNormalizeAgentURL(t *testing.T) {
	got, err := normalizeAgentURL("panel.example.com:8080/api/plugin/webhook")
	if err != nil {
		t.Fatalf("normalize agent URL: %v", err)
	}
	if got != "http://panel.example.com:8080" {
		t.Fatalf("normalized URL = %q", got)
	}
	if _, err := normalizeAgentURL("panel.example.com;reboot"); err == nil {
		t.Fatal("unsafe panel address was accepted")
	}
}

func TestOpenRCTemplates(t *testing.T) {
	for name, content := range map[string]string{"frps": frpsOpenRCTemplate} {
		for _, want := range []string{"#!/sbin/openrc-run", "command_background=\"yes\"", "pidfile="} {
			if !strings.Contains(content, want) {
				t.Errorf("%s OpenRC template does not contain %q", name, want)
			}
		}
	}
}

func TestRenderInstallScriptRejectsUnsafeVersion(t *testing.T) {
	if _, err := renderInstallScript("0.68.0; reboot", ""); err == nil {
		t.Fatal("unsafe version was accepted")
	}
}
