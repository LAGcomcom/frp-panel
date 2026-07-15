package config

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	JWT      JWTConfig      `yaml:"jwt"`
	FRP      FRPConfig      `yaml:"frp"`
	Admin    AdminConfig    `yaml:"admin"`
	License  LicenseConfig  `yaml:"license"`
	Update   UpdateConfig   `yaml:"update"`
}

type UpdateConfig struct {
	Enabled             bool     `yaml:"enabled"` // Deprecated: only controls anonymous statistics.
	CenterURL           string   `yaml:"center_url"`
	InstanceKey         string   `yaml:"instance_key"` // One-time enrollment token.
	PanelVersion        string   `yaml:"panel_version"`
	PanelDomain         string   `yaml:"panel_domain"`
	ControlPublicKey    string   `yaml:"control_public_key"`
	IdentityKeyFile     string   `yaml:"identity_key_file"`
	LeaseCacheFile      string   `yaml:"lease_cache_file"`
	BootstrapCacheFile  string   `yaml:"bootstrap_cache_file"`
	BootstrapURLs       []string `yaml:"bootstrap_urls"`
	AnonymousStatistics bool     `yaml:"anonymous_statistics"`
}

type LicenseConfig struct {
	AuthServer string `yaml:"auth_server"` // 授权服务器地址
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"` // debug, release, test
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"` // sqlite, postgres
	DSN    string `yaml:"dsn"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type JWTConfig struct {
	Secret     string        `yaml:"secret"`
	ExpireTime time.Duration `yaml:"expire_time"`
	Issuer     string        `yaml:"issuer"`
}

type FRPConfig struct {
	DefaultVersion   string `yaml:"default_version"`
	GithubMirror     string `yaml:"github_mirror"` // GitHub 下载镜像
	DownloadTimeout  int    `yaml:"download_timeout"`
	PluginWebhookURL string `yaml:"plugin_webhook_url"` // 面板自身的 webhook 地址
	PollerInterval   int    `yaml:"poller_interval"`    // Dashboard API 轮询间隔（秒）
	ServerToken      string `yaml:"server_token"`       // 所有 frps 实例共用的认证 token
}

type AdminConfig struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "debug",
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "frp-panel.db",
		},
		Redis: RedisConfig{
			Addr: "localhost:6379",
			DB:   0,
		},
		JWT: JWTConfig{
			Secret:     "",
			ExpireTime: 24 * time.Hour,
			Issuer:     "frp-panel",
		},
		FRP: FRPConfig{
			DefaultVersion:  "0.68.0",
			DownloadTimeout: 300,
		},
		Admin: AdminConfig{
			Email:    "",
			Password: "",
		},
		License: LicenseConfig{
			AuthServer: "https://ymsq.movewellpro.fun",
		},
	}

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	return cfg
}
