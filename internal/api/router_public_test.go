package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/config"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/hash"
	"github.com/frp-panel/frp-panel/internal/pkg/jwt"
	"github.com/frp-panel/frp-panel/internal/service/deployer"
	"github.com/frp-panel/frp-panel/internal/service/monitor"
	updateservice "github.com/frp-panel/frp-panel/internal/service/update"
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
