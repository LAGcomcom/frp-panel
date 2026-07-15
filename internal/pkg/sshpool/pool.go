package sshpool

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	Host       string
	Port       int
	User       string
	Password   string
	PrivateKey string
	client     *ssh.Client
	mu         sync.Mutex
}

func NewClient(host string, port int, user, password, privateKey string) *Client {
	if port == 0 {
		port = 22
	}
	return &Client{
		Host:       host,
		Port:       port,
		User:       user,
		Password:   password,
		PrivateKey: privateKey,
	}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		// Test if connection is still alive
		_, _, err := c.client.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			return nil
		}
		c.client.Close()
		c.client = nil
	}

	config := &ssh.ClientConfig{
		User:            c.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	if c.PrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(c.PrivateKey))
		if err != nil {
			return fmt.Errorf("parse private key: %w", err)
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else if c.Password != "" {
		config.Auth = []ssh.AuthMethod{ssh.Password(c.Password)}
	} else {
		return fmt.Errorf("no authentication method provided")
	}

	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}

	c.client = client
	return nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
}

// RunCommand executes a command and returns stdout, stderr, error
func (c *Client) RunCommand(cmd string) (string, string, error) {
	if err := c.Connect(); err != nil {
		return "", "", err
	}

	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return "", "", fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	return stdout.String(), stderr.String(), err
}

// RunCommandStream executes a command and streams output to writer
func (c *Client) RunCommandStream(cmd string, w io.Writer) error {
	if err := c.Connect(); err != nil {
		return err
	}

	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	session.Stdout = w
	session.Stderr = w

	return session.Run(cmd)
}

// UploadFile uploads content to a remote file
func (c *Client) UploadFile(content []byte, remotePath string, perm string) error {
	if perm == "" {
		perm = "0644"
	}

	if err := c.Connect(); err != nil {
		return err
	}

	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	session.Stdin = bytes.NewReader(content)
	err = session.Run(fmt.Sprintf("cat > %s && chmod %s %s", remotePath, perm, remotePath))
	return err
}

// UploadFileFromLocal uploads a local file to remote
func (c *Client) UploadFileFromLocal(localPath, remotePath string) error {
	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}
	return c.UploadFile(content, remotePath, "0644")
}

// DownloadFile downloads a remote file
func (c *Client) DownloadFile(remotePath string) ([]byte, error) {
	if err := c.Connect(); err != nil {
		return nil, err
	}

	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf

	err = session.Run(fmt.Sprintf("cat %s", remotePath))
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// CheckPort checks if a port is listening on the remote host
func (c *Client) CheckPort(port int) (bool, error) {
	cmd := fmt.Sprintf(`if command -v ss >/dev/null 2>&1; then
  ss -tln | grep -E ':%d([[:space:]]|$)'
elif command -v netstat >/dev/null 2>&1; then
  netstat -tln | grep -E ':%d([[:space:]]|$)'
else
  port_hex=$(printf '%%04X' %d)
  grep -i ":$port_hex " /proc/net/tcp /proc/net/tcp6 2>/dev/null
fi`, port, port, port)
	stdout, _, err := c.RunCommand(cmd)
	if err != nil {
		return false, nil
	}
	return strings.Contains(stdout, fmt.Sprintf(":%d", port)), nil
}

// GetSystemInfo returns OS and architecture info
func (c *Client) GetSystemInfo() (osName, arch string, err error) {
	stdout, _, err := c.RunCommand("uname -m && cat /etc/os-release 2>/dev/null | head -5")
	if err != nil {
		return "", "", err
	}

	lines := strings.Split(stdout, "\n")
	if len(lines) > 0 {
		arch = strings.TrimSpace(lines[0])
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "ID=") {
			osName = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
	}

	return osName, arch, nil
}

// TestConnection tests SSH connectivity
func (c *Client) TestConnection() error {
	_, _, err := c.RunCommand("echo ok")
	return err
}

// Pool manages multiple SSH clients
type Pool struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

func NewPool() *Pool {
	return &Pool{
		clients: make(map[string]*Client),
	}
}

func (p *Pool) Get(key string) *Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clients[key]
}

func (p *Pool) Set(key string, client *Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients[key] = client
}

func (p *Pool) Remove(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if client, ok := p.clients[key]; ok {
		client.Close()
		delete(p.clients, key)
	}
}

func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, client := range p.clients {
		client.Close()
	}
	p.clients = make(map[string]*Client)
}

// GetOrCreate gets an existing client or creates a new one
func (p *Pool) GetOrCreate(serverID uint, host string, port int, user, password, privateKey string) (*Client, error) {
	key := fmt.Sprintf("%d", serverID)

	if client := p.Get(key); client != nil {
		return client, nil
	}

	client := NewClient(host, port, user, password, privateKey)
	if err := client.Connect(); err != nil {
		return nil, err
	}

	p.Set(key, client)
	return client, nil
}

// GetClient is a helper to get an SSH client for a server
func GetClient(pool *Pool, serverID uint, host string, port int, user, password, privateKey string) (*Client, error) {
	return pool.GetOrCreate(serverID, host, port, user, password, privateKey)
}
