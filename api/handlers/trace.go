package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/applications/services"
)

// TraceHandler 处理智能体链路追踪相关的 HTTP 请求
type TraceHandler struct {
	svc services.TraceApplicationService
}

// NewTraceHandler 构造 TraceHandler
func NewTraceHandler(svc services.TraceApplicationService) *TraceHandler {
	return &TraceHandler{svc: svc}
}

// GetDetail 处理 GET /api/trace/:trace_id
func (h *TraceHandler) GetDetail(c *gin.Context) {
	traceID := c.Param("trace_id")
	if traceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_PARAM", "message": "trace_id required"})
		return
	}
	dto, err := h.svc.GetTraceDetail(c.Request.Context(), traceID)
	if err != nil {
		if errors.Is(err, services.ErrTraceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "TRACE_NOT_FOUND"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, dto)
}

// List 处理 GET /api/traces
func (h *TraceHandler) List(c *gin.Context) {
	var req dtos.TraceListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_PARAM", "message": err.Error()})
		return
	}
	dto, err := h.svc.ListTraces(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, dto)
}
