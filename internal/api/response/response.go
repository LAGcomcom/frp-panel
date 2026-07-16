package response

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	invalidRequestMessage = "提交内容有误，请检查后重试"
	internalErrorMessage  = "服务暂时无法处理请求，请稍后重试"
)

var technicalErrorFragments = []string{
	"json:", "cannot unmarshal", "invalid character", "unexpected end of json",
	"validation for", "failed on the '", "struct field", "type float", "type int",
	"strconv.", "sql:", "database error", "gorm", ".go:", "stack trace", "panic:",
}

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type PageData struct {
	List  interface{} `json:"list"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

func Error(c *gin.Context, httpCode int, code int, message string) {
	c.JSON(httpCode, Response{
		Code:    code,
		Message: message,
	})
}

func BadRequest(c *gin.Context, message string) {
	userMessage := friendlyBadRequest(message)
	if userMessage != strings.TrimSpace(message) {
		log.Printf("[API] invalid request path=%s: %s", c.Request.URL.Path, message)
	}
	Error(c, http.StatusBadRequest, 400, userMessage)
}

func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, 401, message)
}

func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, 403, message)
}

func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, 404, message)
}

func InternalError(c *gin.Context, message string) {
	log.Printf("[API] internal error path=%s: %s", c.Request.URL.Path, message)
	Error(c, http.StatusInternalServerError, 500, internalErrorMessage)
}

func friendlyBadRequest(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" || len(trimmed) > 240 || strings.ContainsAny(trimmed, "\r\n") {
		return invalidRequestMessage
	}
	lower := strings.ToLower(trimmed)
	for _, fragment := range technicalErrorFragments {
		if strings.Contains(lower, fragment) {
			return invalidRequestMessage
		}
	}
	return trimmed
}

func Page(c *gin.Context, list interface{}, total int64, page, size int) {
	Success(c, PageData{
		List:  list,
		Total: total,
		Page:  page,
		Size:  size,
	})
}
