package accesscontrol

import (
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGroupServerAccessAndBandwidthOverride(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:accesscontrol?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Server{}, &model.UserGroup{}, &model.UserGroupServer{}, &model.Plan{}, &model.User{}, &model.PlanEntitlement{}); err != nil {
		t.Fatal(err)
	}

	serverA := model.Server{Name: "A", IP: "127.0.0.1", PluginAuthEnabled: true}
	serverB := model.Server{Name: "B", IP: "127.0.0.2"}
	group := model.UserGroup{Name: "Group A"}
	plan := model.Plan{Name: "Plan", MaxBandwidth: 5 * 1024 * 1024, DurationDays: 30}
	for _, value := range []interface{}{&serverA, &serverB, &group, &plan} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&group).Association("Servers").Replace([]model.Server{serverA}); err != nil {
		t.Fatal(err)
	}

	ungrouped := model.User{Email: "free@example.com", Password: "x", APIKey: "free-key", InviteCode: "free-invite", Status: "active"}
	if err := db.Create(&ungrouped).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := CanAccessServer(db, &ungrouped, serverB.ID); err != nil || !allowed {
		t.Fatalf("legacy ungrouped user should retain access: allowed=%v err=%v", allowed, err)
	}

	user := model.User{
		Email: "group@example.com", Password: "x", APIKey: "group-key", InviteCode: "group-invite", Status: "active",
		GroupID: &group.ID, PlanID: &plan.ID, BandwidthLimit: 2 * 1024 * 1024,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, _ := CanAccessServer(db, &user, serverA.ID); !allowed {
		t.Fatal("grouped user should access an assigned server")
	}
	if allowed, _ := CanAccessServer(db, &user, serverB.ID); allowed {
		t.Fatal("grouped user must not access an unassigned server")
	}
	db.Model(&serverA).Update("plugin_auth_enabled", false)
	if allowed, _ := CanAccessServer(db, &user, serverA.ID); allowed {
		t.Fatal("grouped user retained access after secure auth was invalidated")
	}
	loaded, err := LoadUser(db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := EffectiveBandwidth(db, loaded); got != 2*1024*1024 {
		t.Fatalf("effective bandwidth = %d, want user override", got)
	}
}

func TestExpiredPlanEntitlementsAreRemovedOnEveryAccessPath(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:expired-entitlements?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Server{}, &model.UserGroup{}, &model.UserGroupServer{}, &model.Plan{}, &model.User{}, &model.PlanEntitlement{}); err != nil {
		t.Fatal(err)
	}

	server := model.Server{Name: "paid", IP: "127.0.0.1", PluginAuthEnabled: true}
	group := model.UserGroup{Name: "paid"}
	plan := model.Plan{Name: "paid", MaxBandwidth: 50 * 1024 * 1024, DurationDays: 30}
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
		Email: "expired@example.com", Password: "x", APIKey: "expired-key", InviteCode: "expired-invite", Status: "active",
		PlanID: &plan.ID, PlanExpiresAt: &expiredAt, GroupID: &group.ID, GroupSource: "plan",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if allowed, err := CanAccessServer(db, &user, server.ID); err != nil || allowed {
		t.Fatalf("expired plan retained node access: allowed=%v err=%v", allowed, err)
	}
	loaded, err := LoadUserByAPIKey(db, user.APIKey)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PlanID != nil || loaded.GroupID != nil || loaded.GroupSource != "expired_plan" {
		t.Fatalf("expired entitlements not normalized: plan=%v group=%v source=%q", loaded.PlanID, loaded.GroupID, loaded.GroupSource)
	}
	if got := EffectiveBandwidth(db, loaded); got != defaultFreeBandwidth {
		t.Fatalf("expired plan bandwidth = %d, want free bandwidth %d", got, defaultFreeBandwidth)
	}
}

func TestQueuedPlansActivateInPurchaseOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queued-plan-order?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserGroup{}, &model.Plan{}, &model.User{}, &model.PlanEntitlement{}); err != nil {
		t.Fatal(err)
	}
	current := model.Plan{Name: "Current", DurationDays: 30}
	firstGroup := model.UserGroup{Name: "First group"}
	secondGroup := model.UserGroup{Name: "Second group"}
	if err := db.Create(&firstGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&secondGroup).Error; err != nil {
		t.Fatal(err)
	}
	first := model.Plan{Name: "First queued", DurationDays: 10, GroupID: &firstGroup.ID}
	second := model.Plan{Name: "Second queued", DurationDays: 20, GroupID: &secondGroup.ID}
	for _, plan := range []*model.Plan{&current, &first, &second} {
		if err := db.Create(plan).Error; err != nil {
			t.Fatal(err)
		}
	}
	expiredAt := time.Now().Add(-time.Minute).Truncate(time.Second)
	user := model.User{
		Email: "queue-order@example.com", Password: "x", APIKey: "queue-order-key", InviteCode: "queue-order-invite", Status: "active",
		PlanID: &current.ID, PlanExpiresAt: &expiredAt,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	queued := []model.PlanEntitlement{
		{UserID: user.ID, PlanID: first.ID, OrderID: 101, DurationDays: 10, Status: model.PlanEntitlementQueued},
		{UserID: user.ID, PlanID: second.ID, OrderID: 102, DurationDays: 20, Status: model.PlanEntitlementQueued},
	}
	if err := db.Create(&queued).Error; err != nil {
		t.Fatal(err)
	}

	firstStart := expiredAt.Add(time.Minute)
	if err := ReconcilePlanEntitlements(db, firstStart); err != nil {
		t.Fatal(err)
	}
	if err := db.Preload("Plan").Preload("Group").First(&user, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.PlanID == nil || *user.PlanID != first.ID || user.PlanExpiresAt == nil || !user.PlanExpiresAt.Equal(expiredAt.AddDate(0, 0, 10)) ||
		user.GroupID == nil || *user.GroupID != firstGroup.ID || user.Group == nil || user.Group.ID != firstGroup.ID {
		t.Fatalf("first queued plan not activated: %+v", user)
	}
	secondBoundary := *user.PlanExpiresAt
	secondStart := secondBoundary.Add(time.Minute)
	if err := ExpireUserPlan(db, &user, secondStart); err != nil {
		t.Fatal(err)
	}
	if user.PlanID == nil || *user.PlanID != second.ID || user.PlanExpiresAt == nil || !user.PlanExpiresAt.Equal(secondBoundary.AddDate(0, 0, 20)) ||
		user.GroupID == nil || *user.GroupID != secondGroup.ID || user.Group == nil || user.Group.ID != secondGroup.ID {
		t.Fatalf("second queued plan not activated: %+v", user)
	}
	if err := db.First(&queued[0], queued[0].ID).Error; err != nil || queued[0].Status != model.PlanEntitlementExpired {
		t.Fatalf("first entitlement status=%q err=%v", queued[0].Status, err)
	}
	if err := db.First(&queued[1], queued[1].ID).Error; err != nil || queued[1].Status != model.PlanEntitlementActive {
		t.Fatalf("second entitlement status=%q err=%v", queued[1].Status, err)
	}
}

func TestReconcileCatchesUpElapsedQueuedPeriods(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queued-plan-catchup?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Plan{}, &model.User{}, &model.PlanEntitlement{}); err != nil {
		t.Fatal(err)
	}
	current := model.Plan{Name: "Expired current", DurationDays: 30}
	elapsed := model.Plan{Name: "Elapsed queued", DurationDays: 10}
	next := model.Plan{Name: "Current queued", DurationDays: 20}
	for _, plan := range []*model.Plan{&current, &elapsed, &next} {
		if err := db.Create(plan).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().Truncate(time.Second)
	boundary := now.AddDate(0, 0, -15)
	user := model.User{
		Email: "queue-catchup@example.com", Password: "x", APIKey: "queue-catchup-key", InviteCode: "queue-catchup-invite", Status: "active",
		PlanID: &current.ID, PlanExpiresAt: &boundary,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	items := []model.PlanEntitlement{
		{UserID: user.ID, PlanID: elapsed.ID, OrderID: 201, DurationDays: 10, Status: model.PlanEntitlementQueued},
		{UserID: user.ID, PlanID: next.ID, OrderID: 202, DurationDays: 20, Status: model.PlanEntitlementQueued},
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReconcilePlanEntitlements(db, now); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&user, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	wantExpiry := boundary.AddDate(0, 0, 30)
	if user.PlanID == nil || *user.PlanID != next.ID || user.PlanExpiresAt == nil || !user.PlanExpiresAt.Equal(wantExpiry) {
		t.Fatalf("catch-up selected wrong entitlement: user=%+v want expiry=%v", user, wantExpiry)
	}
	if err := db.First(&items[0], items[0].ID).Error; err != nil || items[0].Status != model.PlanEntitlementExpired {
		t.Fatalf("elapsed item status=%q err=%v", items[0].Status, err)
	}
	if err := db.First(&items[1], items[1].ID).Error; err != nil || items[1].Status != model.PlanEntitlementActive {
		t.Fatalf("current item status=%q err=%v", items[1].Status, err)
	}
}
