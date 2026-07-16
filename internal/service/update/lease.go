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
	c.startOnce.Do(func() {
		if !c.heartbeatEnabled() || (c.centerURL() == "" && len(c.cfg.BootstrapURLs) == 0) || c.cfg.PanelDomain == "" || c.cfg.PanelVersion == "" {
			return
		}
		ctx, cancel := context.WithCancel(parent)
		c.mu.Lock()
		c.cancel = cancel
		c.mu.Unlock()
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.heartbeatLoop(ctx)
		}()
	})
}
func (c *Client) Stop() {
	c.mu.RLock()
	cancel := c.cancel
	c.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	c.wg.Wait()
}

func (c *Client) heartbeatEnabled() bool {
	return c.cfg.HeartbeatEnabled || c.cfg.AnonymousStatistics || c.cfg.Enabled
}

func (c *Client) heartbeatInterval() time.Duration {
	interval := c.cfg.HeartbeatInterval
	if interval == 0 {
		interval = 5 * time.Second
	}
	if interval < 5*time.Second {
		return 5 * time.Second
	}
	return interval
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	registered := false
	retry := time.Second
	nextBootstrapRefresh := time.Time{}
	for {
		if c.cfg.ControlPublicKey != "" && time.Now().After(nextBootstrapRefresh) {
			_ = c.refreshBootstrap(ctx)
			nextBootstrapRefresh = time.Now().Add(time.Hour)
		}
		if !registered {
			priv, pub, err := c.identity()
			if err == nil {
				err = c.registerPublic(ctx, priv, pub)
			}
			if err == nil || strings.Contains(err.Error(), "409") {
				registered = true
			}
		}

		err := errors.New("instance registration failed")
		if registered {
			err = c.sendHeartbeat(ctx)
		}
		wait := c.heartbeatInterval()
		if err != nil {
			wait = retry + randomJitter(retry/4)
			retry = min(retry*2, time.Minute)
		} else {
			retry = time.Second
		}

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

func (c *Client) sendHeartbeat(ctx context.Context) error {
	priv, _, err := c.identity()
	if err != nil {
		return err
	}
	id, err := c.ensureInstanceID()
	if err != nil {
		return err
	}
	domain := normalizedHost(c.cfg.PanelDomain)
	nonce := randomNonce()
	message := strings.Join([]string{id, domain, c.cfg.PanelVersion, runtime.GOOS, runtime.GOARCH, nonce}, "\n")
	return c.postRaw(ctx, "/api/v1/heartbeat", map[string]string{
		"id": id, "domain": domain, "version": c.cfg.PanelVersion,
		"os": runtime.GOOS, "arch": runtime.GOARCH, "nonce": nonce,
		"signature": base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(message))),
	}, nil, nil)
}

func randomJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return time.Duration(n.Int64())
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

func (c *Client) Download(ctx context.Context, version, requestHost string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.centerURL()+"/api/v1/releases/"+url.PathEscape(version)+"/manifest", nil)
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
	id, err := c.ensureInstanceID()
	if err != nil {
		return "", "", err
	}
	nonce := randomNonce()
	domain := c.cfg.PanelDomain
	if domain == "" {
		domain = requestHost
	}
	if err = c.registerPublicDomain(ctx, priv, domain); err != nil && !strings.Contains(err.Error(), "409") {
		return "", "", fmt.Errorf("register update download: %w", err)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(strings.Join([]string{id, version, asset.Name, nonce}, "\n"))))
	downloadURL := c.centerURL() + "/api/v1/public/releases/" + url.PathEscape(version) + "/download/" + url.PathEscape(asset.Name)
	downloadReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", "", err
	}
	downloadReq.Header.Set("X-Instance-ID", id)
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
	if err := c.refreshBootstrap(ctx); err != nil && c.centerURL() == "" {
		return err
	}
	priv, pub, err := c.identity()
	if err != nil {
		return err
	}
	if err = c.registerPrivate(ctx, priv, pub); err != nil && !strings.Contains(err.Error(), "409") {
		return err
	}
	id, err := c.ensureInstanceID()
	if err != nil {
		return err
	}
	nonce := randomNonce()
	message := strings.Join([]string{id, normalizedHost(c.cfg.PanelDomain), c.cfg.PanelVersion, nonce}, "\n")
	sig := ed25519.Sign(priv, []byte(message))
	var env envelope
	if err = c.postRaw(ctx, "/api/v1/lease/renew", map[string]string{"id": id, "domain": c.cfg.PanelDomain, "version": c.cfg.PanelVersion, "nonce": nonce, "signature": base64.StdEncoding.EncodeToString(sig)}, &env, nil); err != nil {
		return err
	}
	var lease Lease
	if err = c.verifyEnvelope(env, &lease); err != nil {
		return err
	}
	if lease.InstanceID != id || normalizedHost(lease.Domain) != normalizedHost(c.cfg.PanelDomain) {
		return errors.New("lease identity mismatch")
	}
	c.mu.Lock()
	c.lease = &lease
	c.mu.Unlock()
	return c.saveLease(env)
}
func (c *Client) registerPublic(ctx context.Context, priv ed25519.PrivateKey, pub ed25519.PublicKey) error {
	return c.register(ctx, priv, pub, c.cfg.PanelDomain, "/api/v1/public/instances/register", nil)
}

func (c *Client) registerPublicDomain(ctx context.Context, priv ed25519.PrivateKey, domain string) error {
	return c.register(ctx, priv, priv.Public().(ed25519.PublicKey), domain, "/api/v1/public/instances/register", nil)
}

func (c *Client) registerPrivate(ctx context.Context, priv ed25519.PrivateKey, pub ed25519.PublicKey) error {
	return c.register(ctx, priv, pub, c.cfg.PanelDomain, "/api/v1/instances/register", map[string]string{"X-Enrollment-Token": c.cfg.InstanceKey})
}

func (c *Client) register(ctx context.Context, priv ed25519.PrivateKey, pub ed25519.PublicKey, panelDomain, path string, headers map[string]string) error {
	id, err := c.ensureInstanceID()
	if err != nil {
		return err
	}
	nonce := randomNonce()
	pubText := base64.StdEncoding.EncodeToString(pub)
	domain := normalizedHost(panelDomain)
	if domain == "" {
		return errors.New("panel domain is required")
	}
	message := strings.Join([]string{id, domain, c.cfg.PanelVersion, pubText, nonce}, "\n")
	payload := map[string]string{"id": id, "domain": domain, "version": c.cfg.PanelVersion, "os": runtime.GOOS, "arch": runtime.GOARCH, "publickey": pubText, "nonce": nonce, "signature": base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(message)))}
	return c.postRaw(ctx, path, payload, nil, headers)
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
		c.setCenterURL(b.UpdateAPI)
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
	c.setCenterURL(v.UpdateAPI)
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
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
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

func (c *Client) ensureInstanceID() (string, error) {
	c.mu.RLock()
	id := c.instanceID
	c.mu.RUnlock()
	if id != "" {
		return id, nil
	}
	_, pub, err := c.identity()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(pub)
	id = "panel-" + hex.EncodeToString(sum[:16])
	c.mu.Lock()
	if c.instanceID == "" {
		c.instanceID = id
	} else {
		id = c.instanceID
	}
	c.mu.Unlock()
	return id, nil
}
func (c *Client) postRaw(ctx context.Context, path string, payload, out any, headers map[string]string) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.centerURL()+path, bytes.NewReader(body))
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
