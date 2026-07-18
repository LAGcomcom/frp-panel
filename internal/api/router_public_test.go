package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/config"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/hash"
	"github.com/frp-panel/frp-panel/internal/pkg/jwt"
	"github.com/frp-panel/frp-panel/internal/service/deployer"
	"github.com/frp-panel/frp-panel/internal/service/monitor"
	updateservice "github.com/frp-panel/frp-panel/internal/service/update"
	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPublicAdminLoginDoesNotRequireLicense(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "panel.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.User{}, &model.Plan{}, &model.UserGroup{}); err != nil {
		t.Fatal(err)
	}
	password, err := hash.BcryptHash("test-password")
	if err != nil {
		t.Fatal(err)
	}
	admin := model.User{Email: "admin@example.com", Password: password, Role: "super_admin", Status: "active"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}

	jwtManager := jwt.NewJWTManager("test-secret", time.Hour, "test")
	deployerService := deployer.New(db, "", "", 8080)
	alertManager := monitor.NewAlertManager(db)
	updateClient := updateservice.NewClient(config.UpdateConfig{}, "")
	router := SetupRouter(db, jwtManager, deployerService, alertManager, "server-token", updateClient)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewBufferString(`{"email":"admin@example.com","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestClientConfigPreflightAllowsAPIKeyHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(corsMiddleware())
	router.GET("/api/client/configs", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/api/client/configs", nil)
	req.Header.Set("Origin", "https://client.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "X-API-Key")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "x-api-key") {
		t.Fatalf("Access-Control-Allow-Headers=%q", got)
	}
}
