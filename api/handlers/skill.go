package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"

	"mooc-manus/internal/applications/dtos"
	appSvc "mooc-manus/internal/applications/services"
	"mooc-manus/internal/domains/models"
	"mooc-manus/pkg/skillerr"

	"github.com/gin-gonic/gin"
)

type SkillHandler struct {
	skillAppSvc           appSvc.SkillApplicationService
	skillProviderAppSvc   appSvc.SkillProviderApplicationService
	skillVersionAppSvc    appSvc.SkillVersionApplicationService
	skillImportTaskAppSvc appSvc.SkillImportTaskApplicationService
}

func NewSkillHandler(
	skillAppSvc appSvc.SkillApplicationService,
	skillProviderAppSvc appSvc.SkillProviderApplicationService,
	skillVersionAppSvc appSvc.SkillVersionApplicationService,
	skillImportTaskAppSvc appSvc.SkillImportTaskApplicationService,
) *SkillHandler {
	return &SkillHandler{
		skillAppSvc:           skillAppSvc,
		skillProviderAppSvc:   skillProviderAppSvc,
		skillVersionAppSvc:    skillVersionAppSvc,
		skillImportTaskAppSvc: skillImportTaskAppSvc,
	}
}

// writeError 将 skillerr 哨兵错误映射到 HTTP 状态码
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, skillerr.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, skillerr.ErrDuplicate):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, skillerr.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// parseMultipartSkillFiles 从 form-data 中解析通用 Skill 文件结构
func parseMultipartSkillFiles(c *gin.Context) ([]models.SkillFileStructure, error) {
	raw := c.PostForm("skillFiles")
	if raw == "" {
		return nil, fmt.Errorf("skillFiles is required: %w", skillerr.ErrInvalidInput)
	}
	var fs []models.SkillFileStructure
	if err := json.Unmarshal([]byte(raw), &fs); err != nil {
		return nil, fmt.Errorf("skillFiles format invalid: %w", skillerr.ErrInvalidInput)
	}
	return fs, nil
}

// ============================================================
// Skill 子域 9 个接口
// ============================================================

// DraftSave POST /api/v1/skill/draft/save  form-data
func (h *SkillHandler) DraftSave(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(dtos.SkillImportMaxFileSize); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parse form failed: " + err.Error()})
		return
	}
	skillFiles, err := parseMultipartSkillFiles(c)
	if err != nil {
		writeError(c, err)
		return
	}
	// SkillName / Description 由 Service 层从 SKILL.md frontmatter 解析，不再从表单读取
	req := &dtos.SkillDraftSaveRequest{
		SkillID:    c.PostForm("skillId"),
		Icon:       c.PostForm("icon"),
		ImageURL:   c.PostForm("imageUrl"),
		SkillFiles: skillFiles,
	}
	var files []*multipart.FileHeader
	if c.Request.MultipartForm != nil {
		files = c.Request.MultipartForm.File["files"]
	}
	info, err := h.skillAppSvc.DraftSave(req, files)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// Publish POST /api/v1/skill/publish  form-data
func (h *SkillHandler) Publish(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(dtos.SkillImportMaxFileSize); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parse form failed: " + err.Error()})
		return
	}
	skillFiles, err := parseMultipartSkillFiles(c)
	if err != nil {
		writeError(c, err)
		return
	}
	// SkillName / Description 由 Service 层从 SKILL.md frontmatter 解析，不再从表单读取
	req := &dtos.SkillPublishRequest{
		SkillID:            c.PostForm("skillId"),
		ProviderID:         c.PostForm("providerId"),
		VersionDescription: c.PostForm("versionDescription"),
		Icon:               c.PostForm("icon"),
		ImageURL:           c.PostForm("imageUrl"),
		SkillFiles:         skillFiles,
	}
	var files []*multipart.FileHeader
	if c.Request.MultipartForm != nil {
		files = c.Request.MultipartForm.File["files"]
	}
	info, err := h.skillAppSvc.Publish(req, files)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// Update POST /api/v1/skill/update
func (h *SkillHandler) Update(c *gin.Context) {
	var req dtos.SkillUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillAppSvc.Update(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// Delete POST /api/v1/skill/delete
func (h *SkillHandler) Delete(c *gin.Context) {
	var req dtos.SkillDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.skillAppSvc.Delete(req); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// List POST /api/v1/skill/list
func (h *SkillHandler) List(c *gin.Context) {
	var req dtos.SkillListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, err := h.skillAppSvc.List(&req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// ListAll POST /api/v1/skill/listAll
func (h *SkillHandler) ListAll(c *gin.Context) {
	var req dtos.SkillListAllRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	list, err := h.skillAppSvc.ListAll(&req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

// Detail POST /api/v1/skill/detail
func (h *SkillHandler) Detail(c *gin.Context) {
	var req dtos.SkillDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillAppSvc.Detail(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// WithVersion POST /api/v1/skill/with/version
func (h *SkillHandler) WithVersion(c *gin.Context) {
	list, err := h.skillAppSvc.WithVersion()
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

// FileDownload GET /api/v1/skill/file/download?fileKey=xxx
func (h *SkillHandler) FileDownload(c *gin.Context) {
	var q dtos.SkillFileDownloadQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rc, filename, err := h.skillAppSvc.FileDownload(q)
	if err != nil {
		writeError(c, err)
		return
	}
	defer rc.Close()
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))
	c.DataFromReader(http.StatusOK, -1, "application/octet-stream", rc, nil)
}

// ============================================================
// Version 子域 8 个接口
// ============================================================

func (h *SkillHandler) VersionCreate(c *gin.Context) {
	var req dtos.VersionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillVersionAppSvc.Create(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *SkillHandler) VersionValidate(c *gin.Context) {
	var req dtos.VersionValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillVersionAppSvc.Validate(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *SkillHandler) VersionDelete(c *gin.Context) {
	var req dtos.VersionDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.skillVersionAppSvc.Delete(req); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *SkillHandler) VersionList(c *gin.Context) {
	var req dtos.VersionListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	list, err := h.skillVersionAppSvc.List(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *SkillHandler) VersionDetail(c *gin.Context) {
	var req dtos.VersionDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillVersionAppSvc.Detail(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *SkillHandler) VersionLatest(c *gin.Context) {
	var req dtos.VersionLatestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillVersionAppSvc.Latest(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *SkillHandler) VersionRollback(c *gin.Context) {
	var req dtos.VersionRollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillVersionAppSvc.Rollback(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// VersionExport POST /api/v1/skill/version/export  流式 ZIP 下载
func (h *SkillHandler) VersionExport(c *gin.Context) {
	var req dtos.VersionExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	skillName, err := h.skillVersionAppSvc.Export(req, c.Writer)
	if err != nil {
		// ZIP 头未写出前可设置错误码；若已写出则流损坏，依赖客户端检测
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename := fmt.Sprintf("%s-v%s.zip", skillName, req.Version)
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))
}

// ============================================================
// Provider 子域 7 个接口
// ============================================================

func (h *SkillHandler) ProviderImportGit(c *gin.Context) {
	var req dtos.ImportGitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillProviderAppSvc.ImportGit(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// ProviderImportZip POST /api/v1/skill/provider/import/zip  form-data  → 立即返回 taskId
func (h *SkillHandler) ProviderImportZip(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required: " + err.Error()})
		return
	}
	taskID, err := h.skillImportTaskAppSvc.ImportFromZip(file)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"taskId": taskID})
}

func (h *SkillHandler) ProviderImportZipLegacy(c *gin.Context) {
	var req dtos.ImportZipLegacyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillProviderAppSvc.ImportZipLegacy(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *SkillHandler) ProviderSync(c *gin.Context) {
	var req dtos.ProviderSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillProviderAppSvc.Sync(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *SkillHandler) ProviderDelete(c *gin.Context) {
	var req dtos.ProviderDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.skillProviderAppSvc.Delete(req); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *SkillHandler) ProviderList(c *gin.Context) {
	var req dtos.ProviderListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	list, err := h.skillProviderAppSvc.List(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *SkillHandler) ProviderDetail(c *gin.Context) {
	var req dtos.ProviderDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := h.skillProviderAppSvc.Detail(req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// ============================================================
// ImportTask 子域 3 个接口
// ============================================================

// ImportTaskDetail POST /api/v1/skill/provider/import/task/detail  SSE 长连接
func (h *SkillHandler) ImportTaskDetail(c *gin.Context) {
	var req dtos.ImportTaskDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}
	emitter := func(event dtos.SkillImportEventData) {
		data, _ := json.Marshal(event)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}
	if err := h.skillImportTaskAppSvc.SubscribeTask(req.TaskID, emitter); err != nil {
		writeError(c, err)
		return
	}
	<-c.Request.Context().Done()
}

func (h *SkillHandler) ImportTaskList(c *gin.Context) {
	list, err := h.skillImportTaskAppSvc.List()
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *SkillHandler) ImportTaskDelete(c *gin.Context) {
	var req dtos.ImportTaskDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.skillImportTaskAppSvc.Delete(req); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
