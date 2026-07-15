package monitor

import (
	"testing"

	"github.com/frp-panel/frp-panel/internal/model"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openMonitorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.Server{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func TestLoadFreeTrafficLimit(t *testing.T) {
	db := openMonitorTestDB(t)

	if got := loadFreeTrafficLimit(db); got != defaultFreeTraffic {
		t.Fatalf("missing setting: got %d, want %d", got, defaultFreeTraffic)
	}
	if err := db.Create(&model.Setting{Key: "free_max_traffic_gb", Value: "2.5"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if got, want := loadFreeTrafficLimit(db), int64(2.5*1024*1024*1024); got != want {
		t.Fatalf("configured setting: got %d, want %d", got, want)
	}
	if err := db.Model(&model.Setting{}).Where("key = ?", "free_max_traffic_gb").Update("value", "invalid").Error; err != nil {
		t.Fatalf("update setting: %v", err)
	}
	if got := loadFreeTrafficLimit(db); got != defaultFreeTraffic {
		t.Fatalf("invalid setting: got %d, want %d", got, defaultFreeTraffic)
	}
}
