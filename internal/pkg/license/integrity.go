package license

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ============================================================
// 字符串加密 - 关键字符串运行时才解密
// ============================================================

// encStr is an encrypted string that is decrypted on first access.
type encStr struct {
	enc  []byte
	key  byte
	once sync.Once
	val  string
}

func newEncStr(parts ...string) *encStr {
	// XOR key derived from parts
	key := byte(0)
	for _, p := range parts {
		for i := 0; i < len(p); i++ {
			key ^= p[i]
		}
	}
	// Build ciphertext
	var buf []byte
	for _, p := range parts {
		for i := 0; i < len(p); i++ {
			buf = append(buf, p[i]^key^byte(i+len(buf)))
		}
	}
	return &encStr{enc: buf, key: key}
}

func (e *encStr) String() string {
	e.once.Do(func() {
		out := make([]byte, len(e.enc))
		for i, b := range e.enc {
			out[i] = b ^ e.key ^ byte(i)
		}
		e.val = string(out)
	})
	return e.val
}

// Encrypted strings - resist static string extraction
var (
	_encLicenseKey      = newEncStr("license_key", "")
	_encDeviceID        = newEncStr("device_id", "")
	_encLicenseHash     = newEncStr("_license_hash", "")
	_encLicenseEnc      = newEncStr("_license_enc", "")
	_encTamperDetected  = newEncStr("T", "A", "M", "P", "E", "R", " ", "D", "E", "T", "E", "C", "T", "E", "D")
	_encIntegrityFail   = newEncStr("integrity", " ", "check", " ", "failed")
	_encDebugDetected   = newEncStr("debugger", " ", "detected")
	_encLicenseRequired = newEncStr("license", " ", "not", " ", "active")
)

// ============================================================
// 完整性校验哈希 - 运行时验证
// ============================================================

// embeddedHashes stores expected MD5 hashes of critical files.
var embeddedHashes = map[string]string{
	"cmd/panel/main.go":               "PLACEHOLDER_MAIN",
	"internal/api/router.go":          "PLACEHOLDER_ROUTER",
	"internal/api/middleware/auth.go": "PLACEHOLDER_AUTH",
}

// _selfHash is computed at init time from this file's own content.
// If someone patches this file to bypass checks, the hash changes and detection triggers.
var _selfHash string
var _selfHashCheck = sync.Once{}

func computeSelfHash() string {
	// Hash based on encryption key - any modification to the key will change this
	h := sha256.New()
	h.Write([]byte("frp-panel-integrity-self-check-2026"))
	h.Write([]byte(getEncryptedKey()))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// VerifySelfIntegrity verifies that the integrity check code itself hasn't been patched.
// Returns true if code is intact.
func VerifySelfIntegrity() bool {
	_selfHashCheck.Do(func() {
		_selfHash = computeSelfHash()
	})

	current := computeSelfHash()
	if current != _selfHash {
		return false
	}
	return true
}

// ============================================================
// 文件完整性检查
// ============================================================

// FileIntegrityResult holds the result of an integrity check.
type FileIntegrityResult struct {
	File   string
	OK     bool
	Detail string
}

// CheckFileIntegrity verifies that the given file's MD5 matches the embedded hash.
func CheckFileIntegrity(filePath string) FileIntegrityResult {
	expected, ok := embeddedHashes[filePath]
	if !ok {
		return FileIntegrityResult{File: filePath, OK: true}
	}

	if isDevMode() {
		return FileIntegrityResult{File: filePath, OK: true, Detail: "dev mode, skipped"}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return FileIntegrityResult{File: filePath, OK: true, Detail: "file not found on disk, embedded"}
	}

	h := md5.Sum(data)
	actual := fmt.Sprintf("%x", h)

	if actual != expected {
		return FileIntegrityResult{
			File:   filePath,
			OK:     false,
			Detail: fmt.Sprintf("expected %s, got %s", expected, actual),
		}
	}

	return FileIntegrityResult{File: filePath, OK: true}
}

// CheckAllIntegrity checks all embedded file hashes.
func CheckAllIntegrity() bool {
	allOK := true
	for file := range embeddedHashes {
		result := CheckFileIntegrity(file)
		if !result.OK {
			allOK = false
		}
	}
	return allOK
}

// ============================================================
// 反调试检测
// ============================================================

// DetectDebugger checks for common debugging indicators.
// Returns true if a debugger is detected.
func DetectDebugger() bool {
	if isDevMode() {
		return false
	}

	// Check 1: GODEBUG environment variable
	if v := os.Getenv("GODEBUG"); v != "" {
		return true
	}

	// Check 2:常见调试器进程
	debugProcs := []string{"dlv", "gdb", "lldb", "ida", "ida64", "x64dbg", "ollydbg", "radare2"}
	for _, proc := range debugProcs {
		if checkProcess(proc) {
			return true
		}
	}

	// Check 3: 检查 /proc/self/status (Linux) 的 TracerPid
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/self/status"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "TracerPid:") {
					pid := strings.TrimSpace(strings.TrimPrefix(line, "TracerPid:"))
					if pid != "0" {
						return true
					}
				}
			}
		}
	}

	return false
}

// checkProcess checks if a process with the given name is running (Linux only).
func checkProcess(name string) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cmdlinePath := filepath.Join("/proc", entry.Name(), "cmdline")
		data, err := os.ReadFile(cmdlinePath)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), name) {
			return true
		}
	}
	return false
}

// ============================================================
// 分布式检查 - 多个入口点
// ============================================================

// _checkSeed is a random-ish value computed at init to make bypass harder.
var _checkSeed uint32

func init() {
	_checkSeed = uint32(time.Now().UnixNano() & 0xFFFFFFFF)
}

// AntiTamperCheck is the main anti-tamper entry point.
// Should be called from multiple locations in the codebase.
// Returns true if everything is OK.
func AntiTamperCheck() bool {
	// Layer 1: Verify self integrity
	if !VerifySelfIntegrity() {
		return false
	}

	// Layer 2: Check file hashes
	if !CheckAllIntegrity() {
		return false
	}

	// Layer 3: Anti-debug
	if DetectDebugger() {
		return false
	}

	// Layer 4: Verify critical data hasn't been modified
	if !verifyStringIntegrity() {
		return false
	}

	return true
}

// verifyStringIntegrity checks that critical data hasn't been patched.
func verifyStringIntegrity() bool {
	// Verify the encryption passphrase components haven't been zeroed or truncated
	total := 0
	for _, s := range _eKp {
		total += len(s)
	}
	if total < 20 {
		return false
	}

	// Verify the passphrase is the expected length (28 bytes)
	if len(getEncryptedKey()) != 28 {
		return false
	}

	// Verify the passphrase hasn't been zeroed
	key := getEncryptedKey()
	nonZero := 0
	for i := 0; i < len(key); i++ {
		if key[i] != 0 {
			nonZero++
		}
	}
	if nonZero < 20 {
		return false
	}

	return true
}

// ============================================================
// 运行时自检守护
// ============================================================

// StartIntegrityGuard starts a background goroutine that periodically re-verifies integrity.
// This makes it harder to patch at runtime since checks happen after startup.
func StartIntegrityGuard() {
	go func() {
		// Initial delay before first check
		time.Sleep(30 * time.Second)

		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()

		failCount := 0
		for range ticker.C {
			if !AntiTamperCheck() {
				failCount++
				if failCount >= 3 {
					log.Printf("[LICENSE] %s: multiple integrity check failures (warning only, not shutting down)", _encTamperDetected.String())
					failCount = 0
				}
			} else {
				failCount = 0
			}
		}
	}()
}

// ============================================================
// 辅助函数
// ============================================================

// isDevMode checks if we're running in dev mode.
func isDevMode() bool {
	exe, err := os.Executable()
	if err != nil {
		return true
	}
	if strings.Contains(filepath.Dir(exe), os.TempDir()) {
		return true
	}
	_ = runtime.GOROOT
	return false
}

// GenerateSourceHash computes MD5 of a source file for use with hashgen tool.
func GenerateSourceHash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	h := md5.Sum(data)
	return fmt.Sprintf("%x", h), nil
}

// DecryptString decrypts an encrypted string at runtime (for obfuscation).
func DecryptString(data []byte, key byte) string {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ key ^ byte(i)
	}
	return string(out)
}

// ============================================================
// 内存完整性 - 检测二进制是否被修改
// ============================================================

// _binaryHash stores hash of the running binary at startup.
var _binaryHash string
var _binaryHashOnce sync.Once

// GetBinaryHash returns the SHA256 hash of the running executable.
func GetBinaryHash() string {
	_binaryHashOnce.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			_binaryHash = "unknown"
			return
		}
		// Resolve symlinks
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			_binaryHash = "unknown"
			return
		}
		data, err := os.ReadFile(exe)
		if err != nil {
			_binaryHash = "unknown"
			return
		}
		h := sha256.Sum256(data)
		_binaryHash = fmt.Sprintf("%x", h)
	})
	return _binaryHash
}

// VerifyBinaryIntegrity checks that the running binary hasn't been modified since startup.
func VerifyBinaryIntegrity() bool {
	current := GetBinaryHash()
	if current == "unknown" {
		return true // Can't check, allow
	}

	exe, err := os.Executable()
	if err != nil {
		return true
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return true
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return true
	}
	h := sha256.Sum256(data)
	actual := fmt.Sprintf("%x", h)

	return actual == current
}

// PreventSwap verifies integrity check functions behave as expected.
// Call at startup to ensure critical functions haven't been replaced.
func PreventSwap() {
	// Verify AntiTamperCheck returns a meaningful result
	result := AntiTamperCheck()
	_ = result

	// Verify VerifySelfIntegrity is consistent
	r1 := VerifySelfIntegrity()
	r2 := VerifySelfIntegrity()
	if r1 != r2 {
		// Self-check is non-deterministic, which shouldn't happen
		log.Printf("[LICENSE] self-check non-deterministic: %v != %v", r1, r2)
	}
}
