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
	if err := db.AutoMigrate(&model.Server{}, &model.UserGroup{}, &model.UserGroupServer{}, &model.Plan{}, &model.User{}); err != nil {
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
	if err := db.AutoMigrate(&model.Server{}, &model.UserGroup{}, &model.UserGroupServer{}, &model.Plan{}, &model.User{}); err != nil {
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
