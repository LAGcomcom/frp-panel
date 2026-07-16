package model

import (
	"time"

	"gorm.io/gorm"
)

type UserGroup struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"uniqueIndex;size:100;not null" json:"name"`
	Description string         `gorm:"size:500" json:"description"`
	Servers     []Server       `gorm:"many2many:user_group_servers;" json:"servers,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (UserGroup) TableName() string {
	return "user_groups"
}

type UserGroupServer struct {
	UserGroupID uint `gorm:"primaryKey" json:"user_group_id"`
	ServerID    uint `gorm:"primaryKey" json:"server_id"`
}

func (UserGroupServer) TableName() string {
	return "user_group_servers"
}
