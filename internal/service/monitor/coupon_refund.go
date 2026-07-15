package monitor

import (
	"log"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"gorm.io/gorm"
)

var beijingTZ = time.FixedZone("CST", 8*3600)

func StartCouponRefundJob(db *gorm.DB) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			refundExpiredCoupons(db)
		}
	}()
	log.Println("[CouponRefund] Expired coupon refund job started")
}

func refundExpiredCoupons(db *gorm.DB) {
	now := time.Now().In(beijingTZ)
	var coupons []model.Coupon
	db.Where("creator_type = ? AND refund_status = ? AND end_time IS NOT NULL AND end_time < ?",
		"user", "none", now).Find(&coupons)

	for _, coupon := range coupons {
		if coupon.CreatedBy == nil || coupon.DeductedAmount <= 0 {
			continue
		}
		db.Transaction(func(tx *gorm.DB) error {
			tx.Model(&model.User{}).Where("id = ?", *coupon.CreatedBy).
				Update("balance", gorm.Expr("balance + ?", coupon.DeductedAmount))
			tx.Model(&model.Coupon{}).Where("id = ?", coupon.ID).
				Update("refund_status", "refunded")
			return nil
		})
	}
}
