package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	jwtpkg "github.com/frp-panel/frp-panel/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestJWTAuthUsesCurrentRoleAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:jwt-current-user?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Plan{}, &model.UserGroup{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "admin@example.com", Password: "x", InviteCode: "admin", APIKey: "admin-key", Role: "admin", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	manager := jwtpkg.NewJWTManager("test-secret", time.Hour, "test")
	token, err := manager.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.Use(JWTAuth(manager, db), AdminRequired())
	router.GET("/admin", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(recorder, req)
		return recorder
	}
	if recorder := request(); recorder.Code != http.StatusNoContent {
		t.Fatalf("active admin status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.Model(&user).Update("role", "user").Error; err != nil {
		t.Fatal(err)
	}
	if recorder := request(); recorder.Code != http.StatusForbidden {
		t.Fatalf("demoted admin status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.Model(&user).Updates(map[string]interface{}{"role": "admin", "status": "banned"}).Error; err != nil {
		t.Fatal(err)
	}
	if recorder := request(); recorder.Code != http.StatusForbidden {
		t.Fatalf("banned admin status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
