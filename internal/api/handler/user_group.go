package handler

import (
	"fmt"
	"strings"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/service/deployer"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserGroupHandler struct {
	db       *gorm.DB
	deployer *deployer.Deployer
}

func NewUserGroupHandler(db *gorm.DB, deployers ...*deployer.Deployer) *UserGroupHandler {
	var d *deployer.Deployer
	if len(deployers) > 0 {
		d = deployers[0]
	}
	return &UserGroupHandler{db: db, deployer: d}
}

func (h *UserGroupHandler) serverReady(server *model.Server) (bool, string) {
	if h.deployer == nil {
		if server.PluginAuthEnabled {
			server.PluginAuthStatus = "ready"
			server.PluginAuthMessage = "安全模式"
			return true, ""
		}
		server.PluginAuthStatus = "redeploy_required"
		server.PluginAuthMessage = "节点尚未启用安全鉴权，请先重新部署该节点"
		return false, server.PluginAuthMessage
	}
	ok, reason := h.deployer.PluginEndpointMatches("", server)
	if ok {
		server.PluginAuthStatus = "ready"
		server.PluginAuthMessage = "安全模式"
		return true, ""
	}
	server.PluginAuthStatus = "redeploy_required"
	server.PluginAuthMessage = reason
	return false, reason
}

type userGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	ServerIDs   []uint `json:"server_ids"`
}

func (h *UserGroupHandler) List(c *gin.Context) {
	var groups []model.UserGroup
	if err := h.db.Preload("Servers").Order("id asc").Find(&groups).Error; err != nil {
		response.InternalError(c, "failed to list user groups")
		return
	}
	for groupIndex := range groups {
		for serverIndex := range groups[groupIndex].Servers {
			h.serverReady(&groups[groupIndex].Servers[serverIndex])
		}
	}
	response.Success(c, groups)
}

func (h *UserGroupHandler) Create(c *gin.Context) {
	var req userGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	group := model.UserGroup{Name: req.Name, Description: req.Description}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		return h.replaceGroupServers(tx, &group, req.ServerIDs)
	}); err != nil {
		response.BadRequest(c, friendlyGroupError(err))
		return
	}
	h.db.Preload("Servers").First(&group, group.ID)
	response.SuccessWithMessage(c, "用户组已创建", group)
}

func (h *UserGroupHandler) Update(c *gin.Context) {
	var group model.UserGroup
	if err := h.db.First(&group, c.Param("id")).Error; err != nil {
		response.NotFound(c, "用户组不存在")
		return
	}
	var req userGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&group).Updates(map[string]interface{}{
			"name": req.Name, "description": req.Description,
		}).Error; err != nil {
			return err
		}
		return h.replaceGroupServers(tx, &group, req.ServerIDs)
	}); err != nil {
		response.BadRequest(c, friendlyGroupError(err))
		return
	}
	h.db.Preload("Servers").First(&group, group.ID)
	response.SuccessWithMessage(c, "用户组已更新", group)
}

func (h *UserGroupHandler) Delete(c *gin.Context) {
	var group model.UserGroup
	if err := h.db.First(&group, c.Param("id")).Error; err != nil {
		response.NotFound(c, "用户组不存在")
		return
	}
	var userCount, planCount int64
	h.db.Model(&model.User{}).Where("group_id = ?", group.ID).Count(&userCount)
	h.db.Model(&model.Plan{}).Where("group_id = ?", group.ID).Count(&planCount)
	if userCount > 0 || planCount > 0 {
		response.BadRequest(c, "该用户组仍被用户或套餐使用，请先解除关联")
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_group_id = ?", group.ID).Delete(&model.UserGroupServer{}).Error; err != nil {
			return err
		}
		return tx.Delete(&group).Error
	}); err != nil {
		response.InternalError(c, "failed to delete user group")
		return
	}
	response.SuccessWithMessage(c, "用户组已删除", nil)
}

func (h *UserGroupHandler) replaceGroupServers(tx *gorm.DB, group *model.UserGroup, serverIDs []uint) error {
	servers := make([]model.Server, 0, len(serverIDs))
	if len(serverIDs) > 0 {
		if err := tx.Where("id IN ?", serverIDs).Find(&servers).Error; err != nil {
			return err
		}
		if len(servers) != len(serverIDs) {
			return fmt.Errorf("包含不存在的节点")
		}
		for _, server := range servers {
			if ok, reason := h.serverReady(&server); !ok {
				return fmt.Errorf("节点 %s 不可加入用户组：%s", server.Name, reason)
			}
		}
	}
	return tx.Model(group).Association("Servers").Replace(servers)
}

func friendlyGroupError(err error) string {
	if err == nil {
		return "用户组保存失败"
	}
	message := err.Error()
	if strings.Contains(strings.ToLower(message), "unique") && strings.Contains(message, "user_groups.name") {
		return "用户组名称已存在"
	}
	if strings.Contains(message, "节点") || strings.Contains(message, "不存在") {
		return message
	}
	return "用户组保存失败"
}
