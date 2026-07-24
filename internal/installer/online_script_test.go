package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlineInstallerPrefersVerifiedBundledRelease(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "install.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}

	script := string(content)
	fileGuard := `if [ -f "$0" ]; then`
	localCheck := `if [ "$MODE" = "install" ] && [ -z "${FRP_PANEL_VERSION:-}" ] && [ -n "$SCRIPT_DIR" ] && [ -f "$LOCAL_RELEASE_DIR/$ASSET" ] && [ -f "$LOCAL_RELEASE_DIR/checksums.txt" ] && [ -f "$LOCAL_RELEASE_DIR/version.txt" ]`
	downloadFallback := `download "$RELEASE_BASE/$ASSET" "$TMP_DIR/$ASSET"`
	fileGuardIndex := strings.Index(script, fileGuard)
	localIndex := strings.Index(script, localCheck)
	downloadIndex := strings.Index(script, downloadFallback)
	if fileGuardIndex < 0 || fileGuardIndex > localIndex {
		t.Fatal("online installer must only discover bundled files when running from a script file")
	}
	if localIndex < 0 {
		t.Fatal("online installer must only use a complete bundled release for an unpinned initial install")
	}
	if downloadIndex < 0 || localIndex > downloadIndex {
		t.Fatal("online installer must check bundled release files before downloading")
	}
	for _, copyCommand := range []string{
		`cp "$LOCAL_RELEASE_DIR/$ASSET" "$TMP_DIR/$ASSET"`,
		`cp "$LOCAL_RELEASE_DIR/checksums.txt" "$TMP_DIR/checksums.txt"`,
		`cp "$LOCAL_RELEASE_DIR/version.txt" "$TMP_DIR/version.txt"`,
	} {
		if !strings.Contains(script, copyCommand) {
			t.Fatalf("online installer is missing bundled release copy: %s", copyCommand)
		}
	}
}
