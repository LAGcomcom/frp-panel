package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSecurePluginRewritesLoginAndEnforcesBandwidth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:plugin-auth?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Server{}, &model.UserGroup{}, &model.UserGroupServer{}, &model.Plan{}, &model.User{}, &model.Proxy{}, &model.Setting{}, &model.TrafficDaily{}); err != nil {
		t.Fatal(err)
	}

	server := model.Server{Name: "node", IP: "127.0.0.1", Token: "node-secret-token", PluginSecret: "plugin-secret", PluginAuthEnabled: true}
	group := model.UserGroup{Name: "allowed"}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&group).Association("Servers").Replace([]model.Server{server}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "user@example.com", Password: "x", APIKey: "user-api-key", Status: "active", GroupID: &group.ID, BandwidthLimit: 2 * 1024 * 1024}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	proxy := model.Proxy{UserID: user.ID, ServerID: server.ID, Name: "1_ssh", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 60022, Enabled: true}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/api/plugin/webhook/:server_id/:plugin_secret", NewPluginHandler(db, "").HandleServerWebhook)
	path := "/api/plugin/webhook/1/plugin-secret"
	timestamp := int64(123456789)
	login := map[string]interface{}{
		"version": "0.1.0", "op": "Login",
		"content": map[string]interface{}{
			"timestamp": timestamp, "privilege_key": makePrivilegeKey(user.APIKey, timestamp),
			"metas": map[string]string{"apikey": user.APIKey},
		},
	}
	responseBody := postPluginRequest(t, router, path, login)
	if responseBody.Reject || responseBody.Unchange {
		t.Fatalf("secure login was not rewritten: %+v", responseBody)
	}
	content := responseBody.Content.(map[string]interface{})
	if got := content["privilege_key"]; got != makePrivilegeKey(server.Token, timestamp) {
		t.Fatalf("rewritten privilege key = %v", got)
	}
	encoded, _ := json.Marshal(responseBody)
	if strings.Contains(string(encoded), server.Token) {
		t.Fatal("plugin response leaked the node token")
	}

	newProxy := map[string]interface{}{
		"version": "0.1.0", "op": "NewProxy",
		"content": map[string]interface{}{
			"user":       map[string]interface{}{"user": "user_1", "metas": map[string]string{"apikey": user.APIKey}},
			"proxy_name": proxy.Name, "proxy_type": proxy.Type, "remote_port": 65000,
			"bandwidth_limit": "999 GB",
			"group":           "untrusted", "group_key": "untrusted",
		},
	}
	proxyResponse := postPluginRequest(t, router, path, newProxy)
	if proxyResponse.Reject || proxyResponse.Unchange {
		t.Fatalf("secure proxy was not enforced: %+v", proxyResponse)
	}
	proxyContent := proxyResponse.Content.(map[string]interface{})
	if got := proxyContent["bandwidth_limit"]; got != "2 MB" {
		t.Fatalf("bandwidth limit = %v, want 2 MB", got)
	}
	if got := proxyContent["bandwidth_limit_mode"]; got != "server" {
		t.Fatalf("bandwidth limit mode = %v, want server", got)
	}
	if got := int(proxyContent["remote_port"].(float64)); got != proxy.RemotePort {
		t.Fatalf("remote port = %d, want stored port %d", got, proxy.RemotePort)
	}
	if _, exists := proxyContent["group"]; exists {
		t.Fatal("client-supplied proxy group was not removed")
	}

	group2 := model.UserGroup{Name: "denied"}
	db.Create(&group2)
	db.Model(&user).Update("group_id", group2.ID)
	denied := postPluginRequest(t, router, path, login)
	if !denied.Reject {
		t.Fatal("user from an unassigned group was accepted")
	}
	ping := map[string]interface{}{
		"version": "0.1.0", "op": "Ping",
		"content": map[string]interface{}{
			"user": map[string]interface{}{"user": "user_1", "metas": map[string]string{"apikey": user.APIKey}},
		},
	}
	if pingResponse := postPluginRequest(t, router, path, ping); !pingResponse.Reject {
		t.Fatal("existing connection kept heartbeat access after its group was removed")
	}
}

func TestLegacyPluginRequiresDirectlyIndexableAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:legacy-plugin?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Plan{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "legacy@example.com", Password: "x", APIKey: "legacy-key", InviteCode: "legacy-invite", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/api/plugin/webhook", NewPluginHandler(db, "").HandleWebhook)
	timestamp := int64(123456789)
	request := map[string]interface{}{
		"version": "0.1.0", "op": "Login",
		"content": map[string]interface{}{
			"timestamp": timestamp, "privilege_key": makePrivilegeKey(user.APIKey, timestamp),
		},
	}
	if response := postPluginRequest(t, router, "/api/plugin/webhook", request); !response.Reject {
		t.Fatal("legacy login without an explicit API key was accepted")
	}
	request["content"].(map[string]interface{})["token"] = user.APIKey
	if response := postPluginRequest(t, router, "/api/plugin/webhook", request); response.Reject {
		t.Fatalf("legacy login with an explicit API key was rejected: %s", response.RejectReason)
	}
}

func postPluginRequest(t *testing.T, router http.Handler, path string, payload interface{}) PluginResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("plugin status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var raw struct {
		Reject       bool                   `json:"reject"`
		RejectReason string                 `json:"reject_reason"`
		Unchange     bool                   `json:"unchange"`
		Content      map[string]interface{} `json:"content"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	return PluginResponse{Reject: raw.Reject, RejectReason: raw.RejectReason, Unchange: raw.Unchange, Content: raw.Content}
}
