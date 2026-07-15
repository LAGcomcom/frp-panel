package update

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Lease struct {
	InstanceID string         `json:"instance_id"`
	Domain     string         `json:"domain"`
	Version    string         `json:"version"`
	Features   []string       `json:"features"`
	Limits     map[string]int `json:"limits"`
	IssuedAt   time.Time      `json:"issued_at"`
	ExpiresAt  time.Time      `json:"expires_at"`
	GraceUntil time.Time      `json:"grace_until"`
}

type envelope struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
	PublicKey string          `json:"public_key"`
}
type bootstrap struct {
	Version      int       `json:"version"`
	AuthAPI      string    `json:"authorization_api"`
	UpdateAPI    string    `json:"update_api"`
	FallbackURLs []string  `json:"fallback_urls"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type manifest struct {
	Version string          `json:"version"`
	Assets  []manifestAsset `json:"assets"`
}
type manifestAsset struct {
	Name        string `json:"name"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"download_url"`
}

func (c *Client) Start(parent context.Context) {
	c.startOnce.Do(func() { ctx, cancel := context.WithCancel(parent); c.cancel = cancel; go c.renewLoop(ctx) })
}
func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *Client) FeatureAvailable(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lease == nil || time.Now().After(c.lease.GraceUntil) {
		return false
	}
	for _, f := range c.lease.Features {
		if f == name {
			return true
		}
	}
	return false
}
func (c *Client) LeaseStatus() *Lease {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lease == nil {
		return nil
	}
	v := *c.lease
	v.Features = append([]string(nil), c.lease.Features...)
	return &v
}

func (c *Client) Download(ctx context.Context, version string) (string, string, error) {
	if !c.FeatureAvailable("private_updates") {
		return "", "", errors.New("private updates require an active lease")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+"/api/v1/releases/"+url.PathEscape(version)+"/manifest", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("manifest HTTP %d", resp.StatusCode)
	}
	var env envelope
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return "", "", err
	}
	var m manifest
	if err = c.verifyEnvelope(env, &m); err != nil {
		return "", "", err
	}
	var asset *manifestAsset
	for i := range m.Assets {
		if m.Assets[i].OS == runtime.GOOS && m.Assets[i].Arch == runtime.GOARCH {
			asset = &m.Assets[i]
			break
		}
	}
	if asset == nil {
		return "", "", errors.New("no update asset for this platform")
	}
	priv, _, err := c.identity()
	if err != nil {
		return "", "", err
	}
	nonce := randomNonce()
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(strings.Join([]string{c.instanceID, version, asset.Name, nonce}, "\n"))))
	downloadReq, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return "", "", err
	}
	downloadReq.Header.Set("X-Instance-ID", c.instanceID)
	downloadReq.Header.Set("X-Nonce", nonce)
	downloadReq.Header.Set("X-Signature", signature)
	downloadResp, err := c.http.Do(downloadReq)
	if err != nil {
		return "", "", err
	}
	defer downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download HTTP %d", downloadResp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "frp-panel-update-*")
	if err != nil {
		return "", "", err
	}
	path := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(downloadResp.Body, asset.Size+1))
	if err != nil {
		return "", "", err
	}
	if asset.Size > 0 && written != asset.Size {
		return "", "", fmt.Errorf("update size mismatch: got %d want %d", written, asset.Size)
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), asset.SHA256) {
		return "", "", errors.New("update SHA-256 mismatch")
	}
	if err = tmp.Close(); err != nil {
		return "", "", err
	}
	ok = true
	return path, asset.Name, nil
}

func (c *Client) renewLoop(ctx context.Context) {
	_ = c.loadCachedLease()
	delay := time.Second
	for {
		if err := c.renew(ctx); err == nil {
			delay = 6 * time.Hour
		} else {
			delay = min(delay*2, time.Hour)
		}
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(max(delay/5, time.Second))))
		wait := delay + time.Duration(j.Int64())
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}
func (c *Client) renew(ctx context.Context) error {
	if err := c.refreshBootstrap(ctx); err != nil && c.baseURL == "" {
		return err
	}
	priv, pub, err := c.identity()
	if err != nil {
		return err
	}
	if err = c.register(ctx, priv, pub); err != nil && !strings.Contains(err.Error(), "409") {
		return err
	}
	nonce := randomNonce()
	message := strings.Join([]string{c.instanceID, normalizedHost(c.cfg.PanelDomain), c.cfg.PanelVersion, nonce}, "\n")
	sig := ed25519.Sign(priv, []byte(message))
	var env envelope
	if err = c.postRaw(ctx, "/api/v1/lease/renew", map[string]string{"id": c.instanceID, "domain": c.cfg.PanelDomain, "version": c.cfg.PanelVersion, "nonce": nonce, "signature": base64.StdEncoding.EncodeToString(sig)}, &env, nil); err != nil {
		return err
	}
	var lease Lease
	if err = c.verifyEnvelope(env, &lease); err != nil {
		return err
	}
	if lease.InstanceID != c.instanceID || normalizedHost(lease.Domain) != normalizedHost(c.cfg.PanelDomain) {
		return errors.New("lease identity mismatch")
	}
	c.mu.Lock()
	c.lease = &lease
	c.mu.Unlock()
	return c.saveLease(env)
}
func (c *Client) register(ctx context.Context, priv ed25519.PrivateKey, pub ed25519.PublicKey) error {
	nonce := randomNonce()
	pubText := base64.StdEncoding.EncodeToString(pub)
	domain := normalizedHost(c.cfg.PanelDomain)
	message := strings.Join([]string{c.instanceID, domain, c.cfg.PanelVersion, pubText, nonce}, "\n")
	payload := map[string]string{"id": c.instanceID, "domain": domain, "version": c.cfg.PanelVersion, "publickey": pubText, "nonce": nonce, "signature": base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(message)))}
	return c.postRaw(ctx, "/api/v1/instances/register", payload, nil, map[string]string{"X-Enrollment-Token": c.cfg.InstanceKey})
}
func (c *Client) refreshBootstrap(ctx context.Context) error {
	urls := append([]string(nil), c.cfg.BootstrapURLs...)
	if c.cfg.CenterURL != "" {
		urls = append(urls, strings.TrimRight(c.cfg.CenterURL, "/")+"/bootstrap.json")
	}
	var last error
	for _, u := range urls {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if e != nil {
			last = e
			continue
		}
		resp, e := c.http.Do(req)
		if e != nil {
			last = e
			continue
		}
		var env envelope
		e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env)
		resp.Body.Close()
		if e != nil {
			last = e
			continue
		}
		var b bootstrap
		if e = c.verifyEnvelope(env, &b); e != nil {
			last = e
			continue
		}
		if time.Now().After(b.ExpiresAt) {
			last = errors.New("bootstrap expired")
			continue
		}
		parsed, e := url.Parse(b.UpdateAPI)
		if e != nil || parsed.Scheme != "https" && parsed.Hostname() != "127.0.0.1" {
			last = errors.New("bootstrap update API must use HTTPS")
			continue
		}
		c.baseURL = strings.TrimRight(b.UpdateAPI, "/")
		_ = c.saveBootstrap(env)
		return nil
	}
	if err := c.loadCachedBootstrap(); err == nil {
		return nil
	}
	return last
}
func (c *Client) bootstrapCachePath() string {
	if c.cfg.BootstrapCacheFile != "" {
		return c.cfg.BootstrapCacheFile
	}
	return "data/update-bootstrap.json"
}
func (c *Client) saveBootstrap(env envelope) error {
	path := c.bootstrapCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, _ := json.Marshal(env)
	return os.WriteFile(path, b, 0600)
}
func (c *Client) loadCachedBootstrap() error {
	b, err := os.ReadFile(c.bootstrapCachePath())
	if err != nil {
		return err
	}
	var env envelope
	if err = json.Unmarshal(b, &env); err != nil {
		return err
	}
	var v bootstrap
	if err = c.verifyEnvelope(env, &v); err != nil {
		return err
	}
	if time.Now().After(v.ExpiresAt) {
		return errors.New("cached bootstrap expired")
	}
	u, err := url.Parse(v.UpdateAPI)
	if err != nil || u.Scheme != "https" && u.Hostname() != "127.0.0.1" {
		return errors.New("cached bootstrap URL invalid")
	}
	c.baseURL = strings.TrimRight(v.UpdateAPI, "/")
	return nil
}
func (c *Client) verifyEnvelope(env envelope, out any) error {
	pinned, err := base64.StdEncoding.DecodeString(c.cfg.ControlPublicKey)
	if err != nil || len(pinned) != ed25519.PublicKeySize {
		return errors.New("invalid pinned control public key")
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pinned), env.Payload, sig) {
		return errors.New("invalid control signature")
	}
	return json.Unmarshal(env.Payload, out)
}
func (c *Client) identity() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	path := c.cfg.IdentityKeyFile
	if path == "" {
		path = "data/update-identity.key"
	}
	if b, err := os.ReadFile(path); err == nil {
		raw, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if e == nil && len(raw) == ed25519.PrivateKeySize {
			priv := ed25519.PrivateKey(raw)
			return priv, priv.Public().(ed25519.PublicKey), nil
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, nil, err
	}
	if err = os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(priv)), 0600); err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}
func (c *Client) postRaw(ctx context.Context, path string, payload, out any, headers map[string]string) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("control center returned HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
	}
	return nil
}
func (c *Client) saveLease(env envelope) error {
	path := c.cfg.LeaseCacheFile
	if path == "" {
		path = "data/update-lease.json"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, _ := json.Marshal(env)
	return os.WriteFile(path, b, 0600)
}
func (c *Client) loadCachedLease() error {
	path := c.cfg.LeaseCacheFile
	if path == "" {
		path = "data/update-lease.json"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var env envelope
	if err = json.Unmarshal(b, &env); err != nil {
		return err
	}
	var lease Lease
	if err = c.verifyEnvelope(env, &lease); err != nil {
		return err
	}
	if time.Now().After(lease.GraceUntil) {
		return errors.New("cached lease grace expired")
	}
	c.mu.Lock()
	c.lease = &lease
	c.mu.Unlock()
	return nil
}
func normalizedHost(raw string) string {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, _ := url.Parse(raw)
	return strings.ToLower(u.Hostname())
}
func randomNonce() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
