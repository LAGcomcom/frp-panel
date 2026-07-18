package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/frp-panel/frp-panel/internal/api"
	"github.com/frp-panel/frp-panel/internal/config"
	"github.com/frp-panel/frp-panel/internal/edition"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/hash"
	"github.com/frp-panel/frp-panel/internal/pkg/jwt"
	"github.com/frp-panel/frp-panel/internal/service/deployer"
	"github.com/frp-panel/frp-panel/internal/service/monitor"
	updateservice "github.com/frp-panel/frp-panel/internal/service/update"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Load config
	cfg := config.MustLoad(*configPath)
	edition.Apply(cfg)

	// Initialize database
	var db *gorm.DB
	var err error

	switch cfg.Database.Driver {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(cfg.Database.DSN), &gorm.Config{
			Logger: logger.New(log.New(os.Stdout, "", log.LstdFlags), logger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			}),
		})
	default:
		log.Fatalf("unsupported database driver: %s", cfg.Database.Driver)
	}

	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Auto migrate
	if err := db.AutoMigrate(
		&model.Server{},
		&model.UserGroup{},
		&model.UserGroupServer{},
		&model.User{},
		&model.Proxy{},
		&model.Plan{},
		&model.Order{},
		&model.TrafficLog{},
		&model.TrafficDaily{},
		&model.ServerMetricsHistory{},
		&monitor.Alert{},
		&model.Website{},
		&model.PaymentConfig{},
		&model.Setting{},
		&model.Coupon{},
		&model.ReferralRebate{},
		&model.Announcement{},
	); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// Seed default admin user
	seedAdmin(db, cfg)

	// Seed default plan
	seedDefaultPlan(db)

	// Seed default settings
	seedSettings(db)

	// Initialize JWT manager
	jwtManager := jwt.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireTime, cfg.JWT.Issuer)

	// Initialize deployer
	d := deployer.New(db, cfg.FRP.PluginWebhookURL, cfg.FRP.GithubMirror, cfg.Server.Port)

	// Sync agent endpoints - update all agents with current panel address
	go d.SyncAgentEndpoints()

	// Initialize and start poller
	pollerInterval := 30 * time.Second
	if cfg.FRP.PollerInterval > 0 {
		pollerInterval = time.Duration(cfg.FRP.PollerInterval) * time.Second
	}
	p := monitor.NewPoller(db, pollerInterval)
	p.Start()
	log.Printf("Dashboard API poller started with interval %s", pollerInterval)

	// Initialize alert manager
	alertManager := monitor.NewAlertManager(db)
	alertManager.StartPeriodicChecks()
	monitor.StartCouponRefundJob(db)
	monitor.StartOrderExpireJob(db)

	// Setup router
	updateClient := updateservice.NewClient(cfg.Update, cfg.Update.InstanceID)
	if !edition.Offline {
		updateClient.Start(context.Background())
	}
	r := api.SetupRouter(db, jwtManager, d, alertManager, cfg.FRP.ServerToken, updateClient)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down...")
		updateClient.Stop()
		p.Stop()
		os.Exit(0)
	}()

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("frp-panel starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}

func seedAdmin(db *gorm.DB, cfg *config.Config) {
	var count int64
	db.Model(&model.User{}).Where("role = ?", "super_admin").Count(&count)
	if count > 0 {
		return
	}

	hashedPassword, err := hash.BcryptHash(cfg.Admin.Password)
	if err != nil {
		log.Fatalf("failed to hash admin password: %v", err)
	}

	admin := model.User{
		Email:      cfg.Admin.Email,
		Password:   hashedPassword,
		Role:       "super_admin",
		Status:     "active",
		APIKey:     hash.GenerateAPIKey(),
		InviteCode: hash.RandomString(8),
	}

	if err := db.Create(&admin).Error; err != nil {
		log.Fatalf("failed to create admin user: %v", err)
	}

	log.Printf("Admin user created: %s (password set from config)", cfg.Admin.Email)
}

func seedDefaultPlan(db *gorm.DB) {
	var count int64
	db.Model(&model.Plan{}).Count(&count)
	if count > 0 {
		return
	}

	plans := []model.Plan{
		{
			Name:           "Free",
			Description:    "Free plan for basic usage",
			MaxProxies:     3,
			MaxBandwidth:   5 * 1024 * 1024,         // 5MB/s
			MaxTraffic:     10 * 1024 * 1024 * 1024, // 10GB
			MaxPorts:       3,
			DurationDays:   365,
			PriceMonthly:   0,
			PriceQuarterly: 0,
			PriceYearly:    0,
			SortOrder:      0,
			Status:         "active",
		},
		{
			Name:           "Basic",
			Description:    "Basic plan for personal use",
			MaxProxies:     10,
			MaxBandwidth:   20 * 1024 * 1024,         // 20MB/s
			MaxTraffic:     100 * 1024 * 1024 * 1024, // 100GB
			MaxPorts:       10,
			DurationDays:   30,
			PriceMonthly:   9.9,
			PriceQuarterly: 24.9,
			PriceYearly:    79.9,
			SortOrder:      1,
			Status:         "active",
		},
		{
			Name:           "Pro",
			Description:    "Pro plan for power users",
			MaxProxies:     50,
			MaxBandwidth:   100 * 1024 * 1024,         // 100MB/s
			MaxTraffic:     1024 * 1024 * 1024 * 1024, // 1TB
			MaxPorts:       50,
			DurationDays:   30,
			PriceMonthly:   29.9,
			PriceQuarterly: 74.9,
			PriceYearly:    249.9,
			SortOrder:      2,
			Status:         "active",
		},
		{
			Name:           "Enterprise",
			Description:    "Enterprise plan for business use",
			MaxProxies:     200,
			MaxBandwidth:   500 * 1024 * 1024,             // 500MB/s
			MaxTraffic:     5 * 1024 * 1024 * 1024 * 1024, // 5TB
			MaxPorts:       200,
			DurationDays:   30,
			PriceMonthly:   99.9,
			PriceQuarterly: 249.9,
			PriceYearly:    799.9,
			SortOrder:      3,
			Status:         "active",
		},
	}

	for _, plan := range plans {
		db.Create(&plan)
	}

	log.Println("Default plans created")
}

func seedSettings(db *gorm.DB) {
	defaults := map[string]string{
		"registration_enabled":         "true",
		"login_enabled":                "true",
		"site_title":                   "FRP Panel",
		"site_announcement":            "",
		"smtp_host":                    "",
		"smtp_port":                    "587",
		"smtp_user":                    "",
		"smtp_password":                "",
		"smtp_from":                    "",
		"invite_rebate_level1_percent": "10",
		"invite_rebate_level2_percent": "5",
	}

	for key, val := range defaults {
		var existing model.Setting
		if err := db.Where("key = ?", key).First(&existing).Error; err != nil {
			db.Create(&model.Setting{Key: key, Value: val})
		}
	}
	log.Println("Default settings seeded")
}
