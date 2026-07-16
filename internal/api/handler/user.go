package handler

import (
	"fmt"
	"math/rand"
	"net/smtp"
	"sync"
	"time"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/accesscontrol"
	"github.com/frp-panel/frp-panel/internal/pkg/hash"
	"github.com/frp-panel/frp-panel/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Verification code store
type codeEntry struct {
	code      string
	expiresAt time.Time
}

var (
	codeStore = make(map[string]codeEntry)
	codeMu    sync.RWMutex
)

func init() {
	// Cleanup expired codes every minute
	go func() {
		for {
			time.Sleep(time.Minute)
			codeMu.Lock()
			for email, entry := range codeStore {
				if time.Now().After(entry.expiresAt) {
					delete(codeStore, email)
				}
			}
			codeMu.Unlock()
		}
	}()
}

type UserHandler struct {
	db  *gorm.DB
	jwt *jwt.JWTManager
}

func NewUserHandler(db *gorm.DB, jwtManager *jwt.JWTManager) *UserHandler {
	return &UserHandler{db: db, jwt: jwtManager}
}

type RegisterRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=6"`
	InviteCode string `json:"invite_code"`
	Code       string `json:"code"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *UserHandler) Register(c *gin.Context) {
	// Check if registration is enabled
	var regSetting model.Setting
	if err := h.db.Where("key = ?", "registration_enabled").First(&regSetting).Error; err == nil && regSetting.Value == "false" {
		response.Forbidden(c, "注册已关闭")
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Check if email exists
	var count int64
	h.db.Model(&model.User{}).Where("email = ?", req.Email).Count(&count)
	if count > 0 {
		response.BadRequest(c, "email already exists")
		return
	}

	// Check verification code if enabled
	var verifySetting model.Setting
	if err := h.db.Where("key = ?", "email_verification_enabled").First(&verifySetting).Error; err == nil && verifySetting.Value == "true" {
		if req.Code == "" {
			response.BadRequest(c, "请输入验证码")
			return
		}
		codeMu.RLock()
		entry, exists := codeStore[req.Email]
		codeMu.RUnlock()
		if !exists || time.Now().After(entry.expiresAt) || entry.code != req.Code {
			response.BadRequest(c, "验证码错误或已过期")
			return
		}
		// Delete used code
		codeMu.Lock()
		delete(codeStore, req.Email)
		codeMu.Unlock()
	}

	hashedPassword, err := hash.BcryptHash(req.Password)
	if err != nil {
		response.InternalError(c, "failed to hash password")
		return
	}

	user := model.User{
		Email:    req.Email,
		Password: hashedPassword,
		Role:     "user",
		Status:   "active",
		APIKey:   hash.GenerateAPIKey(),
	}

	// Handle invite code
	if req.InviteCode != "" {
		var inviter model.User
		if err := h.db.Where("invite_code = ?", req.InviteCode).First(&inviter).Error; err == nil {
			user.InvitedBy = &inviter.ID
		}
	}

	// Generate unique invite code
	user.InviteCode = hash.RandomString(8)

	if err := h.db.Create(&user).Error; err != nil {
		response.InternalError(c, "failed to create user")
		return
	}

	// Invite rebate is applied on every paid order (see processReferralRebate)

	token, err := h.jwt.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		response.InternalError(c, "failed to generate token")
		return
	}

	response.SuccessWithMessage(c, "registered successfully", gin.H{
		"token": token,
		"user":  user,
	})
}

// SendVerificationCode sends a verification code to the specified email
func (h *UserHandler) SendVerificationCode(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请输入有效的邮箱地址")
		return
	}

	// Check if email verification is enabled
	var verifySetting model.Setting
	if err := h.db.Where("key = ?", "email_verification_enabled").First(&verifySetting).Error; err != nil || verifySetting.Value != "true" {
		response.BadRequest(c, "邮箱验证未启用")
		return
	}

	// Check if email already registered
	var count int64
	h.db.Model(&model.User{}).Where("email = ?", req.Email).Count(&count)
	if count > 0 {
		response.BadRequest(c, "该邮箱已注册")
		return
	}

	// Rate limit: 1 code per 60 seconds
	codeMu.RLock()
	entry, exists := codeStore[req.Email]
	codeMu.RUnlock()
	if exists && time.Since(entry.expiresAt.Add(-10*time.Minute)) < 60*time.Second {
		response.BadRequest(c, "请等待 60 秒后重试")
		return
	}

	// Generate 6-digit code
	code := fmt.Sprintf("%06d", rand.Intn(1000000))

	// Store code (valid for 10 minutes)
	codeMu.Lock()
	codeStore[req.Email] = codeEntry{
		code:      code,
		expiresAt: time.Now().Add(10 * time.Minute),
	}
	codeMu.Unlock()

	// Send email
	settings := h.getSettingsMap()
	host := settings["smtp_host"]
	port := settings["smtp_port"]
	user := settings["smtp_user"]
	password := settings["smtp_password"]
	from := settings["smtp_from"]

	if host == "" || user == "" || password == "" {
		response.InternalError(c, "邮件服务未配置")
		return
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	auth := smtp.PlainAuth("", user, password, host)

	subject := "FRP Panel - 验证码"
	body := fmt.Sprintf("您的验证码是：%s，10 分钟内有效。如非本人操作，请忽略此邮件。", code)
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", from, req.Email, subject, body))

	if err := smtp.SendMail(addr, auth, from, []string{req.Email}, msg); err != nil {
		response.InternalError(c, "验证码发送失败: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "验证码已发送", nil)
}

func (h *UserHandler) Login(c *gin.Context) {
	// Check if login is enabled
	var loginSetting model.Setting
	if err := h.db.Where("key = ?", "login_enabled").First(&loginSetting).Error; err == nil && loginSetting.Value == "false" {
		response.Forbidden(c, "登录已关闭")
		return
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		response.Unauthorized(c, "invalid email or password")
		return
	}

	if !hash.BcryptCheck(req.Password, user.Password) {
		response.Unauthorized(c, "invalid email or password")
		return
	}

	if user.Status == "banned" {
		response.Forbidden(c, "account is banned")
		return
	}

	token, err := h.jwt.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		response.InternalError(c, "failed to generate token")
		return
	}

	response.Success(c, gin.H{
		"token": token,
		"user":  user,
	})
}

func (h *UserHandler) AdminLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		response.Unauthorized(c, "invalid email or password")
		return
	}

	if !hash.BcryptCheck(req.Password, user.Password) {
		response.Unauthorized(c, "invalid email or password")
		return
	}

	if user.Role != "admin" && user.Role != "super_admin" {
		response.Forbidden(c, "admin access required")
		return
	}

	if user.Status == "banned" {
		response.Forbidden(c, "account is banned")
		return
	}

	token, err := h.jwt.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		response.InternalError(c, "failed to generate token")
		return
	}

	response.Success(c, gin.H{
		"token": token,
		"user":  user,
	})
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var user model.User
	if err := h.db.Preload("Plan").Preload("Group").First(&user, userID).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	// Check if plan has expired
	if err := accesscontrol.ExpireUserPlan(h.db, &user, time.Now()); err != nil {
		response.InternalError(c, "failed to expire user plan")
		return
	}

	response.Success(c, user)
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req struct {
		Email string `json:"email" binding:"omitempty,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Email != "" {
		updates["email"] = req.Email
	}

	if err := h.db.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		response.InternalError(c, "failed to update profile")
		return
	}

	response.SuccessWithMessage(c, "profile updated", nil)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var user model.User
	if err := h.db.First(&user, userID).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	if !hash.BcryptCheck(req.OldPassword, user.Password) {
		response.BadRequest(c, "invalid old password")
		return
	}

	newHash, err := hash.BcryptHash(req.NewPassword)
	if err != nil {
		response.InternalError(c, "failed to hash password")
		return
	}

	h.db.Model(&user).Update("password", newHash)
	response.SuccessWithMessage(c, "password changed", nil)
}

// GetInviteStats returns invite statistics for the current user (level 1 + level 2)
func (h *UserHandler) GetInviteStats(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var user model.User
	h.db.First(&user, userID)

	// Level 1: direct referrals
	var level1Users []model.User
	h.db.Where("invited_by = ?", userID).Select("id", "email", "created_at").Order("id desc").Find(&level1Users)
	level1Count := len(level1Users)

	// Collect level 1 user IDs and build email map
	level1IDs := make([]uint, 0, level1Count)
	level1EmailMap := make(map[uint]string, level1Count)
	for _, u := range level1Users {
		level1IDs = append(level1IDs, u.ID)
		level1EmailMap[u.ID] = u.Email
	}

	// Level 2: referrals of referrals
	type level2Entry struct {
		ID              uint      `json:"id"`
		Email           string    `json:"email"`
		CreatedAt       time.Time `json:"created_at"`
		ReferredByEmail string    `json:"referred_by_email"`
	}
	var level2Users []level2Entry
	var level2Count int64
	if len(level1IDs) > 0 {
		var l2Raw []model.User
		h.db.Where("invited_by IN ?", level1IDs).Select("id", "email", "created_at", "invited_by").Order("id desc").Find(&l2Raw)
		level2Count = int64(len(l2Raw))
		for _, u := range l2Raw {
			email := ""
			if u.InvitedBy != nil {
				email = level1EmailMap[*u.InvitedBy]
			}
			level2Users = append(level2Users, level2Entry{
				ID:              u.ID,
				Email:           u.Email,
				CreatedAt:       u.CreatedAt,
				ReferredByEmail: email,
			})
		}
	}

	// Read rebate percentages from settings
	settings := h.getSettingsMap()
	level1Pct := 10.0
	level2Pct := 5.0
	if v := settings["invite_rebate_level1_percent"]; v != "" {
		fmt.Sscanf(v, "%f", &level1Pct)
	}
	if v := settings["invite_rebate_level2_percent"]; v != "" {
		fmt.Sscanf(v, "%f", &level2Pct)
	}

	// Sum total rebate earned
	var totalRebate float64
	var rebates []model.ReferralRebate
	h.db.Where("level1_user_id = ? OR level2_user_id = ?", userID, userID).Find(&rebates)
	for _, r := range rebates {
		if r.Level1UserID == userID {
			totalRebate += r.Level1Amount
		}
		if r.Level2UserID != nil && *r.Level2UserID == userID {
			totalRebate += r.Level2Amount
		}
	}

	response.Success(c, gin.H{
		"invite_code":         user.InviteCode,
		"level1_count":        level1Count,
		"level1_users":        level1Users,
		"level2_count":        level2Count,
		"level2_users":        level2Users,
		"level1_rebate_pct":   level1Pct,
		"level2_rebate_pct":   level2Pct,
		"total_rebate_earned": totalRebate,
	})
}

// RegenerateAPIKey generates a new API key for the current user
func (h *UserHandler) RegenerateAPIKey(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	newKey := hash.GenerateAPIKey()
	if err := h.db.Model(&model.User{}).Where("id = ?", userID).Update("api_key", newKey).Error; err != nil {
		response.InternalError(c, "failed to regenerate API key")
		return
	}

	response.Success(c, gin.H{
		"api_key": newKey,
	})
}

// GetAPIKey returns the current user's API key
func (h *UserHandler) GetAPIKey(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var user model.User
	h.db.Select("api_key").First(&user, userID)

	response.Success(c, gin.H{
		"api_key": user.APIKey,
	})
}

// Admin: List all users
func (h *UserHandler) ListUsers(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	size := c.DefaultQuery("size", "20")
	keyword := c.DefaultQuery("keyword", "")
	status := c.DefaultQuery("status", "")

	var users []model.User
	var total int64

	query := h.db.Model(&model.User{})
	if keyword != "" {
		query = query.Where("email LIKE ?", "%"+keyword+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)

	offset := (parseInt(page) - 1) * parseInt(size)
	query.Offset(offset).Limit(parseInt(size)).Order("id desc").Preload("Plan").Preload("Group").Find(&users)

	response.Page(c, users, total, parseInt(page), parseInt(size))
}

// Admin: Get user detail
func (h *UserHandler) GetUser(c *gin.Context) {
	id := c.Param("id")

	var user model.User
	if err := h.db.Preload("Plan").Preload("Group").First(&user, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	// Get proxy count
	var proxyCount int64
	h.db.Model(&model.Proxy{}).Where("user_id = ?", user.ID).Count(&proxyCount)

	response.Success(c, gin.H{
		"user":        user,
		"proxy_count": proxyCount,
	})
}

// Admin: Update user
func (h *UserHandler) AdminUpdateUser(c *gin.Context) {
	id := c.Param("id")
	target, ok := h.requireManageableUser(c, id, false)
	if !ok {
		return
	}

	var req struct {
		Role           string  `json:"role"`
		Balance        float64 `json:"balance"`
		Status         string  `json:"status"`
		PlanID         *uint   `json:"plan_id"`
		GroupID        *uint   `json:"group_id"`
		ClearGroup     bool    `json:"clear_group"`
		BandwidthLimit *int64  `json:"bandwidth_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Role != "" {
		actorRole, _ := c.Get("role")
		if actorRole != "super_admin" {
			response.Forbidden(c, "只有超级管理员可以修改用户角色")
			return
		}
		if req.Role != "user" && req.Role != "admin" && req.Role != "super_admin" {
			response.BadRequest(c, "用户角色无效")
			return
		}
		if actorID, exists := c.Get("user_id"); exists && actorID == target.ID && req.Role != "super_admin" {
			response.BadRequest(c, "不能降低自己的超级管理员权限")
			return
		}
		updates["role"] = req.Role
	}
	if req.Balance != 0 {
		updates["balance"] = req.Balance
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.PlanID != nil {
		response.BadRequest(c, "请使用套餐分配接口调整用户套餐")
		return
	}
	if req.ClearGroup {
		updates["group_id"] = nil
		updates["group_source"] = ""
	} else if req.GroupID != nil {
		var group model.UserGroup
		if err := h.db.First(&group, *req.GroupID).Error; err != nil {
			response.BadRequest(c, "用户组不存在")
			return
		}
		updates["group_id"] = *req.GroupID
		updates["group_source"] = "manual"
	}
	if req.BandwidthLimit != nil {
		if *req.BandwidthLimit < 0 {
			response.BadRequest(c, "带宽限制不能小于 0")
			return
		}
		updates["bandwidth_limit"] = *req.BandwidthLimit
	}

	if err := h.db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.InternalError(c, "failed to update user")
		return
	}

	response.SuccessWithMessage(c, "user updated", nil)
}

func (h *UserHandler) AdminAssignPlan(c *gin.Context) {
	target, ok := h.requireManageableUser(c, c.Param("id"), false)
	if !ok {
		return
	}
	var req struct {
		PlanID       uint   `json:"plan_id" binding:"required"`
		DurationType string `json:"duration_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var plan model.Plan
	if err := h.db.First(&plan, req.PlanID).Error; err != nil {
		response.BadRequest(c, "套餐不存在")
		return
	}
	days := planDurationDays(&plan, req.DurationType)
	if days <= 0 {
		response.BadRequest(c, "套餐周期无效")
		return
	}
	expiresAt := time.Now().In(beijingTZ).AddDate(0, 0, days)
	updates := map[string]interface{}{
		"plan_id":         plan.ID,
		"plan_expires_at": expiresAt,
	}
	if plan.GroupID != nil {
		updates["group_id"] = *plan.GroupID
		updates["group_source"] = "plan"
	} else if target.GroupSource == "plan" || target.GroupSource == "expired_plan" {
		updates["group_id"] = nil
		updates["group_source"] = ""
	}
	if err := h.db.Model(target).Updates(updates).Error; err != nil {
		response.InternalError(c, "failed to assign plan")
		return
	}
	response.SuccessWithMessage(c, "套餐已分配", gin.H{"expires_at": expiresAt})
}

func (h *UserHandler) AdminClearPlan(c *gin.Context) {
	target, ok := h.requireManageableUser(c, c.Param("id"), false)
	if !ok {
		return
	}
	updates := map[string]interface{}{
		"plan_id":         nil,
		"plan_expires_at": nil,
	}
	if target.GroupSource == "plan" || target.GroupSource == "expired_plan" {
		updates["group_id"] = nil
		updates["group_source"] = ""
	}
	if err := h.db.Model(target).Updates(updates).Error; err != nil {
		response.InternalError(c, "failed to clear plan")
		return
	}
	response.SuccessWithMessage(c, "套餐已清除", nil)
}

// Admin: Ban user
func (h *UserHandler) BanUser(c *gin.Context) {
	id := c.Param("id")
	if _, ok := h.requireManageableUser(c, id, true); !ok {
		return
	}
	if err := h.db.Model(&model.User{}).Where("id = ?", id).Update("status", "banned").Error; err != nil {
		response.InternalError(c, "failed to ban user")
		return
	}
	response.SuccessWithMessage(c, "user banned", nil)
}

// Admin: Unban user
func (h *UserHandler) UnbanUser(c *gin.Context) {
	id := c.Param("id")
	if _, ok := h.requireManageableUser(c, id, false); !ok {
		return
	}
	if err := h.db.Model(&model.User{}).Where("id = ?", id).Update("status", "active").Error; err != nil {
		response.InternalError(c, "failed to unban user")
		return
	}
	response.SuccessWithMessage(c, "user unbanned", nil)
}

func (h *UserHandler) requireManageableUser(c *gin.Context, id string, forbidSelf bool) (*model.User, bool) {
	var target model.User
	if err := h.db.First(&target, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return nil, false
	}
	if !authorizeManageableUser(c, &target) {
		return nil, false
	}
	if forbidSelf {
		if actorID, exists := c.Get("user_id"); exists && actorID == target.ID {
			response.BadRequest(c, "不能封禁当前登录账号")
			return nil, false
		}
	}
	return &target, true
}

func authorizeManageableUser(c *gin.Context, target *model.User) bool {
	actorRole, _ := c.Get("role")
	if actorRole != "super_admin" && target.Role != "user" {
		response.Forbidden(c, "普通管理员只能管理普通用户")
		return false
	}
	return true
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if n == 0 {
		return 1
	}
	return n
}

func (h *UserHandler) getSettingsMap() map[string]string {
	var settings []model.Setting
	h.db.Find(&settings)
	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result
}
