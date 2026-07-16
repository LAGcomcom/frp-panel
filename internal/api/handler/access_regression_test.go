package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/accesscontrol"
	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestExpiredPlanClearsOnlyPlanAssignedGroup(t *testing.T) {
	db := openAccessRegressionDB(t, "plan-expiry")
	group := model.UserGroup{Name: "paid-node-group"}
	plan := model.Plan{Name: "paid", DurationDays: 30, GroupID: &group.ID}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	plan.GroupID = &group.ID
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Hour)

	planUser := model.User{
		Email: "plan-expired@example.com", Password: "x", InviteCode: "plan-expired", APIKey: "plan-expired-key", Status: "active",
		PlanID: &plan.ID, PlanExpiresAt: &expiredAt, GroupID: &group.ID, GroupSource: "plan",
	}
	manualUser := model.User{
		Email: "manual-expired@example.com", Password: "x", InviteCode: "manual-expired", APIKey: "manual-expired-key", Status: "active",
		PlanID: &plan.ID, PlanExpiresAt: &expiredAt, GroupID: &group.ID, GroupSource: "manual",
	}
	if err := db.Create(&planUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&manualUser).Error; err != nil {
		t.Fatal(err)
	}

	if err := accesscontrol.ExpireUserPlan(db, &planUser, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := accesscontrol.ExpireUserPlan(db, &manualUser, time.Now()); err != nil {
		t.Fatal(err)
	}
	var gotPlan, gotManual model.User
	db.First(&gotPlan, planUser.ID)
	db.First(&gotManual, manualUser.ID)
	if gotPlan.GroupID != nil || gotPlan.GroupSource != "expired_plan" {
		t.Fatalf("plan group survived expiry: group_id=%v source=%q", gotPlan.GroupID, gotPlan.GroupSource)
	}
	if gotManual.GroupID == nil || *gotManual.GroupID != group.ID || gotManual.GroupSource != "manual" {
		t.Fatalf("manual group was removed on plan expiry: group_id=%v source=%q", gotManual.GroupID, gotManual.GroupSource)
	}
}

func TestRevokedNodeAccessBlocksExistingProxyEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAccessRegressionDB(t, "proxy-revocation")
	server := model.Server{Name: "revoked-node", IP: "127.0.0.1", Status: "running", PluginAuthEnabled: true}
	group := model.UserGroup{Name: "no-node-access"}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "revoked@example.com", Password: "x", InviteCode: "revoked", APIKey: "revoked-key", Status: "active", GroupID: &group.ID, GroupSource: "manual"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	proxy := model.Proxy{UserID: user.ID, ServerID: server.ID, Name: "1_revoked", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 60022, Enabled: true}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewProxyHandler(db)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", user.ID); c.Next() })
	router.GET("/proxies", handler.ListProxies)
	router.GET("/proxies/:id", handler.GetProxy)
	router.PUT("/proxies/:id", handler.UpdateProxy)
	router.DELETE("/proxies/:id", handler.DeleteProxy)
	router.POST("/proxies/:id/enable", handler.EnableProxy)
	router.POST("/proxies/:id/disable", handler.DisableProxy)

	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/proxies", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRecorder.Code)
	}
	var listResponse struct {
		Data struct {
			List []model.Proxy `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listResponse); err != nil {
		t.Fatal(err)
	}
	if len(listResponse.Data.List) != 0 {
		t.Fatal("revoked proxy remained visible in list")
	}

	id := fmt.Sprint(proxy.ID)
	requests := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodGet, "/proxies/" + id, nil},
		{http.MethodPut, "/proxies/" + id, []byte(`{}`)},
		{http.MethodDelete, "/proxies/" + id, nil},
		{http.MethodPost, "/proxies/" + id + "/enable", nil},
		{http.MethodPost, "/proxies/" + id + "/disable", nil},
	}
	for _, item := range requests {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(item.method, item.path, bytes.NewReader(item.body))
		if item.body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want 403", item.method, item.path, recorder.Code)
		}
	}
	var count int64
	db.Model(&model.Proxy{}).Where("id = ?", proxy.ID).Count(&count)
	if count != 1 {
		t.Fatal("revoked proxy was mutated or deleted")
	}
}

func TestFrpcConfigContainsOnlyEnabledProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAccessRegressionDB(t, "frpc-enabled-only")
	server := model.Server{Name: "config-node", IP: "127.0.0.1", BindPort: 7000, Status: "running", PluginAuthEnabled: true}
	user := model.User{Email: "config@example.com", Password: "x", InviteCode: "config", APIKey: "config-key", Status: "active"}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	for _, proxy := range []model.Proxy{
		{UserID: user.ID, ServerID: server.ID, Name: "enabled-proxy", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 60022, Enabled: true},
		{UserID: user.ID, ServerID: server.ID, Name: "disabled-proxy", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 23, RemotePort: 60023, Enabled: false},
	} {
		if err := db.Create(&proxy).Error; err != nil {
			t.Fatal(err)
		}
	}

	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", user.ID); c.Next() })
	router.GET("/config/:server_id", NewProxyHandler(db).GetFrpcConfig)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/config/"+fmt.Sprint(server.ID), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("config status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			Config string `json:"config"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(payload.Data.Config), []byte("enabled-proxy")) {
		t.Fatal("enabled proxy is missing from generated config")
	}
	if bytes.Contains([]byte(payload.Data.Config), []byte("disabled-proxy")) {
		t.Fatal("disabled proxy was included in generated config")
	}
}

func TestExpiredPlanGroupHidesServersAndProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAccessRegressionDB(t, "expired-list-access")
	server := model.Server{Name: "paid-node", IP: "127.0.0.1", BindPort: 7000, Status: "running", PluginAuthEnabled: true}
	group := model.UserGroup{Name: "paid-group"}
	plan := model.Plan{Name: "paid-plan", DurationDays: 30}
	for _, value := range []interface{}{&server, &group, &plan} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&group).Association("Servers").Replace([]model.Server{server}); err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Minute)
	user := model.User{
		Email: "expired-list@example.com", Password: "x", InviteCode: "expired-list", APIKey: "expired-list-key", Status: "active",
		PlanID: &plan.ID, PlanExpiresAt: &expiredAt, GroupID: &group.ID, GroupSource: "plan",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	proxy := model.Proxy{UserID: user.ID, ServerID: server.ID, Name: "expired-proxy", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 60022, Enabled: true}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", user.ID); c.Next() })
	router.GET("/servers", NewServerHandler(db, nil, "").ListAvailableServers)
	router.GET("/proxies", NewProxyHandler(db).ListProxies)

	serverRecorder := httptest.NewRecorder()
	router.ServeHTTP(serverRecorder, httptest.NewRequest(http.MethodGet, "/servers", nil))
	var serverPayload struct {
		Data []model.Server `json:"data"`
	}
	if serverRecorder.Code != http.StatusOK || json.Unmarshal(serverRecorder.Body.Bytes(), &serverPayload) != nil || len(serverPayload.Data) != 0 {
		t.Fatalf("expired plan server list was not empty: status=%d body=%s", serverRecorder.Code, serverRecorder.Body.String())
	}

	proxyRecorder := httptest.NewRecorder()
	router.ServeHTTP(proxyRecorder, httptest.NewRequest(http.MethodGet, "/proxies", nil))
	var proxyPayload struct {
		Data struct {
			List []model.Proxy `json:"list"`
		} `json:"data"`
	}
	if proxyRecorder.Code != http.StatusOK || json.Unmarshal(proxyRecorder.Body.Bytes(), &proxyPayload) != nil || len(proxyPayload.Data.List) != 0 {
		t.Fatalf("expired plan proxy list was not empty: status=%d body=%s", proxyRecorder.Code, proxyRecorder.Body.String())
	}
}

func openAccessRegressionDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Server{}, &model.UserGroup{}, &model.UserGroupServer{}, &model.Plan{}, &model.User{}, &model.Proxy{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	return db
}
