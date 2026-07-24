package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

func TestAdminUpdateCannotBypassBalanceOrStatusWorkflows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-user-workflow-guard")
	group := model.UserGroup{Name: "workflow-group"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	admin := model.User{Email: "workflow-admin@example.com", Password: "x", InviteCode: "workflow-admin", APIKey: "workflow-admin-key", Role: "admin", Status: "active"}
	regular := model.User{Email: "workflow-user@example.com", Password: "x", InviteCode: "workflow-user", APIKey: "workflow-user-key", Role: "user", Status: "active", Balance: 10}
	for _, user := range []*model.User{&admin, &regular} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("role", "admin")
		c.Next()
	})
	router.PUT("/users/:id", NewUserHandler(db, nil).AdminUpdateUser)

	for _, payload := range []string{`{"balance":999}`, `{"balance":0}`, `{"balance":null}`, `{"status":"banned"}`, `{"status":"unexpected"}`, `{"status":null}`, `{}`} {
		recorder := performJSONRequest(router, http.MethodPut, "/users/"+fmt.Sprint(regular.ID), []byte(payload))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("direct user mutation payload=%s status=%d body=%s", payload, recorder.Code, recorder.Body.String())
		}
	}
	if err := db.First(&regular, regular.ID).Error; err != nil || regular.Balance != 10 || regular.Status != "active" {
		t.Fatalf("rejected direct mutation changed user: balance=%v status=%q err=%v", regular.Balance, regular.Status, err)
	}

	allowed := performJSONRequest(router, http.MethodPut, "/users/"+fmt.Sprint(regular.ID), []byte(fmt.Sprintf(`{"group_id":%d,"bandwidth_limit":1048576}`, group.ID)))
	if allowed.Code != http.StatusOK {
		t.Fatalf("allowed access update status=%d body=%s", allowed.Code, allowed.Body.String())
	}
	if err := db.First(&regular, regular.ID).Error; err != nil || regular.GroupID == nil || *regular.GroupID != group.ID || regular.GroupSource != "manual" || regular.BandwidthLimit != 1048576 {
		t.Fatalf("allowed access update user=%+v err=%v", regular, err)
	}
	cleared := performJSONRequest(router, http.MethodPut, "/users/"+fmt.Sprint(regular.ID), []byte(`{"clear_group":true}`))
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear group status=%d body=%s", cleared.Code, cleared.Body.String())
	}
	if err := db.First(&regular, regular.ID).Error; err != nil || regular.GroupID != nil || regular.GroupSource != "" || regular.BandwidthLimit != 1048576 {
		t.Fatalf("clear group changed unrelated access settings: user=%+v err=%v", regular, err)
	}
}

func TestAdminUserListSupportsOperationalFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-user-filters")
	paidGroup := model.UserGroup{Name: "paid-filter-group"}
	if err := db.Create(&paidGroup).Error; err != nil {
		t.Fatal(err)
	}
	admin := model.User{Email: "filter-admin@example.com", Password: "x", InviteCode: "filter-admin", APIKey: "filter-admin-key", Role: "admin", Status: "active"}
	activeUser := model.User{Email: "filter-user@example.com", Password: "x", InviteCode: "invite-filter-user", APIKey: "filter-user-key", Role: "user", Status: "active", GroupID: &paidGroup.ID}
	bannedUser := model.User{Email: "banned-filter@example.com", Password: "x", InviteCode: "invite-banned", APIKey: "banned-filter-key", Role: "user", Status: "banned"}
	for _, user := range []*model.User{&admin, &activeUser, &bannedUser} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}

	router := gin.New()
	router.GET("/users", NewUserHandler(db, nil).ListUsers)

	tests := []struct {
		name   string
		query  string
		wantID uint
	}{
		{name: "email keyword", query: "keyword=filter-admin@example.com", wantID: admin.ID},
		{name: "invite keyword", query: "keyword=invite-filter-user", wantID: activeUser.ID},
		{name: "api key keyword", query: "keyword=banned-filter-key", wantID: bannedUser.ID},
		{name: "role and status", query: "role=admin&status=active", wantID: admin.ID},
		{name: "group filter", query: "group_id=" + fmt.Sprint(paidGroup.ID), wantID: activeUser.ID},
		{name: "ungrouped and banned", query: "group_id=none&status=banned", wantID: bannedUser.ID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performJSONRequest(router, http.MethodGet, "/users?"+tt.query, nil)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Data struct {
					List  []model.User `json:"list"`
					Total int64        `json:"total"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Data.Total != 1 || len(payload.Data.List) != 1 || payload.Data.List[0].ID != tt.wantID {
				t.Fatalf("filtered users=%+v total=%d want id=%d", payload.Data.List, payload.Data.Total, tt.wantID)
			}
		})
	}

	pageTwo := performJSONRequest(router, http.MethodGet, "/users?role=user&page=2&size=1", nil)
	if pageTwo.Code != http.StatusOK {
		t.Fatalf("page two status=%d body=%s", pageTwo.Code, pageTwo.Body.String())
	}
	var pagedPayload struct {
		Data struct {
			List  []model.User `json:"list"`
			Total int64        `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(pageTwo.Body.Bytes(), &pagedPayload); err != nil {
		t.Fatal(err)
	}
	if pagedPayload.Data.Total != 2 || len(pagedPayload.Data.List) != 1 || pagedPayload.Data.List[0].ID != activeUser.ID {
		t.Fatalf("page two users=%+v total=%d want id=%d", pagedPayload.Data.List, pagedPayload.Data.Total, activeUser.ID)
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

func TestQueuedBalanceOrderRefundRestoresBalanceAndRemovesEntitlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "queued-balance-refund")
	activePlan := model.Plan{Name: "Current", DurationDays: 30, Status: "active"}
	queuedPlan := model.Plan{Name: "Next", DurationDays: 30, Status: "active"}
	if err := db.Create(&activePlan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&queuedPlan).Error; err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().AddDate(0, 0, 20)
	user := model.User{
		Email: "queued-refund@example.com", Password: "x", InviteCode: "queued-refund", APIKey: "queued-refund-key",
		Role: "user", Status: "active", Balance: 10, PlanID: &activePlan.ID, PlanExpiresAt: &expiresAt,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	order := model.Order{
		UserID: user.ID, PlanID: queuedPlan.ID, OrderNo: "queued-balance-order", OrderType: "plan",
		Amount: 9.9, OriginalAmount: 9.9, DurationType: "monthly", PayMethod: "balance", PayStatus: "paid", PaidAt: &now,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	entitlement := model.PlanEntitlement{
		UserID: user.ID, PlanID: queuedPlan.ID, OrderID: order.ID, DurationDays: 30, Status: model.PlanEntitlementQueued,
	}
	if err := db.Create(&entitlement).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/orders/:id/refund", NewOrderHandler(db).RefundOrder)
	recorder := performJSONRequest(router, http.MethodPost, "/orders/"+fmt.Sprint(order.ID)+"/refund", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("refund status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(&user, user.ID).Error; err != nil || math.Abs(user.Balance-19.9) > 0.000001 {
		t.Fatalf("refunded balance=%v err=%v", user.Balance, err)
	}
	if err := db.First(&order, order.ID).Error; err != nil || order.PayStatus != "refunded" {
		t.Fatalf("refunded order status=%q err=%v", order.PayStatus, err)
	}
	if err := db.First(&entitlement, entitlement.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("queued entitlement still exists: %+v err=%v", entitlement, err)
	}
	if err := db.First(&user, user.ID).Error; err != nil || user.PlanID == nil || *user.PlanID != activePlan.ID || !user.PlanExpiresAt.Equal(expiresAt) {
		t.Fatalf("active plan changed after queued refund: %+v err=%v", user, err)
	}

	second := performJSONRequest(router, http.MethodPost, "/orders/"+fmt.Sprint(order.ID)+"/refund", nil)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("duplicate refund status = %d body=%s", second.Code, second.Body.String())
	}
	if err := db.First(&user, user.ID).Error; err != nil || math.Abs(user.Balance-19.9) > 0.000001 {
		t.Fatalf("duplicate refund changed balance=%v err=%v", user.Balance, err)
	}
}

func TestActiveBalanceOrderCannotBeRefundedAutomatically(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "active-balance-refund")
	plan := model.Plan{Name: "Active", DurationDays: 30, Status: "active"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().AddDate(0, 0, 30)
	user := model.User{
		Email: "active-refund@example.com", Password: "x", InviteCode: "active-refund", APIKey: "active-refund-key",
		Role: "user", Status: "active", Balance: 5, PlanID: &plan.ID, PlanExpiresAt: &expiresAt,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	order := model.Order{
		UserID: user.ID, PlanID: plan.ID, OrderNo: "active-balance-order", OrderType: "plan",
		Amount: 20, OriginalAmount: 20, DurationType: "monthly", PayMethod: "balance", PayStatus: "paid", PaidAt: &now,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	entitlement := model.PlanEntitlement{
		UserID: user.ID, PlanID: plan.ID, OrderID: order.ID, DurationDays: 30, Status: model.PlanEntitlementActive,
		StartsAt: &now, ExpiresAt: &expiresAt, ActivatedAt: &now,
	}
	if err := db.Create(&entitlement).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/orders/:id/refund", NewOrderHandler(db).RefundOrder)
	recorder := performJSONRequest(router, http.MethodPost, "/orders/"+fmt.Sprint(order.ID)+"/refund", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("active refund status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(&order, order.ID).Error; err != nil || order.PayStatus != "paid" {
		t.Fatalf("active order changed: status=%q err=%v", order.PayStatus, err)
	}
	if err := db.First(&user, user.ID).Error; err != nil || user.Balance != 5 {
		t.Fatalf("active refund changed balance=%v err=%v", user.Balance, err)
	}
}

func TestRefundabilityMetadataMatchesBackendAccountingRules(t *testing.T) {
	db := openAdminHardeningDB(t, "refundability-metadata")
	queued := &model.PlanEntitlement{Status: model.PlanEntitlementQueued}
	extended := &model.PlanEntitlement{Status: model.PlanEntitlementExtended}
	expired := &model.PlanEntitlement{Status: model.PlanEntitlementExpired}
	orders := []model.Order{
		{ID: 11, PayStatus: "paid", PayMethod: "balance", OrderType: "plan", Entitlement: queued, User: model.User{Role: "user"}},
		{ID: 12, PayStatus: "paid", PayMethod: "balance", OrderType: "plan", Entitlement: queued, CouponCode: "SAVE10", User: model.User{Role: "user"}},
		{ID: 13, PayStatus: "paid", PayMethod: "balance", OrderType: "plan", Entitlement: queued, User: model.User{Role: "user"}},
		{ID: 14, PayStatus: "paid", PayMethod: "balance", OrderType: "plan", Entitlement: extended, User: model.User{Role: "user"}},
		{ID: 15, PayStatus: "paid", PayMethod: "balance", OrderType: "plan", Entitlement: expired, User: model.User{Role: "user"}},
		{ID: 16, PayStatus: "paid", PayMethod: "alipay", OrderType: "plan", Entitlement: queued, User: model.User{Role: "user"}},
	}
	if err := db.Create(&model.ReferralRebate{
		ReferredUserID: 1, OrderID: 13, Level1UserID: 2, Level1Amount: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := NewOrderHandler(db).annotateRefundability("admin", orders); err != nil {
		t.Fatal(err)
	}
	for i := range orders {
		if i == 0 {
			if !orders[i].Refundable || orders[i].RefundUnavailableReason != "" {
				t.Fatalf("eligible order metadata = %+v", orders[i])
			}
			continue
		}
		if orders[i].Refundable || orders[i].RefundUnavailableReason == "" {
			t.Fatalf("ineligible order %d metadata = %+v", orders[i].ID, orders[i])
		}
	}
}

func TestRefundabilityMetadataRespectsAdministratorScope(t *testing.T) {
	db := openAdminHardeningDB(t, "refundability-admin-scope")
	queued := &model.PlanEntitlement{Status: model.PlanEntitlementQueued}
	makeOrders := func() []model.Order {
		return []model.Order{
			{ID: 21, PayStatus: "paid", PayMethod: "balance", OrderType: "plan", Entitlement: queued, User: model.User{Role: "user"}},
			{ID: 22, PayStatus: "paid", PayMethod: "balance", OrderType: "plan", Entitlement: queued, User: model.User{Role: "admin"}},
			{ID: 23, PayStatus: "paid", PayMethod: "balance", OrderType: "plan", Entitlement: queued, User: model.User{Role: "super_admin"}},
		}
	}

	adminOrders := makeOrders()
	if err := NewOrderHandler(db).annotateRefundability("admin", adminOrders); err != nil {
		t.Fatal(err)
	}
	if !adminOrders[0].Refundable || adminOrders[1].Refundable || adminOrders[2].Refundable {
		t.Fatalf("admin refundability metadata = %+v", adminOrders)
	}
	for _, order := range adminOrders[1:] {
		if order.RefundUnavailableReason == "" {
			t.Fatalf("missing authorization reason for order %d", order.ID)
		}
	}

	superOrders := makeOrders()
	if err := NewOrderHandler(db).annotateRefundability("super_admin", superOrders); err != nil {
		t.Fatal(err)
	}
	for _, order := range superOrders {
		if !order.Refundable || order.RefundUnavailableReason != "" {
			t.Fatalf("super-admin order metadata = %+v", order)
		}
	}
}

func TestUserOrderResponsesHideAdminAuditFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "order-audit-privacy")
	user := model.User{Email: "audit-user@example.com", Password: "x", InviteCode: "audit-user", APIKey: "audit-user-key", Role: "user", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	order := model.Order{
		UserID: user.ID, OrderNo: "audit-private-order", OrderType: "recharge", Amount: 5, OriginalAmount: 5,
		DurationType: "recharge", PayMethod: "admin", PayStatus: "paid", Remark: "internal note",
		OperatorID: 99, OperatorEmail: "operator@example.com",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	userRouter := gin.New()
	userRouter.Use(func(c *gin.Context) { c.Set("user_id", user.ID); c.Next() })
	userRouter.GET("/orders", NewOrderHandler(db).ListOrders)
	userResponse := performJSONRequest(userRouter, http.MethodGet, "/orders", nil)
	if userResponse.Code != http.StatusOK {
		t.Fatalf("user order list status=%d body=%s", userResponse.Code, userResponse.Body.String())
	}
	for _, secret := range []string{"internal note", "operator@example.com", `"operator_id"`, `"remark"`} {
		if strings.Contains(userResponse.Body.String(), secret) {
			t.Fatalf("user order response exposed %q: %s", secret, userResponse.Body.String())
		}
	}

	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) { c.Set("role", "super_admin"); c.Next() })
	adminRouter.GET("/orders", NewOrderHandler(db).AdminListOrders)
	adminResponse := performJSONRequest(adminRouter, http.MethodGet, "/orders", nil)
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin order list status=%d body=%s", adminResponse.Code, adminResponse.Body.String())
	}
	for _, auditValue := range []string{"internal note", "operator@example.com", `"operator_id":99`} {
		if !strings.Contains(adminResponse.Body.String(), auditValue) {
			t.Fatalf("admin order response missing %q: %s", auditValue, adminResponse.Body.String())
		}
	}
}

func TestAdminOrderListSupportsOperationalFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-order-filters")
	alice := model.User{Email: "alice-filter@example.com", Password: "x", InviteCode: "alice-filter", APIKey: "alice-filter-key", Role: "user", Status: "active"}
	bob := model.User{Email: "bob-filter@example.com", Password: "x", InviteCode: "bob-filter", APIKey: "bob-filter-key", Role: "user", Status: "active"}
	for _, user := range []*model.User{&alice, &bob} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	orders := []model.Order{
		{UserID: alice.ID, OrderNo: "ORD-ALPHA-FILTER", OrderType: "plan", Amount: 10, DurationType: "monthly", PayMethod: "balance", PayStatus: "paid", Remark: "manual-note-alpha", OperatorEmail: "operator-alpha@example.com"},
		{UserID: bob.ID, OrderNo: "RCH-BETA-FILTER", OrderType: "recharge", Amount: 20, DurationType: "recharge", PayMethod: "admin", PayStatus: "refunded", Remark: "manual-note-beta", OperatorEmail: "operator-beta@example.com"},
		{UserID: alice.ID, OrderNo: "RCH-GAMMA-FILTER", OrderType: "recharge", Amount: 30, DurationType: "recharge", PayMethod: "admin", PayStatus: "paid", Remark: "manual-note-gamma", OperatorEmail: "operator-gamma@example.com"},
	}
	if err := db.Create(&orders).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("role", "super_admin"); c.Next() })
	router.GET("/orders", NewOrderHandler(db).AdminListOrders)

	tests := []struct {
		name   string
		query  string
		wantID uint
	}{
		{name: "order number", query: "keyword=ALPHA-FILTER", wantID: orders[0].ID},
		{name: "user email with filters", query: "keyword=bob-filter@example.com&status=refunded&order_type=recharge", wantID: orders[1].ID},
		{name: "accounting remark", query: "keyword=manual-note-beta", wantID: orders[1].ID},
		{name: "audit details", query: "keyword=operator-alpha@example.com", wantID: orders[0].ID},
		{name: "status and type", query: "status=refunded&order_type=recharge", wantID: orders[1].ID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performJSONRequest(router, http.MethodGet, "/orders?"+tt.query, nil)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Data struct {
					List  []model.Order `json:"list"`
					Total int64         `json:"total"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Data.Total != 1 || len(payload.Data.List) != 1 || payload.Data.List[0].ID != tt.wantID {
				t.Fatalf("filtered orders=%+v total=%d want id=%d", payload.Data.List, payload.Data.Total, tt.wantID)
			}
		})
	}

	pageTwo := performJSONRequest(router, http.MethodGet, "/orders?keyword=alice-filter@example.com&page=2&size=1", nil)
	if pageTwo.Code != http.StatusOK {
		t.Fatalf("page two status=%d body=%s", pageTwo.Code, pageTwo.Body.String())
	}
	var pagedPayload struct {
		Data struct {
			List  []model.Order `json:"list"`
			Total int64         `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(pageTwo.Body.Bytes(), &pagedPayload); err != nil {
		t.Fatal(err)
	}
	if pagedPayload.Data.Total != 2 || len(pagedPayload.Data.List) != 1 || pagedPayload.Data.List[0].ID != orders[0].ID {
		t.Fatalf("page two orders=%+v total=%d want id=%d", pagedPayload.Data.List, pagedPayload.Data.Total, orders[0].ID)
	}
}

func TestQueuedRefundRejectsCouponAndReferralWithoutMutation(t *testing.T) {
	for _, scenario := range []string{"coupon", "referral"} {
		t.Run(scenario, func(t *testing.T) {
			db := openAdminHardeningDB(t, "queued-refund-"+scenario)
			plan := model.Plan{Name: "Queued", DurationDays: 30, Status: "active"}
			if err := db.Create(&plan).Error; err != nil {
				t.Fatal(err)
			}
			user := model.User{
				Email: scenario + "@example.com", Password: "x", InviteCode: scenario, APIKey: scenario + "-key",
				Role: "user", Status: "active", Balance: 8,
			}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			order := model.Order{
				UserID: user.ID, PlanID: plan.ID, OrderNo: "queued-" + scenario, OrderType: "plan",
				Amount: 12, OriginalAmount: 12, DurationType: "monthly", PayMethod: "balance", PayStatus: "paid",
			}
			if scenario == "coupon" {
				order.CouponCode = "SAVE10"
			}
			if err := db.Create(&order).Error; err != nil {
				t.Fatal(err)
			}
			entitlement := model.PlanEntitlement{
				UserID: user.ID, PlanID: plan.ID, OrderID: order.ID, DurationDays: 30, Status: model.PlanEntitlementQueued,
			}
			if err := db.Create(&entitlement).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "referral" {
				if err := db.Create(&model.ReferralRebate{
					ReferredUserID: user.ID, OrderID: order.ID, Level1UserID: user.ID, Level1Amount: 1,
				}).Error; err != nil {
					t.Fatal(err)
				}
			}

			router := gin.New()
			router.POST("/orders/:id/refund", NewOrderHandler(db).RefundOrder)
			recorder := performJSONRequest(router, http.MethodPost, "/orders/"+fmt.Sprint(order.ID)+"/refund", nil)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("refund status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if err := db.First(&order, order.ID).Error; err != nil || order.PayStatus != "paid" {
				t.Fatalf("rejected order changed: status=%q err=%v", order.PayStatus, err)
			}
			if err := db.First(&user, user.ID).Error; err != nil || user.Balance != 8 {
				t.Fatalf("rejected refund changed balance=%v err=%v", user.Balance, err)
			}
			if err := db.First(&entitlement, entitlement.ID).Error; err != nil || entitlement.Status != model.PlanEntitlementQueued {
				t.Fatalf("rejected refund changed entitlement=%+v err=%v", entitlement, err)
			}
		})
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

func TestAdminProxyListSupportsOperationalFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-proxy-filters")
	alice := model.User{Email: "alice-proxy@example.com", Password: "x", InviteCode: "alice-proxy", APIKey: "alice-proxy-key", Role: "user", Status: "active"}
	bob := model.User{Email: "bob-proxy@example.com", Password: "x", InviteCode: "bob-proxy", APIKey: "bob-proxy-key", Role: "user", Status: "active"}
	edge := model.Server{Name: "edge-hk", IP: "10.0.0.10", Region: "HK", Status: "running"}
	backup := model.Server{Name: "backup-us", IP: "10.0.1.20", Region: "US", Status: "running"}
	for _, value := range []any{&alice, &bob, &edge, &backup} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	proxies := []model.Proxy{
		{UserID: alice.ID, ServerID: edge.ID, Name: "rdp-alpha", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 3389, RemotePort: 60001, Status: "running", Enabled: true, RemoteAddr: "10.0.0.10:60001"},
		{UserID: bob.ID, ServerID: backup.ID, Name: "site-beta", Type: "http", LocalIP: "127.0.0.1", LocalPort: 8080, Status: "pending", Enabled: false, Subdomain: "beta", CustomDomains: `["beta.example.com"]`},
		{UserID: alice.ID, ServerID: backup.ID, Name: "api-gamma", Type: "https", LocalIP: "127.0.0.1", LocalPort: 8443, Status: "error", Enabled: true, CustomDomains: `["api.example.com"]`},
	}
	if err := db.Create(&proxies).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/proxies", NewProxyHandler(db).AdminListProxies)

	tests := []struct {
		name   string
		query  string
		wantID uint
	}{
		{name: "proxy name", query: "keyword=rdp-alpha", wantID: proxies[0].ID},
		{name: "user email with enabled", query: "keyword=bob-proxy@example.com&enabled=false", wantID: proxies[1].ID},
		{name: "server name and type", query: "keyword=edge-hk&type=tcp", wantID: proxies[0].ID},
		{name: "domain and status", query: "keyword=api.example.com&status=error", wantID: proxies[2].ID},
		{name: "type status and enabled", query: "type=http&status=pending&enabled=false", wantID: proxies[1].ID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performJSONRequest(router, http.MethodGet, "/proxies?"+tt.query, nil)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Data struct {
					List  []model.Proxy `json:"list"`
					Total int64         `json:"total"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Data.Total != 1 || len(payload.Data.List) != 1 || payload.Data.List[0].ID != tt.wantID {
				t.Fatalf("filtered proxies=%+v total=%d want id=%d", payload.Data.List, payload.Data.Total, tt.wantID)
			}
		})
	}

	pageTwo := performJSONRequest(router, http.MethodGet, "/proxies?keyword=alice-proxy@example.com&page=2&size=1", nil)
	if pageTwo.Code != http.StatusOK {
		t.Fatalf("page two status=%d body=%s", pageTwo.Code, pageTwo.Body.String())
	}
	var pagedPayload struct {
		Data struct {
			List  []model.Proxy `json:"list"`
			Total int64         `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(pageTwo.Body.Bytes(), &pagedPayload); err != nil {
		t.Fatal(err)
	}
	if pagedPayload.Data.Total != 2 || len(pagedPayload.Data.List) != 1 || pagedPayload.Data.List[0].ID != proxies[0].ID {
		t.Fatalf("page two proxies=%+v total=%d want id=%d", pagedPayload.Data.List, pagedPayload.Data.Total, proxies[0].ID)
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

func TestInviteRebateCanBeDisabled(t *testing.T) {
	db := openAdminHardeningDB(t, "invite-rebate-disabled")
	inviter := model.User{Email: "inviter@example.com", Password: "x", InviteCode: "inviter", APIKey: "inviter-key", Role: "user", Status: "active"}
	if err := db.Create(&inviter).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "invited-rebate@example.com", Password: "x", InviteCode: "invited-rebate", APIKey: "invited-rebate-key", Role: "user", Status: "active", InvitedBy: &inviter.ID}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Setting{Key: "invite_rebate_enabled", Value: "false"}).Error; err != nil {
		t.Fatal(err)
	}
	order := model.Order{UserID: user.ID, OrderNo: "rebate-disabled-order", OrderType: "recharge", Amount: 30, OriginalAmount: 30, DurationType: "recharge", PayMethod: "alipay", PayStatus: "pending"}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	processed, err := NewOrderHandler(db).confirmOrderPayment(&order, user.ID, "rebate-disabled-trade")
	if err != nil || !processed {
		t.Fatalf("confirmation with disabled rebate: processed=%v err=%v", processed, err)
	}
	if err := db.First(&inviter, inviter.ID).Error; err != nil || inviter.Balance != 0 {
		t.Fatalf("disabled rebate credited inviter: balance=%v err=%v", inviter.Balance, err)
	}
	var rebateCount int64
	db.Model(&model.ReferralRebate{}).Where("order_id = ?", order.ID).Count(&rebateCount)
	if rebateCount != 0 {
		t.Fatalf("disabled rebate created %d records", rebateCount)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", inviter.ID)
		c.Next()
	})
	router.GET("/invite-stats", NewUserHandler(db, nil).GetInviteStats)
	recorder := performJSONRequest(router, http.MethodGet, "/invite-stats", nil)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("disabled invite stats status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicSettingsExposeInviteRebateDefaultAndOverride(t *testing.T) {
	db := openAdminHardeningDB(t, "public-invite-rebate-setting")
	router := gin.New()
	router.GET("/settings/public", NewSettingHandler(db).GetPublicSettings)

	recorder := performJSONRequest(router, http.MethodGet, "/settings/public", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("public settings status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data["invite_rebate_enabled"] != "true" {
		t.Fatalf("default invite rebate setting=%q", payload.Data["invite_rebate_enabled"])
	}

	if err := db.Create(&model.Setting{Key: "invite_rebate_enabled", Value: "false"}).Error; err != nil {
		t.Fatal(err)
	}
	recorder = performJSONRequest(router, http.MethodGet, "/settings/public", nil)
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data["invite_rebate_enabled"] != "false" {
		t.Fatalf("overridden invite rebate setting=%q", payload.Data["invite_rebate_enabled"])
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
	missingActorRouter := gin.New()
	missingActorRouter.Use(func(c *gin.Context) { c.Set("role", "super_admin"); c.Next() })
	missingActorRouter.POST("/recharge", handler.RechargeBalance)
	missingActor := performJSONRequest(missingActorRouter, http.MethodPost, "/recharge", []byte(fmt.Sprintf(`{"user_id":%d,"amount":5}`, regular.ID)))
	if missingActor.Code != http.StatusUnauthorized {
		t.Fatalf("missing actor recharge status=%d body=%s", missingActor.Code, missingActor.Body.String())
	}

	superRouter := gin.New()
	superRouter.Use(func(c *gin.Context) {
		c.Set("user_id", superAdmin.ID)
		c.Set("email", superAdmin.Email)
		c.Set("role", "super_admin")
		c.Next()
	})
	superRouter.POST("/recharge", handler.RechargeBalance)
	longRemark := performJSONRequest(superRouter, http.MethodPost, "/recharge", []byte(fmt.Sprintf(`{"user_id":%d,"amount":5,"remark":%q}`, regular.ID, strings.Repeat("a", 501))))
	if longRemark.Code != http.StatusBadRequest {
		t.Fatalf("long recharge remark status=%d body=%s", longRemark.Code, longRemark.Body.String())
	}
	succeeded := performJSONRequest(superRouter, http.MethodPost, "/recharge", []byte(fmt.Sprintf(`{"user_id":%d,"amount":5,"remark":"accounting test"}`, regular.ID)))
	if succeeded.Code != http.StatusOK {
		t.Fatalf("successful recharge status=%d body=%s", succeeded.Code, succeeded.Body.String())
	}
	if err := db.First(&regular, regular.ID).Error; err != nil || regular.Balance != 15 {
		t.Fatalf("successful recharge balance=%v err=%v", regular.Balance, err)
	}
	var rechargeOrder model.Order
	if err := db.Where("user_id = ? AND order_type = ?", regular.ID, "recharge").First(&rechargeOrder).Error; err != nil || rechargeOrder.Amount != 5 || rechargeOrder.PayMethod != "admin" || rechargeOrder.PayStatus != "paid" || rechargeOrder.Remark != "accounting test" || rechargeOrder.OperatorID != superAdmin.ID || rechargeOrder.OperatorEmail != superAdmin.Email {
		t.Fatalf("successful recharge order=%+v err=%v", rechargeOrder, err)
	}

	if err := db.Callback().Create().Before("gorm:create").Register("test:fail-admin-recharge-order", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Order" {
			tx.AddError(errors.New("forced order create failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failed := performJSONRequest(superRouter, http.MethodPost, "/recharge", []byte(fmt.Sprintf(`{"user_id":%d,"amount":5}`, regular.ID)))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("failed recharge status=%d body=%s", failed.Code, failed.Body.String())
	}
	if err := db.First(&regular, regular.ID).Error; err != nil || regular.Balance != 15 {
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

func TestAdminServerListSupportsOperationalFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAdminHardeningDB(t, "admin-server-filters")
	servers := []model.Server{
		{Name: "alpha-node", IP: "10.0.0.10", Region: "HK", FrpVersion: "0.68.0", Status: "stopped", BindPort: 7000},
		{Name: "beta-node", IP: "10.0.1.20", Region: "US", FrpVersion: "0.61.1", Status: "error", BindPort: 7000},
		{Name: "gamma-node", IP: "172.16.0.30", Region: "HK", FrpVersion: "0.68.0", Status: "pending", BindPort: 7000},
	}
	if err := db.Create(&servers).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/servers", NewServerHandler(db, nil, "").ListServers)

	tests := []struct {
		name   string
		query  string
		wantID uint
	}{
		{name: "name keyword", query: "keyword=alpha", wantID: servers[0].ID},
		{name: "ip keyword", query: "keyword=10.0.1.20", wantID: servers[1].ID},
		{name: "version and status", query: "keyword=0.61.1&status=error", wantID: servers[1].ID},
		{name: "region and status", query: "region=HK&status=pending", wantID: servers[2].ID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performJSONRequest(router, http.MethodGet, "/servers?"+tt.query, nil)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Data struct {
					List  []model.Server `json:"list"`
					Total int64          `json:"total"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Data.Total != 1 || len(payload.Data.List) != 1 || payload.Data.List[0].ID != tt.wantID {
				t.Fatalf("filtered servers=%+v total=%d want id=%d", payload.Data.List, payload.Data.Total, tt.wantID)
			}
		})
	}

	pageTwo := performJSONRequest(router, http.MethodGet, "/servers?keyword=0.68.0&page=2&size=1", nil)
	if pageTwo.Code != http.StatusOK {
		t.Fatalf("page two status=%d body=%s", pageTwo.Code, pageTwo.Body.String())
	}
	var pagedPayload struct {
		Data struct {
			List  []model.Server `json:"list"`
			Total int64          `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(pageTwo.Body.Bytes(), &pagedPayload); err != nil {
		t.Fatal(err)
	}
	if pagedPayload.Data.Total != 2 || len(pagedPayload.Data.List) != 1 || pagedPayload.Data.List[0].ID != servers[0].ID {
		t.Fatalf("page two servers=%+v total=%d want id=%d", pagedPayload.Data.List, pagedPayload.Data.Total, servers[0].ID)
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
		&model.Order{}, &model.PlanEntitlement{}, &model.ReferralRebate{}, &model.TrafficDaily{}, &model.Setting{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}
