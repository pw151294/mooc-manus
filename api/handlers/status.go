package handlers

import (
	"net/http"

	"mooc-manus/internal/applications/services"
	"mooc-manus/internal/infra/external/health_checker"

	"github.com/gin-gonic/gin"
)

type StatusHandler struct {
	statusAppSvc services.StatusApplicationService
}

func NewStatusHandler(statusAppSvc services.StatusApplicationService) *StatusHandler {
	return &StatusHandler{statusAppSvc: statusAppSvc}
}

func (h *StatusHandler) Check(c *gin.Context) {
	status := h.statusAppSvc.Check()
	if status.Status == health_checker.UnHealthyStatus {
		// 返回503状态码 说明健康检查不通过
		c.JSON(http.StatusServiceUnavailable, status)
		return
	}
	//返回200状态码 标识健康检查通过
	c.JSON(http.StatusOK, status)
}
