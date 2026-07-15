package monitor

import (
	"log"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"gorm.io/gorm"
)

func StartOrderExpireJob(db *gorm.DB) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			expirePendingOrders(db)
		}
	}()
	log.Println("[OrderExpire] Pending order expire job started")
}

func expirePendingOrders(db *gorm.DB) {
	cutoff := time.Now().In(beijingTZ).Add(-30 * time.Minute)
	result := db.Model(&model.Order{}).
		Where("pay_status = ? AND created_at < ?", "pending", cutoff).
		Update("pay_status", "expired")
	if result.RowsAffected > 0 {
		log.Printf("[OrderExpire] Expired %d pending orders", result.RowsAffected)
	}
}
