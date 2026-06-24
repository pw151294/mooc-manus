package dtos

import (
	"time"

	"mooc-manus/internal/domains/models"
)

// SkillImportTaskInfo 导入任务列表项
type SkillImportTaskInfo struct {
	TaskID     string    `json:"taskId"`
	FileName   string    `json:"fileName,omitempty"`
	FileSize   int64     `json:"fileSize,omitempty"`
	Status     string    `json:"status"`
	Stage      string    `json:"stage,omitempty"`
	Progress   int       `json:"progress"`
	SkillCount int       `json:"skillCount,omitempty"`
	ProviderID string    `json:"providerId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// SkillImportEventData SSE 推送的事件载荷
type SkillImportEventData struct {
	TaskID       string             `json:"taskId"`
	Status       string             `json:"status"`
	Stage        string             `json:"stage,omitempty"`
	Progress     int                `json:"progress"`
	Logs         []models.ImportLog `json:"logs,omitempty"`
	SkillCount   int                `json:"skillCount,omitempty"`
	ProviderID   string             `json:"providerId,omitempty"`
	ErrorMessage string             `json:"errorMessage,omitempty"`
}

// ImportTaskDetailRequest 任务订阅请求（SSE）
type ImportTaskDetailRequest struct {
	TaskID string `json:"taskId" binding:"required"`
}

// ImportTaskListRequest 任务列表请求（当前主体下所有导入任务）
type ImportTaskListRequest struct{}

// ImportTaskDeleteRequest 批量删除任务请求
type ImportTaskDeleteRequest struct {
	TaskIDs []string `json:"taskIds" binding:"required"`
}

// ConvertTaskExecutionDO2Info 将任务 DO 转为列表项 DTO
func ConvertTaskExecutionDO2Info(do models.TaskExecutionDO) SkillImportTaskInfo {
	return SkillImportTaskInfo{
		TaskID:     do.TaskID,
		FileName:   do.ExtInfo.FileName,
		FileSize:   do.ExtInfo.FileSize,
		Status:     do.Status,
		Stage:      do.Stage,
		Progress:   do.Progress,
		SkillCount: do.ExtInfo.SkillCount,
		ProviderID: do.ExtInfo.ProviderID,
		CreatedAt:  do.CreatedAt,
	}
}

// ConvertTaskExecutionDO2Event 将任务 DO 转为 SSE 事件 DTO
func ConvertTaskExecutionDO2Event(do models.TaskExecutionDO) SkillImportEventData {
	return SkillImportEventData{
		TaskID:       do.TaskID,
		Status:       do.Status,
		Stage:        do.Stage,
		Progress:     do.Progress,
		Logs:         do.ExtInfo.Logs,
		SkillCount:   do.ExtInfo.SkillCount,
		ProviderID:   do.ExtInfo.ProviderID,
		ErrorMessage: do.ExtInfo.ErrorMessage,
	}
}
