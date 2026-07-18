package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/accesscontrol"
	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAdminCannotManageSuperAdminOrChangeRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-role-guard")
	admin := model.User{Email: "admin@example.com", Password: "x", InviteCode: "admin", APIKey: "admin-key", Role: "admin", Status: "active"}
	superAdmin := model.User{Email: "root@example.com", Password: "x", InviteCode: "root", APIKey: "root-key", Role: "super_admin", Status: "active"}
	regular := model.User{Email: "user@example.com", Password: "x", InviteCode: "user", APIKey: "user-key", Role: "user", Status: "active"}
	for _, user := range []*model.User{&admin, &superAdmin, &regular} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}

	handler := NewUserHandler(db, nil)
	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("role", "admin")
		c.Next()
	})
	adminRouter.PUT("/users/:id", handler.AdminUpdateUser)
	adminRouter.POST("/users/:id/ban", handler.BanUser)

	roleRecorder := performJSONRequest(adminRouter, http.MethodPut, "/users/"+fmt.Sprint(regular.ID), []byte(`{"role":"admin"}`))
	if roleRecorder.Code != http.StatusForbidden {
		t.Fatalf("admin role update status = %d body=%s", roleRecorder.Code, roleRecorder.Body.String())
	}
	banRecorder := performJSONRequest(adminRouter, http.MethodPost, "/users/"+fmt.Sprint(superAdmin.ID)+"/ban", nil)
	if banRecorder.Code != http.StatusForbidden {
		t.Fatalf("admin ban super-admin status = %d body=%s", banRecorder.Code, banRecorder.Body.String())
	}

	superRouter := gin.New()
	superRouter.Use(func(c *gin.Context) {
		c.Set("user_id", superAdmin.ID)
		c.Set("role", "super_admin")
		c.Next()
	})
	superRouter.PUT("/users/:id", handler.AdminUpdateUser)
	promoteRecorder := performJSONRequest(superRouter, http.MethodPut, "/users/"+fmt.Sprint(regular.ID), []byte(`{"role":"admin"}`))
	if promoteRecorder.Code != http.StatusOK {
		t.Fatalf("super-admin role update status = %d body=%s", promoteRecorder.Code, promoteRecorder.Body.String())
	}
	if err := db.First(&regular, regular.ID).Error; err != nil || regular.Role != "admin" {
		t.Fatalf("regular user role = %q err=%v", regular.Role, err)
	}
}

func TestExternalRefundDoesNotCreditBalanceOrChangeOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "refund-safety")
	user := model.User{Email: "refund@example.com", Password: "x", InviteCode: "refund", APIKey: "refund-key", Role: "user", Status: "active", Balance: 10}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	order := model.Order{UserID: user.ID, OrderNo: "external-paid", OrderType: "plan", Amount: 99, PayMethod: "alipay", PayStatus: "paid", PaidAt: &now}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/orders/:id/refund", NewOrderHandler(db).RefundOrder)
	recorder := performJSONRequest(router, http.MethodPost, "/orders/"+fmt.Sprint(order.ID)+"/refund", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("refund status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(&user, user.ID).Error; err != nil || user.Balance != 10 {
		t.Fatalf("balance changed after rejected refund: balance=%v err=%v", user.Balance, err)
	}
	if err := db.First(&order, order.ID).Error; err != nil || order.PayStatus != "paid" {
		t.Fatalf("order changed after rejected refund: status=%q err=%v", order.PayStatus, err)
	}
}

func TestDashboardRevenueUsesPortableTimeRanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "dashboard-revenue")
	user := model.User{Email: "dashboard@example.com", Password: "x", InviteCode: "dashboard", APIKey: "dashboard-key", Role: "user", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	previousMonth := now.AddDate(0, -1, 0)
	orders := []model.Order{
		{UserID: user.ID, OrderNo: "this-month", OrderType: "plan", Amount: 100, PayMethod: "balance", PayStatus: "paid", PaidAt: &now},
		{UserID: user.ID, OrderNo: "previous-month", OrderType: "plan", Amount: 50, PayMethod: "balance", PayStatus: "paid", PaidAt: &previousMonth},
	}
	if err := db.Create(&orders).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/dashboard", NewDashboardHandler(db).GetDashboard)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			Revenue struct {
				Today float64 `json:"today"`
				Month float64 `json:"month"`
				Total float64 `json:"total"`
			} `json:"revenue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Revenue.Today != 100 || payload.Data.Revenue.Month != 100 || payload.Data.Revenue.Total != 150 {
		t.Fatalf("revenue = %+v", payload.Data.Revenue)
	}
}

func TestMatchesStoredProxyNameSupportsSecureAndLegacyNames(t *testing.T) {
	proxy := &model.Proxy{Name: "7_ssh"}
	for _, name := range []string{"7_ssh", "api-key.ssh", "api-key.7_ssh"} {
		if !matchesStoredProxyName(name, proxy, "api-key") {
			t.Fatalf("proxy name %q did not match", name)
		}
	}
	if matchesStoredProxyName("other.ssh", proxy, "api-key") {
		t.Fatal("unrelated proxy name matched")
	}
}

func TestAdminProxyMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-proxy-mutations")
	user := model.User{Email: "proxy-admin@example.com", Password: "x", InviteCode: "proxy-admin", APIKey: "proxy-admin-key", Role: "user", Status: "active"}
	server := model.Server{Name: "node", IP: "127.0.0.1", Status: "running"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	proxy := model.Proxy{UserID: user.ID, ServerID: server.ID, Name: "admin-proxy", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 60022, Enabled: false}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewProxyHandler(db)
	router := gin.New()
	router.POST("/proxies/:id/enable", handler.AdminEnableProxy)
	router.POST("/proxies/:id/disable", handler.AdminDisableProxy)
	router.DELETE("/proxies/:id", handler.AdminDeleteProxy)
	path := "/proxies/" + fmt.Sprint(proxy.ID)
	if recorder := performJSONRequest(router, http.MethodPost, path+"/enable", nil); recorder.Code != http.StatusOK {
		t.Fatalf("enable status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	db.First(&proxy, proxy.ID)
	if !proxy.Enabled {
		t.Fatal("admin enable did not persist")
	}
	if recorder := performJSONRequest(router, http.MethodPost, path+"/disable", nil); recorder.Code != http.StatusOK {
		t.Fatalf("disable status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	db.First(&proxy, proxy.ID)
	if proxy.Enabled {
		t.Fatal("admin disable did not persist")
	}
	if recorder := performJSONRequest(router, http.MethodDelete, path, nil); recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var count int64
	db.Model(&model.Proxy{}).Where("id = ?", proxy.ID).Count(&count)
	if count != 0 {
		t.Fatal("admin delete did not remove proxy")
	}
}

func TestAdminCannotMutateAdministratorOwnedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-proxy-owner-guard")
	owner := model.User{Email: "proxy-root@example.com", Password: "x", InviteCode: "proxy-root", APIKey: "proxy-root-key", Role: "super_admin", Status: "active"}
	server := model.Server{Name: "owner-node", IP: "127.0.0.1", Status: "running"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	proxy := model.Proxy{UserID: owner.ID, ServerID: server.ID, Name: "protected-proxy", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 60023, Enabled: true}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("role", "admin")
		c.Next()
	})
	router.POST("/proxies/:id/disable", NewProxyHandler(db).AdminDisableProxy)
	recorder := performJSONRequest(router, http.MethodPost, "/proxies/"+fmt.Sprint(proxy.ID)+"/disable", nil)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin disable administrator proxy status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(&proxy, proxy.ID).Error; err != nil || !proxy.Enabled {
		t.Fatalf("protected proxy changed: enabled=%v err=%v", proxy.Enabled, err)
	}
}

func TestPaymentConfirmationIsIdempotent(t *testing.T) {
	db := openAdminHardeningDB(t, "payment-idempotency")
	user := model.User{Email: "callback@example.com", Password: "x", InviteCode: "callback", APIKey: "callback-key", Role: "user", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	order := model.Order{UserID: user.ID, OrderNo: "callback-order", OrderType: "recharge", Amount: 25, OriginalAmount: 25, DurationType: "recharge", PayMethod: "alipay", PayStatus: "pending"}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewOrderHandler(db)
	processed, err := handler.confirmOrderPayment(&order, user.ID, "trade-1")
	if err != nil || !processed {
		t.Fatalf("first confirmation: processed=%v err=%v", processed, err)
	}
	processed, err = handler.confirmOrderPayment(&order, user.ID, "trade-2")
	if err != nil || processed {
		t.Fatalf("duplicate confirmation: processed=%v err=%v", processed, err)
	}
	if err := db.First(&user, user.ID).Error; err != nil || user.Balance != 25 {
		t.Fatalf("duplicate callback balance=%v err=%v", user.Balance, err)
	}
	if err := db.First(&order, order.ID).Error; err != nil || order.TradeNo != "trade-1" || order.PayStatus != "paid" {
		t.Fatalf("confirmed order=%+v err=%v", order, err)
	}
}

func TestDifferentPlanPurchaseQueuesUntilCurrentPlanExpires(t *testing.T) {
	db := openAdminHardeningDB(t, "plan-purchase-queue")
	highPlan := model.Plan{Name: "Premium 99", DurationDays: 30, Status: "active"}
	lowPlan := model.Plan{Name: "Basic 9.9", DurationDays: 30, Status: "active"}
	if err := db.Create(&highPlan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&lowPlan).Error; err != nil {
		t.Fatal(err)
	}
	currentExpiry := time.Now().AddDate(0, 0, 30).Truncate(time.Second)
	user := model.User{
		Email: "queued-plan@example.com", Password: "x", InviteCode: "queued-plan", APIKey: "queued-plan-key",
		Role: "user", Status: "active", PlanID: &highPlan.ID, PlanExpiresAt: &currentExpiry,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	order := model.Order{
		UserID: user.ID, PlanID: lowPlan.ID, OrderNo: "queued-low-plan", OrderType: "plan",
		Amount: 9.9, DurationType: "monthly", PayMethod: "alipay", PayStatus: "pending",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewOrderHandler(db)
	processed, err := handler.confirmOrderPayment(&order, user.ID, "queued-trade")
	if err != nil || !processed {
		t.Fatalf("confirm queued purchase: processed=%v err=%v", processed, err)
	}
	if err := db.First(&user, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.PlanID == nil || *user.PlanID != highPlan.ID || user.PlanExpiresAt == nil || !user.PlanExpiresAt.Equal(currentExpiry) {
		t.Fatalf("later purchase overwrote current plan: user=%+v", user)
	}
	var entitlement model.PlanEntitlement
	if err := db.Where("order_id = ?", order.ID).First(&entitlement).Error; err != nil {
		t.Fatal(err)
	}
	if entitlement.Status != model.PlanEntitlementQueued || entitlement.StartsAt != nil || entitlement.ExpiresAt != nil {
		t.Fatalf("different plan was not queued: %+v", entitlement)
	}
	processed, err = handler.confirmOrderPayment(&order, user.ID, "duplicate-trade")
	if err != nil || processed {
		t.Fatalf("duplicate queued confirmation: processed=%v err=%v", processed, err)
	}
	var entitlementCount int64
	db.Model(&model.PlanEntitlement{}).Where("order_id = ?", order.ID).Count(&entitlementCount)
	if entitlementCount != 1 {
		t.Fatalf("duplicate callback created %d entitlements", entitlementCount)
	}

	transitionAt := currentExpiry.Add(time.Minute)
	if err := accesscontrol.ExpireUserPlan(db, &user, transitionAt); err != nil {
		t.Fatal(err)
	}
	if user.PlanID == nil || *user.PlanID != lowPlan.ID || user.PlanExpiresAt == nil {
		t.Fatalf("queued plan did not activate: %+v", user)
	}
	wantExpiry := currentExpiry.AddDate(0, 0, 30)
	if !user.PlanExpiresAt.Equal(wantExpiry) {
		t.Fatalf("queued expiry=%v want=%v", user.PlanExpiresAt, wantExpiry)
	}
	if err := db.First(&entitlement, entitlement.ID).Error; err != nil || entitlement.Status != model.PlanEntitlementActive {
		t.Fatalf("activated entitlement=%+v err=%v", entitlement, err)
	}
}

func TestSamePlanPurchaseExtendsCurrentExpiry(t *testing.T) {
	db := openAdminHardeningDB(t, "same-plan-extension")
	plan := model.Plan{Name: "Premium", DurationDays: 30, Status: "active"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	currentExpiry := time.Now().AddDate(0, 0, 10).Truncate(time.Second)
	user := model.User{
		Email: "extend-plan@example.com", Password: "x", InviteCode: "extend-plan", APIKey: "extend-plan-key",
		Role: "user", Status: "active", PlanID: &plan.ID, PlanExpiresAt: &currentExpiry,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	order := model.Order{
		UserID: user.ID, PlanID: plan.ID, OrderNo: "extend-current-plan", OrderType: "plan",
		Amount: 99, DurationType: "monthly", PayMethod: "alipay", PayStatus: "pending",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	processed, err := NewOrderHandler(db).confirmOrderPayment(&order, user.ID, "extend-trade")
	if err != nil || !processed {
		t.Fatalf("confirm extension: processed=%v err=%v", processed, err)
	}
	if err := db.First(&user, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	wantExpiry := currentExpiry.AddDate(0, 0, 30)
	if user.PlanID == nil || *user.PlanID != plan.ID || user.PlanExpiresAt == nil || !user.PlanExpiresAt.Equal(wantExpiry) {
		t.Fatalf("same plan was not extended: user=%+v want expiry=%v", user, wantExpiry)
	}
	var entitlement model.PlanEntitlement
	if err := db.Where("order_id = ?", order.ID).First(&entitlement).Error; err != nil || entitlement.Status != model.PlanEntitlementExtended {
		t.Fatalf("extension entitlement=%+v err=%v", entitlement, err)
	}
}

func TestConcurrentSamePlanPaymentsBothExtendExpiry(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "concurrent-plan.db") + "?_pragma=busy_timeout%285000%29&_txlock=immediate"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserGroup{}, &model.Plan{}, &model.User{}, &model.Order{}, &model.PlanEntitlement{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(4)
	plan := model.Plan{Name: "Concurrent", DurationDays: 30, Status: "active"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	currentExpiry := time.Now().AddDate(0, 0, 10).Truncate(time.Second)
	user := model.User{
		Email: "concurrent-plan@example.com", Password: "x", InviteCode: "concurrent-plan", APIKey: "concurrent-plan-key",
		Role: "user", Status: "active", PlanID: &plan.ID, PlanExpiresAt: &currentExpiry,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	orders := []model.Order{
		{UserID: user.ID, PlanID: plan.ID, OrderNo: "concurrent-plan-1", OrderType: "plan", Amount: 99, DurationType: "monthly", PayMethod: "alipay", PayStatus: "pending"},
		{UserID: user.ID, PlanID: plan.ID, OrderNo: "concurrent-plan-2", OrderType: "plan", Amount: 99, DurationType: "monthly", PayMethod: "alipay", PayStatus: "pending"},
	}
	if err := db.Create(&orders).Error; err != nil {
		t.Fatal(err)
	}

	type result struct {
		processed bool
		err       error
	}
	results := make(chan result, len(orders))
	var wg sync.WaitGroup
	for index := range orders {
		wg.Add(1)
		go func(order model.Order) {
			defer wg.Done()
			processed, err := NewOrderHandler(db).confirmOrderPayment(&order, user.ID, "trade-"+order.OrderNo)
			results <- result{processed: processed, err: err}
		}(orders[index])
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil || !result.processed {
			t.Fatalf("concurrent confirmation failed: processed=%v err=%v", result.processed, result.err)
		}
	}
	if err := db.First(&user, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	wantExpiry := currentExpiry.AddDate(0, 0, 60)
	if user.PlanExpiresAt == nil || !user.PlanExpiresAt.Equal(wantExpiry) {
		t.Fatalf("concurrent extensions lost time: expiry=%v want=%v", user.PlanExpiresAt, wantExpiry)
	}
	var count int64
	db.Model(&model.PlanEntitlement{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 2 {
		t.Fatalf("concurrent payments created %d entitlements", count)
	}
}

func TestArchivingPlanPreservesPendingPurchaseFulfillment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "archive-pending-plan")
	plan := model.Plan{Name: "Pending purchase", DurationDays: 30, Status: "active"}
	user := model.User{Email: "pending-plan@example.com", Password: "x", InviteCode: "pending-plan", APIKey: "pending-plan-key", Role: "user", Status: "active"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	order := model.Order{
		UserID: user.ID, PlanID: plan.ID, OrderNo: "pending-before-archive", OrderType: "plan",
		Amount: 9.9, DurationType: "monthly", PayMethod: "alipay", PayStatus: "pending",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.DELETE("/plans/:id", NewPlanHandler(db).DeletePlan)
	archived := performJSONRequest(router, http.MethodDelete, "/plans/"+fmt.Sprint(plan.ID), nil)
	if archived.Code != http.StatusOK {
		t.Fatalf("archive status=%d body=%s", archived.Code, archived.Body.String())
	}
	if err := db.First(&plan, plan.ID).Error; err != nil || plan.Status != "archived" {
		t.Fatalf("plan was hard-deleted or not archived: status=%q err=%v", plan.Status, err)
	}
	processed, err := NewOrderHandler(db).confirmOrderPayment(&order, user.ID, "paid-after-archive")
	if err != nil || !processed {
		t.Fatalf("archived pending purchase was not delivered: processed=%v err=%v", processed, err)
	}
	if err := db.First(&user, user.ID).Error; err != nil || user.PlanID == nil || *user.PlanID != plan.ID {
		t.Fatalf("archived plan entitlement missing: user=%+v err=%v", user, err)
	}

	createRouter := gin.New()
	createRouter.Use(func(c *gin.Context) { c.Set("user_id", user.ID); c.Next() })
	createRouter.POST("/orders", NewOrderHandler(db).CreateOrder)
	rejected := performJSONRequest(createRouter, http.MethodPost, "/orders", []byte(fmt.Sprintf(
		`{"plan_id":%d,"duration_type":"monthly","pay_method":"balance"}`, plan.ID)))
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("new archived-plan order status=%d body=%s", rejected.Code, rejected.Body.String())
	}
}

func TestMissingInviterDoesNotBlockPaymentConfirmation(t *testing.T) {
	db := openAdminHardeningDB(t, "payment-missing-inviter")
	inviter := model.User{Email: "deleted-inviter@example.com", Password: "x", InviteCode: "deleted-inviter", APIKey: "deleted-inviter-key", Role: "user", Status: "active"}
	if err := db.Create(&inviter).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "invited@example.com", Password: "x", InviteCode: "invited", APIKey: "invited-key", Role: "user", Status: "active", InvitedBy: &inviter.ID}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&inviter).Error; err != nil {
		t.Fatal(err)
	}
	order := model.Order{UserID: user.ID, OrderNo: "missing-inviter-order", OrderType: "recharge", Amount: 30, OriginalAmount: 30, DurationType: "recharge", PayMethod: "alipay", PayStatus: "pending"}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	processed, err := NewOrderHandler(db).confirmOrderPayment(&order, user.ID, "missing-inviter-trade")
	if err != nil || !processed {
		t.Fatalf("confirmation with missing inviter: processed=%v err=%v", processed, err)
	}
	if err := db.First(&user, user.ID).Error; err != nil || user.Balance != 30 {
		t.Fatalf("missing inviter blocked recharge: balance=%v err=%v", user.Balance, err)
	}
}

func TestBalancePaymentCannotOverdraw(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "balance-no-overdraw")
	user := model.User{Email: "balance@example.com", Password: "x", InviteCode: "balance", APIKey: "balance-key", Role: "user", Status: "active", Balance: 100}
	plan := model.Plan{Name: "balance-plan", DurationDays: 30, Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	orders := []model.Order{
		{UserID: user.ID, PlanID: plan.ID, OrderNo: "balance-1", OrderType: "plan", Amount: 80, DurationType: "monthly", PayMethod: "balance", PayStatus: "pending"},
		{UserID: user.ID, PlanID: plan.ID, OrderNo: "balance-2", OrderType: "plan", Amount: 80, DurationType: "monthly", PayMethod: "balance", PayStatus: "pending"},
	}
	if err := db.Create(&orders).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewOrderHandler(db)
	first := httptest.NewRecorder()
	firstContext, _ := gin.CreateTestContext(first)
	handler.processBalancePayment(firstContext, &orders[0], user.ID)
	second := httptest.NewRecorder()
	secondContext, _ := gin.CreateTestContext(second)
	handler.processBalancePayment(secondContext, &orders[1], user.ID)
	if first.Code != http.StatusOK || second.Code != http.StatusBadRequest {
		t.Fatalf("balance statuses first=%d second=%d", first.Code, second.Code)
	}
	if err := db.First(&user, user.ID).Error; err != nil || user.Balance != 20 {
		t.Fatalf("balance after competing payments=%v err=%v", user.Balance, err)
	}
	var paid int64
	db.Model(&model.Order{}).Where("pay_status = ?", "paid").Count(&paid)
	if paid != 1 {
		t.Fatalf("paid orders=%d want 1", paid)
	}
}

func TestAdminRechargeAuthorizationAndRollback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-recharge-guard")
	superAdmin := model.User{Email: "recharge-root@example.com", Password: "x", InviteCode: "recharge-root", APIKey: "recharge-root-key", Role: "super_admin", Status: "active"}
	regular := model.User{Email: "recharge-user@example.com", Password: "x", InviteCode: "recharge-user", APIKey: "recharge-user-key", Role: "user", Status: "active", Balance: 10}
	if err := db.Create(&superAdmin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&regular).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewOrderHandler(db)
	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) { c.Set("role", "admin"); c.Next() })
	adminRouter.POST("/recharge", handler.RechargeBalance)
	forbidden := performJSONRequest(adminRouter, http.MethodPost, "/recharge", []byte(fmt.Sprintf(`{"user_id":%d,"amount":5}`, superAdmin.ID)))
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("admin recharge super-admin status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}

	if err := db.Callback().Create().Before("gorm:create").Register("test:fail-admin-recharge-order", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Order" {
			tx.AddError(errors.New("forced order create failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	superRouter := gin.New()
	superRouter.Use(func(c *gin.Context) { c.Set("role", "super_admin"); c.Next() })
	superRouter.POST("/recharge", handler.RechargeBalance)
	failed := performJSONRequest(superRouter, http.MethodPost, "/recharge", []byte(fmt.Sprintf(`{"user_id":%d,"amount":5}`, regular.ID)))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("failed recharge status=%d body=%s", failed.Code, failed.Body.String())
	}
	if err := db.First(&regular, regular.ID).Error; err != nil || regular.Balance != 10 {
		t.Fatalf("failed recharge changed balance=%v err=%v", regular.Balance, err)
	}
}

func TestServerUpdateAcceptsZeroMaxUsersAndRejectsUnboundedLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "server-input-bounds")
	server := model.Server{Name: "bounded-node", IP: "127.0.0.1", Status: "running", MaxUsers: 50}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewServerHandler(db, nil, "")
	router := gin.New()
	router.PUT("/servers/:id", handler.UpdateServer)
	router.GET("/servers/:id/logs", handler.GetServerLogs)
	updated := performJSONRequest(router, http.MethodPut, "/servers/"+fmt.Sprint(server.ID), []byte(`{"max_users":0}`))
	if updated.Code != http.StatusOK {
		t.Fatalf("zero max users status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := db.First(&server, server.ID).Error; err != nil || server.MaxUsers != 0 {
		t.Fatalf("max users=%d err=%v", server.MaxUsers, err)
	}
	logs := performJSONRequest(router, http.MethodGet, "/servers/"+fmt.Sprint(server.ID)+"/logs?lines=2001", nil)
	if logs.Code != http.StatusBadRequest {
		t.Fatalf("unbounded logs status=%d body=%s", logs.Code, logs.Body.String())
	}
}

func TestAdminAssignAndClearPlanUsesDurationAndGroupRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-plan-assignment")
	admin := model.User{Email: "plan-admin@example.com", Password: "x", InviteCode: "plan-admin", APIKey: "plan-admin-key", Role: "admin", Status: "active"}
	group := model.UserGroup{Name: "paid-group"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	plan := model.Plan{Name: "paid", DurationDays: 15, GroupID: &group.ID, Status: "active"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "plan-user@example.com", Password: "x", InviteCode: "plan-user", APIKey: "plan-user-key", Role: "user", Status: "active"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewUserHandler(db, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("role", "admin")
		c.Next()
	})
	router.POST("/users/:id/plan", handler.AdminAssignPlan)
	router.DELETE("/users/:id/plan", handler.AdminClearPlan)
	path := "/users/" + fmt.Sprint(user.ID) + "/plan"
	before := time.Now()
	assignRecorder := performJSONRequest(router, http.MethodPost, path, []byte(fmt.Sprintf(`{"plan_id":%d,"duration_type":"quarterly"}`, plan.ID)))
	if assignRecorder.Code != http.StatusOK {
		t.Fatalf("assign plan status = %d body=%s", assignRecorder.Code, assignRecorder.Body.String())
	}
	db.First(&user, user.ID)
	if user.PlanID == nil || *user.PlanID != plan.ID || user.GroupID == nil || *user.GroupID != group.ID || user.GroupSource != "plan" {
		t.Fatalf("assigned user = %+v", user)
	}
	wantMin := before.AddDate(0, 0, 45)
	if user.PlanExpiresAt == nil || user.PlanExpiresAt.Before(wantMin.Add(-time.Minute)) || user.PlanExpiresAt.After(wantMin.Add(time.Minute)) {
		t.Fatalf("plan expiry = %v want near %v", user.PlanExpiresAt, wantMin)
	}
	clearRecorder := performJSONRequest(router, http.MethodDelete, path, nil)
	if clearRecorder.Code != http.StatusOK {
		t.Fatalf("clear plan status = %d body=%s", clearRecorder.Code, clearRecorder.Body.String())
	}
	var cleared model.User
	db.First(&cleared, user.ID)
	if cleared.PlanID != nil || cleared.PlanExpiresAt != nil || cleared.GroupID != nil || cleared.GroupSource != "" {
		t.Fatalf("cleared user retained plan state: %+v", cleared)
	}
}

func performJSONRequest(router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, req)
	return recorder
}

func openAdminHardeningDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.UserGroup{}, &model.Plan{}, &model.User{}, &model.Server{}, &model.Proxy{},
		&model.Order{}, &model.PlanEntitlement{}, &model.TrafficDaily{}, &model.Setting{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}
