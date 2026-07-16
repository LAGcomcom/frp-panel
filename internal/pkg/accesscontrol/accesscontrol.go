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
	if user.PlanID == nil || user.PlanExpiresAt == nil || user.PlanExpiresAt.After(now) {
		return nil
	}

	updates := map[string]interface{}{
		"plan_id":         nil,
		"plan_expires_at": nil,
	}
	if user.GroupSource == "plan" {
		updates["group_id"] = nil
		updates["group_source"] = "expired_plan"
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
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
