package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOfflineInstallerLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer integration test")
	}

	packageDir := prepareOfflinePackage(t, []byte("panel-v1"), true)
	root := t.TempDir()
	installDir := filepath.Join(root, "opt", "frp-panel")
	dataDir := filepath.Join(root, "var", "lib", "frp-panel")
	configDir := filepath.Join(root, "etc", "frp-panel")
	unitPath := filepath.Join(root, "etc", "systemd", "frp-panel.service")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
		t.Fatal(err)
	}
	mockDir := prepareMocks(t)
	env := append(os.Environ(),
		"PATH="+mockDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FRP_PANEL_INIT=systemd",
		"FRP_PANEL_INSTALL_DIR="+installDir,
		"FRP_PANEL_DATA_DIR="+dataDir,
		"FRP_PANEL_CONFIG_DIR="+configDir,
		"FRP_PANEL_SYSTEMD_UNIT="+unitPath,
		"FRP_PANEL_PORT=18080",
		"FRP_PANEL_DOMAIN=panel.example.com",
		"FRP_PANEL_ADMIN_EMAIL=owner@example.com",
		"FRP_PANEL_ADMIN_PASSWORD=Test-password_123",
		"FRP_PANEL_SKIP_HEALTHCHECK=1",
	)

	runInstaller(t, packageDir, env, "--install", "--yes")
	installed, err := os.ReadFile(filepath.Join(installDir, "frp-panel"))
	if err != nil || string(installed) != "panel-v1" {
		t.Fatalf("installed binary=%q err=%v", installed, err)
	}
	configBytes, err := os.ReadFile(filepath.Join(configDir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(configBytes)
	for _, expected := range []string{
		`port: 18080`,
		`email: "owner@example.com"`,
		`password: "Test-password_123"`,
		`center_url: ""`,
		`heartbeat_enabled: false`,
		`panel_domain: "panel.example.com"`,
	} {
		if !strings.Contains(config, expected) {
			t.Fatalf("config missing %q:\n%s", expected, config)
		}
	}
	if strings.Contains(config, "08642.xyz") {
		t.Fatalf("offline config contains update center: %s", config)
	}
	if _, err = os.Stat(unitPath); err != nil {
		t.Fatalf("systemd unit missing: %v", err)
	}

	writeAssetAndChecksum(t, packageDir, []byte("panel-v2"))
	runInstaller(t, packageDir, env, "--update", "--yes")
	installed, err = os.ReadFile(filepath.Join(installDir, "frp-panel"))
	if err != nil || string(installed) != "panel-v2" {
		t.Fatalf("updated binary=%q err=%v", installed, err)
	}
	backups, err := filepath.Glob(filepath.Join(installDir, "frp-panel.backup.*"))
	if err != nil || len(backups) == 0 {
		t.Fatalf("binary backup missing: %v %v", backups, err)
	}
	configureEnv := append(append([]string(nil), env...),
		"FRP_PANEL_PORT=19090",
		"FRP_PANEL_DOMAIN=https://new-panel.example.com",
		"FRP_PANEL_DEFAULT_FRP_VERSION=0.69.0",
		"FRP_PANEL_GITHUB_MIRROR=https://mirror.example.com",
		"FRP_PANEL_POLLER_INTERVAL=45",
		"FRP_PANEL_PLUGIN_WEBHOOK_URL=https://new-panel.example.com/api/frps/auth",
	)
	runInstaller(t, packageDir, configureEnv, "--configure", "--yes")
	configBytes, err = os.ReadFile(filepath.Join(configDir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`port: 19090`,
		`panel_domain: "https://new-panel.example.com"`,
		`default_version: "0.69.0"`,
		`github_mirror: "https://mirror.example.com"`,
		`poller_interval: 45`,
		`plugin_webhook_url: "https://new-panel.example.com/api/frps/auth"`,
	} {
		if !strings.Contains(string(configBytes), expected) {
			t.Fatalf("configured file missing %q:\n%s", expected, configBytes)
		}
	}
	failingConfigureEnv := append(append([]string(nil), env...), "FRP_PANEL_PORT=20000", "MOCK_FAIL_RESTART=1")
	if output, err := runInstallerError(packageDir, failingConfigureEnv, "--configure", "--yes"); err == nil || !strings.Contains(output, "配置修改失败") {
		t.Fatalf("failed configure did not report rollback: err=%v output=%s", err, output)
	}
	configBytes, err = os.ReadFile(filepath.Join(configDir, "config.yaml"))
	if err != nil || !strings.Contains(string(configBytes), "port: 19090") || strings.Contains(string(configBytes), "port: 20000") {
		t.Fatalf("configuration rollback failed: err=%v\n%s", err, configBytes)
	}
	stopFailEnv := append(append([]string(nil), env...), "FRP_PANEL_PORT=21000", "MOCK_FAIL_STOP=1")
	if output, err := runInstallerError(packageDir, stopFailEnv, "--configure", "--yes"); err == nil || !strings.Contains(output, "无法确认旧服务已经停止") {
		t.Fatalf("failed stop did not abort configuration: err=%v output=%s", err, output)
	}
	configBytes, err = os.ReadFile(filepath.Join(configDir, "config.yaml"))
	if err != nil || !strings.Contains(string(configBytes), "port: 19090") || strings.Contains(string(configBytes), "port: 21000") {
		t.Fatalf("stop failure changed configuration: err=%v\n%s", err, configBytes)
	}
	if err = os.WriteFile(filepath.Join(dataDir, "frp-panel.db"), []byte("database-v2"), 0640); err != nil {
		t.Fatal(err)
	}
	originalUnit := []byte("original service definition")
	if err = os.WriteFile(unitPath, originalUnit, 0644); err != nil {
		t.Fatal(err)
	}
	writeAssetAndChecksum(t, packageDir, []byte("panel-v3"))
	failingEnv := append(append([]string(nil), env...),
		"MOCK_FAIL_RESTART=1",
		"MOCK_MUTATE_DB="+filepath.Join(dataDir, "frp-panel.db"),
		"MOCK_MUTATE_MARKER="+filepath.Join(root, "database-mutated"),
	)
	if output, err := runInstallerError(packageDir, failingEnv, "--update", "--yes"); err == nil || !strings.Contains(output, "服务启动失败") {
		t.Fatalf("failed update did not report rollback: err=%v output=%s", err, output)
	}
	installed, err = os.ReadFile(filepath.Join(installDir, "frp-panel"))
	if err != nil || string(installed) != "panel-v2" {
		t.Fatalf("binary rollback=%q err=%v", installed, err)
	}
	database, err := os.ReadFile(filepath.Join(dataDir, "frp-panel.db"))
	if err != nil || string(database) != "database-v2" {
		t.Fatalf("database rollback=%q err=%v", database, err)
	}
	restoredUnit, err := os.ReadFile(unitPath)
	if err != nil || string(restoredUnit) != string(originalUnit) {
		t.Fatalf("service rollback=%q err=%v", restoredUnit, err)
	}

	if output, err := runInstallerError(packageDir, env, "--uninstall"); err == nil || !strings.Contains(output, "--yes") {
		t.Fatalf("non-interactive uninstall was not rejected: err=%v output=%s", err, output)
	}
	if _, err = os.Stat(filepath.Join(installDir, "frp-panel")); err != nil {
		t.Fatalf("rejected uninstall changed binary: %v", err)
	}

	runInstaller(t, packageDir, env, "--uninstall", "--yes")
	if _, err = os.Stat(filepath.Join(installDir, "frp-panel")); !os.IsNotExist(err) {
		t.Fatalf("binary still exists after uninstall: %v", err)
	}
	if _, err = os.Stat(filepath.Join(configDir, "config.yaml")); err != nil {
		t.Fatalf("config was not preserved: %v", err)
	}
	if _, err = os.Stat(dataDir); err != nil {
		t.Fatalf("data directory was not preserved: %v", err)
	}
}

func TestOfflineInstallerCleansFailedFirstInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer integration test")
	}
	packageDir := prepareOfflinePackage(t, []byte("panel"), true)
	root := t.TempDir()
	installDir := filepath.Join(root, "install")
	dataDir := filepath.Join(root, "data")
	configDir := filepath.Join(root, "config")
	unitPath := filepath.Join(root, "systemd", "frp-panel.service")
	env := append(os.Environ(),
		"PATH="+prepareMocks(t)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FRP_PANEL_INIT=systemd",
		"FRP_PANEL_INSTALL_DIR="+installDir,
		"FRP_PANEL_DATA_DIR="+dataDir,
		"FRP_PANEL_CONFIG_DIR="+configDir,
		"FRP_PANEL_SYSTEMD_UNIT="+unitPath,
		"FRP_PANEL_SKIP_HEALTHCHECK=1",
		"MOCK_FAIL_RESTART=1",
		"MOCK_MUTATE_DB="+filepath.Join(dataDir, "frp-panel.db"),
		"MOCK_MUTATE_MARKER="+filepath.Join(root, "database-mutated"),
	)
	if _, err := runInstallerError(packageDir, env, "--install", "--yes"); err == nil {
		t.Fatal("first install unexpectedly succeeded")
	}
	for _, path := range []string{filepath.Join(installDir, "frp-panel"), filepath.Join(configDir, "config.yaml"), filepath.Join(dataDir, "frp-panel.db"), unitPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("failed first install left %s: %v", path, err)
		}
	}
}

func TestOfflineInstallerOpenRCLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer integration test")
	}
	packageDir := prepareOfflinePackage(t, []byte("panel"), true)
	root := t.TempDir()
	installDir := filepath.Join(root, "install")
	configDir := filepath.Join(root, "config")
	openrcPath := filepath.Join(root, "init.d", "frp-panel")
	env := append(os.Environ(),
		"PATH="+prepareMocks(t)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FRP_PANEL_INIT=openrc",
		"FRP_PANEL_INSTALL_DIR="+installDir,
		"FRP_PANEL_DATA_DIR="+filepath.Join(root, "data"),
		"FRP_PANEL_CONFIG_DIR="+configDir,
		"FRP_PANEL_OPENRC_SCRIPT="+openrcPath,
		"FRP_PANEL_SKIP_HEALTHCHECK=1",
	)
	runInstaller(t, packageDir, env, "--install", "--yes")
	if _, err := os.Stat(openrcPath); err != nil {
		t.Fatalf("OpenRC service missing: %v", err)
	}
	runInstaller(t, packageDir, env, "--uninstall", "--yes")
	if _, err := os.Stat(openrcPath); !os.IsNotExist(err) {
		t.Fatalf("OpenRC service remained after uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "config.yaml")); err != nil {
		t.Fatalf("OpenRC uninstall removed config: %v", err)
	}
}

func TestOfflineInstallerSelectsArm64Asset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer integration test")
	}
	packageDir := prepareOfflinePackage(t, []byte("amd64-panel"), true)
	armAsset := []byte("arm64-panel")
	armPath := filepath.Join(packageDir, "bin", "frp-panel-offline-linux-arm64")
	if err := os.WriteFile(armPath, armAsset, 0755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(armAsset)
	checksum, err := os.OpenFile(filepath.Join(packageDir, "SHA256SUMS.txt"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = checksum.WriteString(hex.EncodeToString(sum[:]) + "  bin/frp-panel-offline-linux-arm64\n"); err != nil {
		checksum.Close()
		t.Fatal(err)
	}
	if err = checksum.Close(); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	installDir := filepath.Join(root, "install")
	env := append(os.Environ(),
		"PATH="+prepareMocks(t)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_ARCH=aarch64",
		"FRP_PANEL_INIT=systemd",
		"FRP_PANEL_INSTALL_DIR="+installDir,
		"FRP_PANEL_DATA_DIR="+filepath.Join(root, "data"),
		"FRP_PANEL_CONFIG_DIR="+filepath.Join(root, "config"),
		"FRP_PANEL_SYSTEMD_UNIT="+filepath.Join(root, "systemd", "frp-panel.service"),
		"FRP_PANEL_SKIP_HEALTHCHECK=1",
	)
	runInstaller(t, packageDir, env, "--install", "--yes")
	installed, err := os.ReadFile(filepath.Join(installDir, "frp-panel"))
	if err != nil || string(installed) != string(armAsset) {
		t.Fatalf("arm64 asset selection=%q err=%v", installed, err)
	}
}

func TestOfflineInstallerHasNoPanelDownloadCommands(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	script, err := os.ReadFile(filepath.Join(repoRoot, "install-offline.sh"))
	if err != nil {
		t.Fatal(err)
	}
	content := strings.ToLower(string(script))
	for _, forbidden := range []string{"github.com", "08642.xyz", "curl ", "wget "} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("offline installer contains network command %q", forbidden)
		}
	}
}

func TestOfflineInstallerRejectsChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer integration test")
	}
	packageDir := prepareOfflinePackage(t, []byte("panel"), false)
	root := t.TempDir()
	env := append(os.Environ(),
		"PATH="+prepareMocks(t)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FRP_PANEL_INIT=systemd",
		"FRP_PANEL_INSTALL_DIR="+filepath.Join(root, "install"),
		"FRP_PANEL_DATA_DIR="+filepath.Join(root, "data"),
		"FRP_PANEL_CONFIG_DIR="+filepath.Join(root, "config"),
		"FRP_PANEL_SYSTEMD_UNIT="+filepath.Join(root, "frp-panel.service"),
	)
	cmd := exec.Command("sh", filepath.Join(packageDir, "install-offline.sh"), "--install", "--yes")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "SHA-256") {
		t.Fatalf("checksum mismatch was not rejected: err=%v output=%s", err, output)
	}
}

func prepareOfflinePackage(t *testing.T, asset []byte, validChecksum bool) string {
	t.Helper()
	_, currentFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	script, err := os.ReadFile(filepath.Join(repoRoot, "install-offline.sh"))
	if err != nil {
		t.Fatal(err)
	}
	packageDir := t.TempDir()
	if err = os.WriteFile(filepath.Join(packageDir, "install-offline.sh"), script, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(packageDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	writeAssetAndChecksum(t, packageDir, asset)
	if !validChecksum {
		if err = os.WriteFile(filepath.Join(packageDir, "SHA256SUMS.txt"), []byte(strings.Repeat("0", 64)+"  bin/frp-panel-offline-linux-amd64\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return packageDir
}

func writeAssetAndChecksum(t *testing.T, packageDir string, asset []byte) {
	t.Helper()
	assetPath := filepath.Join(packageDir, "bin", "frp-panel-offline-linux-amd64")
	if err := os.WriteFile(assetPath, asset, 0755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(asset)
	line := hex.EncodeToString(sum[:]) + "  bin/frp-panel-offline-linux-amd64\n"
	if err := os.WriteFile(filepath.Join(packageDir, "SHA256SUMS.txt"), []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
}

func prepareMocks(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mocks := map[string]string{
		"uname":      "#!/bin/sh\ncase \"$1\" in -s) echo Linux ;; -m) echo \"${MOCK_ARCH:-x86_64}\" ;; *) echo Linux ;; esac\n",
		"id":         "#!/bin/sh\nif [ \"${1:-}\" = -u ]; then echo 0; fi\nexit 0\n",
		"systemctl":  "#!/bin/sh\nif [ \"${1:-}\" = stop ] && [ \"${MOCK_FAIL_STOP:-0}\" = 1 ]; then exit 1; fi\nif [ \"${1:-}\" = is-active ]; then exit 3; fi\nif [ \"${1:-}\" = restart ] && [ -n \"${MOCK_MUTATE_DB:-}\" ] && [ ! -f \"${MOCK_MUTATE_MARKER:-}\" ]; then echo migrated > \"$MOCK_MUTATE_DB\"; : > \"$MOCK_MUTATE_MARKER\"; fi\nif [ \"${1:-}\" = restart ] && [ \"${MOCK_FAIL_RESTART:-0}\" = 1 ]; then exit 1; fi\nexit 0\n",
		"rc-service": "#!/bin/sh\nif [ \"${2:-}\" = stop ] && [ \"${MOCK_FAIL_STOP:-0}\" = 1 ]; then exit 1; fi\nif [ \"${2:-}\" = status ]; then exit 3; fi\nif [ \"${2:-}\" = restart ] && [ \"${MOCK_FAIL_RESTART:-0}\" = 1 ]; then exit 1; fi\nexit 0\n",
		"rc-update":  "#!/bin/sh\nexit 0\n",
		"chown":      "#!/bin/sh\nexit 0\n",
		"hostname":   "#!/bin/sh\nif [ \"${1:-}\" = -I ]; then echo 192.0.2.10; else echo test-host; fi\n",
	}
	for name, content := range mocks {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runInstaller(t *testing.T, packageDir string, env []string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{filepath.Join(packageDir, "install-offline.sh")}, args...)
	cmd := exec.Command("sh", cmdArgs...)
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("installer failed: %v\n%s", err, output)
	}
}

func runInstallerError(packageDir string, env []string, args ...string) (string, error) {
	cmdArgs := append([]string{filepath.Join(packageDir, "install-offline.sh")}, args...)
	cmd := exec.Command("sh", cmdArgs...)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}
