package accesscontrol

import (
	"fmt"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
	"gorm.io/gorm"
)

const defaultFreeBandwidth = int64(10 * 1024 * 1024)

func LoadUser(db *gorm.DB, userID uint) (*model.User, error) {
	var user model.User
	if err := db.Preload("Plan").Preload("Group").First(&user, userID).Error; err != nil {
		return nil, err
	}
	if err := ExpireUserPlan(db, &user, time.Now()); err != nil {
		return nil, err
	}
	return &user, nil
}

func LoadUserByAPIKey(db *gorm.DB, apiKey string) (*model.User, error) {
	var user model.User
	if err := db.Preload("Plan").Preload("Group").Where("api_key = ?", apiKey).First(&user).Error; err != nil {
		return nil, err
	}
	if err := ExpireUserPlan(db, &user, time.Now()); err != nil {
		return nil, err
	}
	return &user, nil
}

func ExpireUserPlan(db *gorm.DB, user *model.User, now time.Time) error {
	if user == nil || user.PlanID == nil || user.PlanExpiresAt == nil || user.PlanExpiresAt.After(now) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var current model.User
		if err := tx.First(&current, user.ID).Error; err != nil {
			return err
		}
		if err := reconcileUserPlanTx(tx, &current, now); err != nil {
			return err
		}
		*user = current
		if current.PlanID != nil {
			var plan model.Plan
			if err := tx.First(&plan, *current.PlanID).Error; err != nil {
				return err
			}
			user.Plan = &plan
		}
		if current.GroupID != nil {
			var group model.UserGroup
			if err := tx.First(&group, *current.GroupID).Error; err != nil {
				return err
			}
			user.Group = &group
		}
		return nil
	})
}

// GrantPurchasedPlan delivers one paid plan order. Matching plans extend the
// current period; a different plan is queued until the active period expires.
func GrantPurchasedPlan(tx *gorm.DB, userID, orderID, planID uint, durationDays int, now time.Time) (string, error) {
	if durationDays <= 0 {
		return "", fmt.Errorf("invalid plan duration")
	}
	var plan model.Plan
	if err := tx.First(&plan, planID).Error; err != nil {
		return "", err
	}
	var user model.User
	if err := tx.First(&user, userID).Error; err != nil {
		return "", err
	}
	if user.PlanID != nil && user.PlanExpiresAt != nil && !user.PlanExpiresAt.After(now) {
		if err := reconcileUserPlanTx(tx, &user, now); err != nil {
			return "", err
		}
	}

	entitlement := model.PlanEntitlement{
		UserID:       userID,
		PlanID:       planID,
		OrderID:      orderID,
		DurationDays: durationDays,
	}
	active := user.PlanID != nil && user.PlanExpiresAt != nil && user.PlanExpiresAt.After(now)
	if active && *user.PlanID != planID {
		entitlement.Status = model.PlanEntitlementQueued
		if err := tx.Create(&entitlement).Error; err != nil {
			return "", err
		}
		return entitlement.Status, nil
	}

	startsAt := now
	status := model.PlanEntitlementActive
	if active {
		startsAt = *user.PlanExpiresAt
		status = model.PlanEntitlementExtended
	}
	expiresAt := startsAt.AddDate(0, 0, durationDays)
	entitlement.Status = status
	entitlement.StartsAt = &startsAt
	entitlement.ExpiresAt = &expiresAt
	entitlement.ActivatedAt = &now
	if err := tx.Create(&entitlement).Error; err != nil {
		return "", err
	}
	if err := applyPlanToUser(tx, &user, &plan, expiresAt); err != nil {
		return "", err
	}
	return status, nil
}

// ReconcilePlanEntitlements activates queued plans for expired or empty users.
func ReconcilePlanEntitlements(db *gorm.DB, now time.Time) error {
	if err := db.Model(&model.PlanEntitlement{}).
		Where("status IN ? AND expires_at IS NOT NULL AND expires_at <= ?",
			[]string{model.PlanEntitlementActive, model.PlanEntitlementExtended}, now).
		Update("status", model.PlanEntitlementExpired).Error; err != nil {
		return err
	}
	userIDs := make(map[uint]struct{})
	var expiredIDs []uint
	if err := db.Model(&model.User{}).
		Where("plan_id IS NOT NULL AND plan_expires_at IS NOT NULL AND plan_expires_at <= ?", now).
		Pluck("id", &expiredIDs).Error; err != nil {
		return err
	}
	for _, id := range expiredIDs {
		userIDs[id] = struct{}{}
	}
	var queuedIDs []uint
	if err := db.Table("plan_entitlements AS pe").
		Joins("JOIN users AS u ON u.id = pe.user_id AND u.deleted_at IS NULL").
		Where("pe.status = ? AND (u.plan_id IS NULL OR u.plan_expires_at IS NULL OR u.plan_expires_at <= ?)",
			model.PlanEntitlementQueued, now).
		Distinct("pe.user_id").Pluck("pe.user_id", &queuedIDs).Error; err != nil {
		return err
	}
	for _, id := range queuedIDs {
		userIDs[id] = struct{}{}
	}
	for id := range userIDs {
		if err := db.Transaction(func(tx *gorm.DB) error {
			var user model.User
			if err := tx.First(&user, id).Error; err != nil {
				return err
			}
			return reconcileUserPlanTx(tx, &user, now)
		}); err != nil {
			return err
		}
	}
	return nil
}

func reconcileUserPlanTx(tx *gorm.DB, user *model.User, now time.Time) error {
	if user.PlanID != nil && user.PlanExpiresAt != nil && user.PlanExpiresAt.After(now) {
		return nil
	}
	if err := tx.Model(&model.PlanEntitlement{}).
		Where("user_id = ? AND status IN ? AND expires_at IS NOT NULL AND expires_at <= ?", user.ID,
			[]string{model.PlanEntitlementActive, model.PlanEntitlementExtended}, now).
		Update("status", model.PlanEntitlementExpired).Error; err != nil {
		return err
	}

	startsAt := now
	if user.PlanExpiresAt != nil {
		startsAt = *user.PlanExpiresAt
	}
	for {
		var next model.PlanEntitlement
		err := tx.Where("user_id = ? AND status = ?", user.ID, model.PlanEntitlementQueued).
			Order("id ASC").First(&next).Error
		if err == gorm.ErrRecordNotFound {
			break
		}
		if err != nil {
			return err
		}
		expiresAt := startsAt.AddDate(0, 0, next.DurationDays)
		status := model.PlanEntitlementActive
		if !expiresAt.After(now) {
			status = model.PlanEntitlementExpired
		}
		claim := tx.Model(&model.PlanEntitlement{}).
			Where("id = ? AND status = ?", next.ID, model.PlanEntitlementQueued).
			Updates(map[string]interface{}{
				"status":       status,
				"starts_at":    startsAt,
				"expires_at":   expiresAt,
				"activated_at": now,
			})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return nil
		}
		if status == model.PlanEntitlementExpired {
			startsAt = expiresAt
			continue
		}
		var plan model.Plan
		if err := tx.First(&plan, next.PlanID).Error; err != nil {
			return err
		}
		return applyPlanToUser(tx, user, &plan, expiresAt)
	}

	updates := map[string]interface{}{
		"plan_id":         nil,
		"plan_expires_at": nil,
	}
	if user.GroupSource == "plan" {
		updates["group_id"] = nil
		updates["group_source"] = "expired_plan"
	}
	if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		return err
	}

	user.PlanID = nil
	user.PlanExpiresAt = nil
	user.Plan = nil
	if user.GroupSource == "plan" {
		user.GroupID = nil
		user.GroupSource = "expired_plan"
		user.Group = nil
	}
	return nil
}

func applyPlanToUser(tx *gorm.DB, user *model.User, plan *model.Plan, expiresAt time.Time) error {
	updates := map[string]interface{}{
		"plan_id":         plan.ID,
		"plan_expires_at": expiresAt,
	}
	if plan.GroupID != nil {
		updates["group_id"] = *plan.GroupID
		updates["group_source"] = "plan"
	} else if user.GroupSource == "plan" || user.GroupSource == "expired_plan" {
		updates["group_id"] = nil
		updates["group_source"] = ""
	}
	if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		return err
	}
	user.PlanID = &plan.ID
	user.PlanExpiresAt = &expiresAt
	user.Plan = plan
	if plan.GroupID != nil {
		user.GroupID = plan.GroupID
		user.GroupSource = "plan"
	} else if user.GroupSource == "plan" || user.GroupSource == "expired_plan" {
		user.GroupID = nil
		user.GroupSource = ""
	}
	return nil
}

func CanAccessServer(db *gorm.DB, user *model.User, serverID uint) (bool, error) {
	if err := ExpireUserPlan(db, user, time.Now()); err != nil {
		return false, err
	}
	if IsPlanGroupExpired(user) {
		return false, nil
	}
	if user.GroupID == nil {
		return true, nil
	}

	var count int64
	err := db.Table("user_group_servers AS ugs").
		Joins("JOIN servers AS s ON s.id = ugs.server_id AND s.deleted_at IS NULL").
		Where("ugs.user_group_id = ? AND ugs.server_id = ? AND s.plugin_auth_enabled = ?", *user.GroupID, serverID, true).
		Count(&count).Error
	return count > 0, err
}

func IsPlanGroupExpired(user *model.User) bool {
	return user != nil && user.GroupID == nil && user.GroupSource == "expired_plan"
}

func EffectiveBandwidth(db *gorm.DB, user *model.User) int64 {
	if user.BandwidthLimit > 0 {
		return user.BandwidthLimit
	}
	if user.Plan != nil && user.Plan.MaxBandwidth > 0 {
		return user.Plan.MaxBandwidth
	}
	if user.PlanID != nil {
		var plan model.Plan
		if err := db.First(&plan, *user.PlanID).Error; err == nil && plan.MaxBandwidth > 0 {
			return plan.MaxBandwidth
		}
	}

	var setting model.Setting
	if err := db.Where("key = ?", "free_max_bandwidth_mb").First(&setting).Error; err == nil {
		var mb float64
		if _, err := fmt.Sscanf(setting.Value, "%f", &mb); err == nil && mb > 0 {
			return int64(mb * 1024 * 1024)
		}
	}
	return defaultFreeBandwidth
}
