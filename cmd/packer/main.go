// packer encrypts the panel binary for protected distribution.
// Usage: go run ./cmd/packer/ panel.exe panel.protected.exe
//
// This creates a self-extracting executable that:
// 1. Decrypts the real binary at runtime
// 2. Writes it to a temp file
// 3. Executes it and waits for it to finish
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Encryption key (split into parts, concatenated at runtime)
var _ek = []string{
	"f7r-p4n", "-3l!c", "3ns-3cr", "3tK3y-20",
	"26-xQz-p", "acker-k", "ey-xyz9",
}

func getKey() []byte {
	b := make([]byte, 0, 64)
	for _, s := range _ek {
		b = append(b, []byte(s)...)
	}
	h := sha256.Sum256(b)
	return h[:]
}

// Loader template - {{MARKER}} is replaced at build time with a random marker
const loaderTemplate = `package main

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var _ek = []string{
	"f7r-p4n", "-3l!c", "3ns-3cr", "3tK3y-20",
	"26-xQz-p", "acker-k", "ey-xyz9",
}

func getKey() []byte {
	b := make([]byte, 0, 64)
	for _, s := range _ek {
		b = append(b, []byte(s)...)
	}
	h := sha256.Sum256(b)
	return h[:]
}

func main() {
	// Read our own executable
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	data, err := os.ReadFile(exe)
	if err != nil {
		fmt.Println("Error reading executable")
		os.Exit(1)
	}

	// Find the LAST occurrence of the marker (the real one, not any in code)
	marker := []byte("{{MARKER}}")
	idx := bytes.LastIndex(data, marker)
	if idx == -1 {
		fmt.Println("Invalid protected binary")
		os.Exit(1)
	}
	encrypted := data[idx+len(marker):]

	// Decrypt
	key := getKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Println("Decryption error")
		os.Exit(1)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Println("Decryption error")
		os.Exit(1)
	}
	nonceSize := aesGCM.NonceSize()
	if len(encrypted) < nonceSize {
		fmt.Println("Invalid encrypted data")
		os.Exit(1)
	}
	nonce, ciphertext := encrypted[:nonceSize], encrypted[nonceSize:]
	compressed, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		fmt.Println("Decryption failed - wrong key or corrupted data")
		os.Exit(1)
	}

	// Decompress
	gr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		fmt.Println("Decompression error")
		os.Exit(1)
	}
	binary, err := io.ReadAll(gr)
	gr.Close()
	if err != nil {
		fmt.Println("Decompression error")
		os.Exit(1)
	}

	// Write to temp file
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("panel_%d.bin", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, binary, 0755); err != nil {
		fmt.Println("Failed to write temp file")
		os.Exit(1)
	}
	defer os.Remove(tmpFile)

	// Execute
	cmd := exec.Command(tmpFile, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
`

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input> <output> [target_os/target_arch]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Example: %s panel.exe panel.protected.exe\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Example: %s panel-linux panel.protected linux/amd64\n", os.Args[0])
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputPath := os.Args[2]
	targetGOOS := "" // empty = same as current OS
	targetGOARCH := ""
	if len(os.Args) >= 4 {
		parts := splitTarget(os.Args[3])
		if len(parts) == 2 {
			targetGOOS = parts[0]
			targetGOARCH = parts[1]
		}
	}

	// Read original binary
	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Original size: %d bytes\n", len(data))

	// Compress with gzip
	var compressed bytes.Buffer
	gw, _ := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	gw.Write(data)
	gw.Close()
	fmt.Printf("Compressed size: %d bytes\n", compressed.Len())

	// Encrypt with AES-GCM
	key := getKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cipher error: %v\n", err)
		os.Exit(1)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GCM error: %v\n", err)
		os.Exit(1)
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		fmt.Fprintf(os.Stderr, "Nonce error: %v\n", err)
		os.Exit(1)
	}
	encrypted := aesGCM.Seal(nonce, nonce, compressed.Bytes(), nil)
	fmt.Printf("Encrypted size: %d bytes\n", len(encrypted))

	// Generate random marker (32 bytes hex) - won't appear in the loader code
	markerBytes := make([]byte, 16)
	rand.Read(markerBytes)
	marker := fmt.Sprintf("FRP_PK_%X", markerBytes)
	fmt.Printf("Marker: %s\n", marker)

	// Write loader source with marker injected
	loaderDir := filepath.Join(os.TempDir(), "panel_loader")
	os.MkdirAll(loaderDir, 0755)
	loaderSource := strings.Replace(loaderTemplate, "{{MARKER}}", marker, 1)
	os.WriteFile(filepath.Join(loaderDir, "main.go"), []byte(loaderSource), 0644)

	// Initialize go module
	initCmd := exec.Command("go", "mod", "init", "panel_loader")
	initCmd.Dir = loaderDir
	initCmd.Run()

	// Build loader (support cross-compilation)
	loaderPath := filepath.Join(loaderDir, "loader")
	if targetGOOS == "windows" || (targetGOOS == "" && isWindows()) {
		loaderPath += ".exe"
	}
	buildArgs := []string{"build", "-o", loaderPath, "."}
	buildCmd := exec.Command("go", buildArgs...)
	buildCmd.Dir = loaderDir
	buildCmd.Stderr = os.Stderr
	// Set GOOS/GOARCH for cross-compilation of the loader
	env := os.Environ()
	if targetGOOS != "" {
		env = append(env, "GOOS="+targetGOOS, "GOARCH="+targetGOARCH)
	}
	buildCmd.Env = env
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Loader build error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loader built for %s/%s\n", orDefault(targetGOOS, runtime.GOOS), orDefault(targetGOARCH, runtime.GOARCH))

	// Read compiled loader
	loaderData, err := os.ReadFile(loaderPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading loader: %v\n", err)
		os.Exit(1)
	}

	// Concatenate: loader + marker + encrypted data
	markerBytes2 := []byte(marker)
	output := make([]byte, 0, len(loaderData)+len(markerBytes2)+len(encrypted))
	output = append(output, loaderData...)
	output = append(output, markerBytes2...)
	output = append(output, encrypted...)

	if err := os.WriteFile(outputPath, output, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Protected binary: %s (%d bytes)\n", outputPath, len(output))
	fmt.Println("Done! The binary is now encrypted and protected.")

	// Cleanup
	os.RemoveAll(loaderDir)
}

func isWindows() bool {
	return filepath.Separator == '\\'
}

func splitTarget(target string) []string {
	if idx := strings.Index(target, "/"); idx >= 0 {
		return []string{target[:idx], target[idx+1:]}
	}
	return []string{target}
}

func orDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
