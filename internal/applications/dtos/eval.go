package dtos

import "time"

// ===== 用例 (Case) =====

// CaseCreateRequest 创建用例请求。
// InitScript 可空；TaskPrompt / VerifyScript 必填。
type CaseCreateRequest struct {
	Name         string   `json:"name" binding:"required"`
	Description  string   `json:"description"`
	InitScript   string   `json:"init_script"`
	TaskPrompt   string   `json:"task_prompt" binding:"required"`
	VerifyScript string   `json:"verify_script" binding:"required"`
	Tags         []string `json:"tags"`
}

// CaseUpdateRequest 用例部分字段更新。
// 指针字段为 nil 表示不更新；ID 从 URL 注入。
type CaseUpdateRequest struct {
	ID           string    `json:"-"`
	Name         *string   `json:"name"`
	Description  *string   `json:"description"`
	InitScript   *string   `json:"init_script"`
	TaskPrompt   *string   `json:"task_prompt"`
	VerifyScript *string   `json:"verify_script"`
	Tags         *[]string `json:"tags"`
}

// CaseView 用例视图，返回给前端。
type CaseView struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	InitScript   string    `json:"init_script"`
	TaskPrompt   string    `json:"task_prompt"`
	VerifyScript string    `json:"verify_script"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ListCasesQuery 列表查询参数。
type ListCasesQuery struct {
	NameLike string   `form:"name_like"`
	Tags     []string `form:"tags"`
	Page     int      `form:"page,default=1"`
	Size     int      `form:"size,default=20"`
}

// UploadContentResp 上传脚本 / prompt 的返回体。
type UploadContentResp struct {
	Content string `json:"content"`
	Size    int    `json:"size"`
}

// ===== 任务 (Task) =====

// TaskCreateRequest 创建评测任务。
// CaseIDs × AgentConfigIDs 是 M × N 组合。
type TaskCreateRequest struct {
	Name           string   `json:"name" binding:"required"`
	CaseIDs        []string `json:"case_ids" binding:"required,min=1"`
	AgentConfigIDs []string `json:"agent_config_ids" binding:"required,min=1"`
}

// TaskView 任务视图。
type TaskView struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	CaseIDs        []string   `json:"case_ids"`
	AgentConfigIDs []string   `json:"agent_config_ids"`
	Status         string     `json:"status"`
	TotalCount     int        `json:"total_count"`
	SucceededCount int        `json:"succeeded_count"`
	FailedCount    int        `json:"failed_count"`
	RunningCount   int        `json:"running_count"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// ListTasksQuery 任务列表查询参数。
type ListTasksQuery struct {
	Status string `form:"status"`
	Page   int    `form:"page,default=1"`
	Size   int    `form:"size,default=20"`
}

// RetryTaskResp 重试任务返回体（重试的实例数）。
type RetryTaskResp struct {
	RetriedCount int `json:"retried_count"`
}

// ===== 实例 (RunInstance) =====

// InstanceView 单实例视图；Result 字段仅在存在时填充。
type InstanceView struct {
	ID             string      `json:"id"`
	TaskID         string      `json:"task_id"`
	CaseID         string      `json:"case_id"`
	Status         string      `json:"status"`
	Attempt        int         `json:"attempt"`
	ConversationID string      `json:"conversation_id"`
	MessageID      string      `json:"message_id"`
	TraceID        string      `json:"trace_id"`
	QueuedAt       *time.Time  `json:"queued_at,omitempty"`
	StartedAt      *time.Time  `json:"started_at,omitempty"`
	FinishedAt     *time.Time  `json:"finished_at,omitempty"`
	HeartbeatAt    *time.Time  `json:"heartbeat_at,omitempty"`
	DeadlineAt     *time.Time  `json:"deadline_at,omitempty"`
	WorkerID       string      `json:"worker_id"`
	ErrorMessage   string      `json:"error_message"`
	Result         *ResultView `json:"result,omitempty"`
}

// ResultView 实例执行结果视图。
type ResultView struct {
	InstanceID       string    `json:"instance_id"`
	Passed           bool      `json:"passed"`
	VerifyExitCode   int       `json:"verify_exit_code"`
	VerifyStdout     string    `json:"verify_stdout"`
	VerifyStderr     string    `json:"verify_stderr"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	TotalTokens      int64     `json:"total_tokens"`
	AgentLatencyMs   int64     `json:"agent_latency_ms"`
	ErrorLog         string    `json:"error_log"`
	FinishedAt       time.Time `json:"finished_at"`
}

// ListInstancesQuery 实例列表参数（任务 ID 走 URL path）。
type ListInstancesQuery struct {
	Status string `form:"status"`
	Page   int    `form:"page,default=1"`
	Size   int    `form:"size,default=50"`
}

// AgentConfigView 仅暴露评测所需的最少字段；apiKey 等敏感字段不外泄。
type AgentConfigView struct {
	ID        string `json:"id"`
	ModelName string `json:"model_name"`
	Provider  string `json:"provider"`
}

// ListPage 通用分页返回；Go 1.18+ 支持泛型序列化。
type ListPage[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}
