package middleware

import (
	"strings"

	"github.com/frp-panel/frp-panel/internal/api/response"
	"github.com/frp-panel/frp-panel/internal/model"
	"github.com/frp-panel/frp-panel/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func JWTAuth(jwtManager *jwt.JWTManager, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := ""

		// Try Authorization header first
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == authHeader {
				tokenString = ""
			}
		}

		// Fallback to query parameter (for WebSocket connections)
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			response.Unauthorized(c, "missing authorization")
			c.Abort()
			return
		}

		claims, err := jwtManager.ParseToken(tokenString)
		if err != nil {
			response.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		var user model.User
		if err := db.Select("id", "email", "role", "status").First(&user, claims.UserID).Error; err != nil {
			response.Unauthorized(c, "account no longer exists")
			c.Abort()
			return
		}
		if user.Status != "active" {
			response.Forbidden(c, "account is not active")
			c.Abort()
			return
		}

		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
		c.Set("role", user.Role)
		c.Next()
	}
}

func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Unauthorized(c, "unauthorized")
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || (roleStr != "admin" && roleStr != "super_admin") {
			response.Forbidden(c, "admin access required")
			c.Abort()
			return
		}

		c.Next()
	}
}

func SuperAdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Unauthorized(c, "unauthorized")
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || roleStr != "super_admin" {
			response.Forbidden(c, "super admin access required")
			c.Abort()
			return
		}

		c.Next()
	}
}
