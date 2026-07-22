package api

import (
	"github.com/frp-panel/frp-panel/internal/api/handler"
	"github.com/frp-panel/frp-panel/internal/api/middleware"
	"github.com/frp-panel/frp-panel/internal/edition"
	"github.com/frp-panel/frp-panel/internal/pkg/jwt"
	"github.com/frp-panel/frp-panel/internal/service/deployer"
	"github.com/frp-panel/frp-panel/internal/service/monitor"
	updateservice "github.com/frp-panel/frp-panel/internal/service/update"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, jwtManager *jwt.JWTManager, deployer *deployer.Deployer, alertManager *monitor.AlertManager, serverToken string, updateClient *updateservice.Client) *gin.Engine {
	r := gin.Default()

	// Disable trailing slash redirect to allow middleware to handle SPA routes
	r.RedirectTrailingSlash = false

	// CORS
	r.Use(corsMiddleware())

	// SPA middleware - must be registered before routes
	registerStaticRoutes(r)

	// Health check (always available)
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Handlers
	userHandler := handler.NewUserHandler(db, jwtManager)
	userGroupHandler := handler.NewUserGroupHandler(db, deployer)
	serverHandler := handler.NewServerHandler(db, deployer, serverToken)
	proxyHandler := handler.NewProxyHandler(db, deployer)
	planHandler := handler.NewPlanHandler(db)
	orderHandler := handler.NewOrderHandler(db)
	dashboardHandler := handler.NewDashboardHandler(db)
	pluginHandler := handler.NewPluginHandler(db, serverToken)
	trafficHandler := handler.NewTrafficHandler(db)
	wsHandler := handler.NewWebSocketHandler(db)
	alertHandler := handler.NewAlertHandler(db, alertManager)

	// Wire up WebSocket notifications
	alertManager.SetNotifyFunc(wsHandler.NotifyUser)
	websiteHandler := handler.NewWebsiteHandler(db)
	paymentHandler := handler.NewPaymentHandler(db, orderHandler)
	settingHandler := handler.NewSettingHandler(db)
	couponHandler := handler.NewCouponHandler(db)
	announcementHandler := handler.NewAnnouncementHandler(db)

	// WebSocket
	r.GET("/ws", wsHandler.HandleWebSocket)
	r.GET("/ws/user", middleware.JWTAuth(jwtManager, db), wsHandler.HandleUserWebSocket)

	// Public routes
	api := r.Group("/api")
	{
		api.POST("/user/register", userHandler.Register)
		api.POST("/user/send-code", userHandler.SendVerificationCode)
		api.POST("/user/login", userHandler.Login)
		api.POST("/admin/login", userHandler.AdminLogin)
		api.GET("/plans", planHandler.ListPlans)
		api.GET("/plans/:id", planHandler.GetPlan)

		// Legacy webhook remains for existing nodes; redeployed nodes use the
		// server-specific path authenticated by plugin_secret below.
		api.POST("/plugin/webhook", pluginHandler.HandleWebhook)
		api.POST("/plugin/webhook/", pluginHandler.HandleWebhook)
		api.POST("/plugin/webhook/:server_id/:plugin_secret", pluginHandler.HandleServerWebhook)

		// Agent metrics reporting (authenticated by agent API key in header)
		api.POST("/servers/:id/metrics", serverHandler.ReportMetrics)

		// Machine-readable client configs authenticated by the user's API key.
		api.GET("/client/configs", proxyHandler.GetClientConfigsByAPIKey)

		// Public settings
		api.GET("/settings/public", settingHandler.GetPublicSettings)

		// Public announcements
		api.GET("/announcements", announcementHandler.GetActiveAnnouncements)

		// Payment gateway callbacks (no JWT, authenticated by gateway signature)
		api.POST("/pay/notify/:type", paymentHandler.PayNotify)
		api.GET("/pay/notify/:type", paymentHandler.PayNotify)

		// Coupon verify (authenticated users)
		api.POST("/coupons/verify", middleware.JWTAuth(jwtManager, db), couponHandler.VerifyCoupon)
	}

	// Authenticated user routes
	user := api.Group("")
	user.Use(middleware.JWTAuth(jwtManager, db))
	{
		user.GET("/user/profile", userHandler.GetProfile)
		user.PUT("/user/profile", userHandler.UpdateProfile)
		user.POST("/user/change-password", userHandler.ChangePassword)
		user.GET("/user/invite-stats", userHandler.GetInviteStats)
		user.GET("/user/api-key", userHandler.GetAPIKey)
		user.POST("/user/api-key/regenerate", userHandler.RegenerateAPIKey)

		// Proxies
		user.POST("/proxies", proxyHandler.CreateProxy)
		user.GET("/proxies", proxyHandler.ListProxies)
		user.GET("/proxies/:id", proxyHandler.GetProxy)
		user.PUT("/proxies/:id", proxyHandler.UpdateProxy)
		user.DELETE("/proxies/:id", proxyHandler.DeleteProxy)
		user.POST("/proxies/:id/enable", proxyHandler.EnableProxy)
		user.POST("/proxies/:id/disable", proxyHandler.DisableProxy)
		user.GET("/proxies/config/:server_id", proxyHandler.GetFrpcConfig)
		user.GET("/proxies/ports/:server_id", proxyHandler.GetServerPorts)

		// Orders
		user.POST("/orders", orderHandler.CreateOrder)
		user.GET("/orders", orderHandler.ListOrders)
		user.GET("/orders/:id", orderHandler.GetOrder)
		user.POST("/orders/recharge", orderHandler.CreateRechargeOrder)

		// Payment methods (user-facing)
		user.GET("/payment-methods", paymentHandler.GetEnabledPaymentMethods)

		// Traffic
		user.GET("/traffic/stats", trafficHandler.GetTrafficStats)
		user.GET("/traffic/logs", trafficHandler.GetTrafficLogs)

		// Alerts
		user.GET("/alerts", alertHandler.GetAlerts)
		user.POST("/alerts/:id/read", alertHandler.MarkAlertRead)

		// Servers (available to users)
		user.GET("/servers/available", serverHandler.ListAvailableServers)

		// User coupons
		user.POST("/coupons/create", couponHandler.CreateUserCoupon)
		user.GET("/coupons/mine", couponHandler.ListMyCoupons)
		user.GET("/coupons/available", couponHandler.ListMyAvailableCoupons)
	}

	// Admin routes
	admin := api.Group("/admin")
	admin.Use(middleware.JWTAuth(jwtManager, db), middleware.AdminRequired())
	{
		// Dashboard
		admin.GET("/dashboard", dashboardHandler.GetDashboard)

		// Users
		admin.GET("/users", userHandler.ListUsers)
		admin.GET("/users/:id", userHandler.GetUser)
		admin.PUT("/users/:id", userHandler.AdminUpdateUser)
		admin.POST("/users/:id/plan", userHandler.AdminAssignPlan)
		admin.DELETE("/users/:id/plan", userHandler.AdminClearPlan)
		admin.POST("/users/:id/ban", userHandler.BanUser)
		admin.POST("/users/:id/unban", userHandler.UnbanUser)

		// User groups and node access
		admin.GET("/user-groups", userGroupHandler.List)
		admin.POST("/user-groups", userGroupHandler.Create)
		admin.PUT("/user-groups/:id", userGroupHandler.Update)
		admin.DELETE("/user-groups/:id", userGroupHandler.Delete)

		// Servers
		admin.POST("/servers", serverHandler.CreateServer)
		admin.GET("/servers", serverHandler.ListServers)
		admin.GET("/servers/:id", serverHandler.GetServer)
		admin.PUT("/servers/:id", serverHandler.UpdateServer)
		admin.DELETE("/servers/:id", serverHandler.DeleteServer)
		admin.POST("/servers/:id/deploy", serverHandler.DeployServer)
		admin.POST("/servers/:id/restart", serverHandler.RestartServer)
		admin.POST("/servers/:id/stop", serverHandler.StopServer)
		admin.GET("/servers/:id/config", serverHandler.GetServerConfig)
		admin.PUT("/servers/:id/config", serverHandler.UpdateServerConfig)
		admin.GET("/servers/:id/logs", serverHandler.GetServerLogs)
		admin.POST("/servers/:id/uninstall", serverHandler.UninstallServer)
		admin.GET("/servers/:id/clients", serverHandler.GetServerClients)
		admin.GET("/servers/:id/proxies", serverHandler.GetServerProxies)
		admin.GET("/servers/:id/metrics", serverHandler.GetServerMetrics)
		admin.POST("/servers/:id/install-agent", serverHandler.InstallAgent)

		// Plans
		admin.POST("/plans", planHandler.CreatePlan)
		admin.GET("/plans", planHandler.AdminListPlans)
		admin.PUT("/plans/:id", planHandler.UpdatePlan)
		admin.PUT("/plans/:id/toggle", planHandler.TogglePlanStatus)
		admin.DELETE("/plans/:id", planHandler.DeletePlan)

		// Orders
		admin.GET("/orders", orderHandler.AdminListOrders)
		admin.POST("/orders/:id/refund", orderHandler.RefundOrder)
		admin.POST("/orders/recharge", orderHandler.RechargeBalance)

		// Proxies (admin view)
		admin.GET("/proxies", proxyHandler.AdminListProxies)
		admin.POST("/proxies/:id/enable", proxyHandler.AdminEnableProxy)
		admin.POST("/proxies/:id/disable", proxyHandler.AdminDisableProxy)
		admin.DELETE("/proxies/:id", proxyHandler.AdminDeleteProxy)

		// Traffic (admin view)
		admin.GET("/traffic/stats", trafficHandler.AdminGetTrafficStats)

		// Alerts (admin view)
		admin.GET("/alerts", alertHandler.AdminGetAlerts)
		admin.POST("/alerts/send", alertHandler.AdminSendNotification)

		// Websites
		admin.GET("/websites", websiteHandler.ListWebsites)
		admin.POST("/websites", websiteHandler.CreateWebsite)
		admin.GET("/websites/:id", websiteHandler.GetWebsite)
		admin.PUT("/websites/:id", websiteHandler.UpdateWebsite)
		admin.DELETE("/websites/:id", websiteHandler.DeleteWebsite)
		admin.POST("/websites/:id/check", websiteHandler.CheckWebsite)

		// Payment Configs
		admin.GET("/payment-configs", paymentHandler.ListPaymentConfigs)
		admin.POST("/payment-configs", paymentHandler.CreatePaymentConfig)
		admin.GET("/payment-configs/:id", paymentHandler.GetPaymentConfig)
		admin.PUT("/payment-configs/:id", paymentHandler.UpdatePaymentConfig)
		admin.DELETE("/payment-configs/:id", paymentHandler.DeletePaymentConfig)
		admin.POST("/payment-configs/:id/toggle", paymentHandler.TogglePaymentConfig)

		// Coupons
		admin.GET("/coupons", couponHandler.ListCoupons)
		admin.POST("/coupons", couponHandler.CreateCoupon)
		admin.PUT("/coupons/:id", couponHandler.UpdateCoupon)
		admin.DELETE("/coupons/:id", couponHandler.DeleteCoupon)
		admin.POST("/coupons/:id/toggle", couponHandler.ToggleCoupon)

		// Settings
		admin.GET("/settings", settingHandler.GetSettings)
		admin.PUT("/settings", settingHandler.UpdateSettings)
		admin.POST("/settings/test-smtp", settingHandler.TestSMTP)
		registerUpdateRoutes(admin, updateClient)

		// Announcements
		admin.GET("/announcements", announcementHandler.ListAnnouncements)
		admin.POST("/announcements", announcementHandler.CreateAnnouncement)
		admin.GET("/announcements/:id", announcementHandler.GetAnnouncement)
		admin.PUT("/announcements/:id", announcementHandler.UpdateAnnouncement)
		admin.DELETE("/announcements/:id", announcementHandler.DeleteAnnouncement)
		admin.POST("/announcements/:id/toggle", announcementHandler.ToggleAnnouncement)
	}

	return r
}

func registerUpdateRoutes(admin *gin.RouterGroup, updateClient *updateservice.Client) {
	if edition.Offline {
		return
	}
	updateHandler := handler.NewUpdateHandler(updateClient)
	admin.GET("/update/check", updateHandler.Check)
	admin.GET("/update/lease", updateHandler.LeaseStatus)
	admin.GET("/update/download/:version", updateHandler.Download)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
