package main

import (
	"path/filepath"
	"testing"
	"time"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSQLiteDSNAddsContentionProtection(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain path", in: "/var/lib/frp-panel/frp-panel.db", want: "/var/lib/frp-panel/frp-panel.db?_pragma=busy_timeout%285000%29&_txlock=immediate"},
		{name: "existing query", in: "file:test.db?mode=memory&cache=shared", want: "file:test.db?mode=memory&cache=shared&_pragma=busy_timeout%285000%29&_txlock=immediate"},
		{name: "preserve settings", in: "file:test.db?_pragma=busy_timeout%2810000%29&_txlock=exclusive", want: "file:test.db?_pragma=busy_timeout%2810000%29&_txlock=exclusive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sqliteDSN(tt.in); got != tt.want {
				t.Fatalf("sqliteDSN(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSQLiteDSNWaitsForConcurrentWriter(t *testing.T) {
	type probe struct {
		ID uint `gorm:"primaryKey"`
	}
	db, err := gorm.Open(sqlite.Open(sqliteDSN(filepath.Join(t.TempDir(), "busy.db"))), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&probe{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(2)

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := tx.Create(&probe{}).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- db.Create(&probe{}).Error }()
	time.Sleep(100 * time.Millisecond)
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("concurrent writer did not wait: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent writer remained blocked")
	}
}
