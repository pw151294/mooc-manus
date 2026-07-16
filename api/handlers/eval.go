package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/applications/services"
	evalsvc "mooc-manus/internal/domains/services/evaluation"
)

// EvalHandler 评测模块入口。
// 所有 HTTP 请求 → Application Service → Domain Service。
type EvalHandler struct {
	svc services.EvaluationApplicationService
}

// NewEvalHandler 装配 EvalHandler。
func NewEvalHandler(svc services.EvaluationApplicationService) *EvalHandler {
	return &EvalHandler{svc: svc}
}

// ============ Case ============

// UploadContent POST /api/eval/cases/upload-content
func (h *EvalHandler) UploadContent(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.UploadContent(c.Request.Context(), file)
	if err != nil {
		if errors.Is(err, services.ErrUploadTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, services.ErrUploadNotUTF8) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// CreateCase POST /api/eval/cases
func (h *EvalHandler) CreateCase(c *gin.Context) {
	var req dtos.CaseCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	view, err := h.svc.CreateCase(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

// UpdateCase PUT /api/eval/cases/:id
func (h *EvalHandler) UpdateCase(c *gin.Context) {
	var req dtos.CaseUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ID = c.Param("id")
	view, err := h.svc.UpdateCase(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

// ListCases GET /api/eval/cases
func (h *EvalHandler) ListCases(c *gin.Context) {
	var q dtos.ListCasesQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, err := h.svc.ListCases(c.Request.Context(), &q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

// GetCase GET /api/eval/cases/:id
func (h *EvalHandler) GetCase(c *gin.Context) {
	view, err := h.svc.GetCase(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

// DeleteCase DELETE /api/eval/cases/:id
// 若用例正被 PENDING / RUNNING 任务引用，返回 409。
func (h *EvalHandler) DeleteCase(c *gin.Context) {
	err := h.svc.DeleteCase(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, evalsvc.ErrCaseHasRunningReferences) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ============ Task ============

// CreateTask POST /api/eval/tasks
func (h *EvalHandler) CreateTask(c *gin.Context) {
	var req dtos.TaskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	view, err := h.svc.CreateTask(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

// ListTasks GET /api/eval/tasks
func (h *EvalHandler) ListTasks(c *gin.Context) {
	var q dtos.ListTasksQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, err := h.svc.ListTasks(c.Request.Context(), &q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

// GetTask GET /api/eval/tasks/:id
func (h *EvalHandler) GetTask(c *gin.Context) {
	view, err := h.svc.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

// RetryTask POST /api/eval/tasks/:id/retry
func (h *EvalHandler) RetryTask(c *gin.Context) {
	resp, err := h.svc.RetryTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// DeleteTask DELETE /api/eval/tasks/:id
func (h *EvalHandler) DeleteTask(c *gin.Context) {
	err := h.svc.DeleteTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ============ Instance ============

// ListInstances GET /api/eval/tasks/:id/instances
func (h *EvalHandler) ListInstances(c *gin.Context) {
	var q dtos.ListInstancesQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, err := h.svc.ListInstances(c.Request.Context(), c.Param("id"), &q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

// GetInstance GET /api/eval/instances/:id
func (h *EvalHandler) GetInstance(c *gin.Context) {
	view, err := h.svc.GetInstance(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

// GetInstanceTrace GET /api/eval/instances/:id/trace
// 仅返回 trace_id，前端拿到后跳转 /api/trace/:id 详情页。
func (h *EvalHandler) GetInstanceTrace(c *gin.Context) {
	tid, err := h.svc.GetInstanceTraceID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"trace_id": tid})
}

// RetryInstance POST /api/eval/instances/:id/retry
// 仅允许 FAILED / TIMEOUT 实例重试；其他状态返回 409。
func (h *EvalHandler) RetryInstance(c *gin.Context) {
	err := h.svc.RetryInstance(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, evalsvc.ErrInstanceNotRetryable) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DeleteInstance DELETE /api/eval/instances/:id
// 仅允许终态或 PENDING 状态删除；进行中的实例返回 409。
func (h *EvalHandler) DeleteInstance(c *gin.Context) {
	err := h.svc.DeleteInstance(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, evalsvc.ErrInstanceNotDeletable) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ListAgentConfigs GET /api/eval/agent-configs
func (h *EvalHandler) ListAgentConfigs(c *gin.Context) {
	list, err := h.svc.ListAgentConfigs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}
