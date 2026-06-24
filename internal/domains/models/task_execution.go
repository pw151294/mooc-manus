package models

import (
	"encoding/json"
	"errors"
	"time"

	infra "mooc-manus/internal/infra/models"
)

// TaskExecutionDO 异步任务执行实体（共用聚合根）
type TaskExecutionDO struct {
	TaskID     string
	AppID      string // SkillAppID
	AppType    string // SkillImportAppType
	Status     string // TaskStatusProcessing / TaskStatusCompleted / TaskStatusFailed
	Stage      string // TaskStageUpload / Extract / Validate / Register / Completed
	Progress   int    // 0 - 100
	ExtInfo    TaskExecutionExtInfo
	Creator    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ArchivedAt *time.Time
}

// ImportLog 导入任务日志条目值对象
type ImportLog struct {
	Time    time.Time `json:"time"`    // ISO 8601
	Level   string    `json:"level"`   // INFO / SUCCESS / WARNING / ERROR / DEBUG
	Message string    `json:"message"`
}

// TaskExecutionExtInfo task_execution 表的 ext_info JSON 结构
type TaskExecutionExtInfo struct {
	Logs         []ImportLog `json:"logs,omitempty"`
	SkillCount   int         `json:"skillCount,omitempty"`
	ProviderID   string      `json:"providerId,omitempty"`
	ErrorMessage string      `json:"errorMessage,omitempty"`
	FileName     string      `json:"fileName,omitempty"`
	FileSize     int64       `json:"fileSize,omitempty"`
}

// ValidateCanComplete 仅当当前状态为 PROCESSING 时允许标记完成
func (t *TaskExecutionDO) ValidateCanComplete() error {
	if t.Status != TaskStatusProcessing {
		return errors.New("task is not in PROCESSING state, cannot complete")
	}
	return nil
}

// MarkAsCompleted 状态置为 COMPLETED + stage=COMPLETED + progress=100 + 写入归档时间
func (t *TaskExecutionDO) MarkAsCompleted() {
	now := time.Now()
	t.Status = TaskStatusCompleted
	t.Stage = TaskStageCompleted
	t.Progress = 100
	t.ArchivedAt = &now
}

// MarkAsFailed 状态置为 FAILED 并写入归档时间
func (t *TaskExecutionDO) MarkAsFailed(errMsg string) {
	now := time.Now()
	t.Status = TaskStatusFailed
	t.ArchivedAt = &now
	if errMsg != "" {
		t.ExtInfo.ErrorMessage = errMsg
	}
}

// UpdateProgressInfo 更新阶段与进度
func (t *TaskExecutionDO) UpdateProgressInfo(stage string, progress int) {
	t.Stage = stage
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	t.Progress = progress
}

// AppendLog 追加一条日志（保留全部历史，让 SSE 订阅者可重放）
func (t *TaskExecutionDO) AppendLog(level, message string) {
	t.ExtInfo.Logs = append(t.ExtInfo.Logs, ImportLog{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	})
}

func ConvertTaskExecutionDO2PO(do TaskExecutionDO) infra.TaskExecutionPO {
	if do.Status == "" {
		do.Status = TaskStatusProcessing
	}
	extBytes, _ := json.Marshal(do.ExtInfo)
	return infra.TaskExecutionPO{
		TaskID:     do.TaskID,
		AppID:      do.AppID,
		AppType:    do.AppType,
		Status:     do.Status,
		Stage:      do.Stage,
		Progress:   do.Progress,
		ExtInfo:    string(extBytes),
		Creator:    do.Creator,
		ArchivedAt: do.ArchivedAt,
	}
}

func ConvertTaskExecutionPO2DO(po infra.TaskExecutionPO) TaskExecutionDO {
	var ext TaskExecutionExtInfo
	if po.ExtInfo != "" {
		_ = json.Unmarshal([]byte(po.ExtInfo), &ext)
	}
	return TaskExecutionDO{
		TaskID:     po.TaskID,
		AppID:      po.AppID,
		AppType:    po.AppType,
		Status:     po.Status,
		Stage:      po.Stage,
		Progress:   po.Progress,
		ExtInfo:    ext,
		Creator:    po.Creator,
		CreatedAt:  po.CreatedAt,
		UpdatedAt:  po.UpdatedAt,
		ArchivedAt: po.ArchivedAt,
	}
}
