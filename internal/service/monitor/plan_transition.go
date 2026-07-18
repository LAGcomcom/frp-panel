package monitor

import (
	"log"
	"time"

	"github.com/frp-panel/frp-panel/internal/pkg/accesscontrol"
	"gorm.io/gorm"
)

func StartPlanTransitionJob(db *gorm.DB) {
	run := func() {
		if err := accesscontrol.ReconcilePlanEntitlements(db, time.Now()); err != nil {
			log.Printf("[PlanTransition] Reconciliation failed: %v", err)
		}
	}
	run()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
	log.Println("[PlanTransition] Entitlement transition job started")
}
