package model

import (
	"testing"
	"time"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPlanEntitlementMigrationPreservesExistingData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:entitlement-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Plan{}, &User{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE orders (
		id integer PRIMARY KEY AUTOINCREMENT,
		user_id integer NOT NULL,
		plan_id integer NOT NULL,
		order_no text NOT NULL UNIQUE,
		order_type text NOT NULL,
		amount real NOT NULL,
		duration_type text,
		pay_method text,
		pay_status text NOT NULL,
		deleted_at datetime
	)`).Error; err != nil {
		t.Fatal(err)
	}
	plan := Plan{Name: "Existing", DurationDays: 30, Status: "active"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().AddDate(0, 0, 30)
	user := User{Email: "existing@example.com", Password: "x", InviteCode: "existing", APIKey: "existing-key", Status: "active", PlanID: &plan.ID, PlanExpiresAt: &expiresAt}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	order := Order{ID: 1, UserID: user.ID, PlanID: plan.ID, OrderNo: "existing-order", OrderType: "plan", Amount: 9.9, DurationType: "monthly", PayMethod: "balance", PayStatus: "paid"}
	if err := db.Exec("INSERT INTO orders (id, user_id, plan_id, order_no, order_type, amount, duration_type, pay_method, pay_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		order.ID, order.UserID, order.PlanID, order.OrderNo, order.OrderType, order.Amount, order.DurationType, order.PayMethod, order.PayStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&PlanEntitlement{}); err != nil {
		t.Fatal(err)
	}
	var gotUser User
	if err := db.First(&gotUser, user.ID).Error; err != nil || gotUser.PlanID == nil || *gotUser.PlanID != plan.ID {
		t.Fatalf("existing user changed after migration: %+v err=%v", gotUser, err)
	}
	var gotOrder Order
	if err := db.First(&gotOrder, order.ID).Error; err != nil || gotOrder.PayStatus != "paid" {
		t.Fatalf("existing order changed after migration: %+v err=%v", gotOrder, err)
	}
}
