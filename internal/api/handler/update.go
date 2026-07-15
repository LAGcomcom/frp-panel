package handler

import (
	"github.com/frp-panel/frp-panel/internal/api/response"
	updateservice "github.com/frp-panel/frp-panel/internal/service/update"
	"github.com/gin-gonic/gin"
	"os"
)

type UpdateHandler struct{ client *updateservice.Client }

func NewUpdateHandler(client *updateservice.Client) *UpdateHandler {
	return &UpdateHandler{client: client}
}

func (h *UpdateHandler) Download(c *gin.Context) {
	path, name, err := h.client.Download(c.Request.Context(), c.Param("version"))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	defer os.Remove(path)
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.File(path)
}

func (h *UpdateHandler) Check(c *gin.Context) {
	result, err := h.client.Check(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *UpdateHandler) LeaseStatus(c *gin.Context) {
	response.Success(c, gin.H{
		"lease":            h.client.LeaseStatus(),
		"private_updates":  h.client.FeatureAvailable("private_updates"),
		"cloud_alerts":     h.client.FeatureAvailable("cloud_alerts"),
		"advanced_reports": h.client.FeatureAvailable("advanced_reports"),
	})
}
