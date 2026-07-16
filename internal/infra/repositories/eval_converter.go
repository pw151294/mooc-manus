package repositories

import (
	"encoding/json"
	"mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/models"

	"gorm.io/datatypes"
)

// Case 转换
func caseToDO(po *models.EvalCasePO) *evaluation.Case {
	if po == nil {
		return nil
	}
	var tags []string
	if po.Tags != nil {
		_ = json.Unmarshal(po.Tags, &tags)
	}
	return &evaluation.Case{
		ID:           po.ID,
		Name:         po.Name,
		Description:  po.Description,
		InitScript:   po.InitScript,
		TaskPrompt:   po.TaskPrompt,
		VerifyScript: po.VerifyScript,
		Tags:         tags,
		CreatedAt:    po.CreatedAt,
		UpdatedAt:    po.UpdatedAt,
	}
}

func caseToPO(do *evaluation.Case) *models.EvalCasePO {
	if do == nil {
		return nil
	}
	tagsJSON, _ := json.Marshal(do.Tags)
	return &models.EvalCasePO{
		ID:           do.ID,
		Name:         do.Name,
		Description:  do.Description,
		InitScript:   do.InitScript,
		TaskPrompt:   do.TaskPrompt,
		VerifyScript: do.VerifyScript,
		Tags:         datatypes.JSON(tagsJSON),
		CreatedAt:    do.CreatedAt,
		UpdatedAt:    do.UpdatedAt,
	}
}

// Task 转换
func taskToDO(po *models.EvalTaskPO) *evaluation.Task {
	if po == nil {
		return nil
	}
	var caseIDs, agentConfigIDs []string
	if po.CaseIDs != nil {
		_ = json.Unmarshal(po.CaseIDs, &caseIDs)
	}
	if po.AgentConfigIDs != nil {
		_ = json.Unmarshal(po.AgentConfigIDs, &agentConfigIDs)
	}
	return &evaluation.Task{
		ID:             po.ID,
		Name:           po.Name,
		CaseIDs:        caseIDs,
		AgentConfigIDs: agentConfigIDs,
		Status:         evaluation.TaskStatus(po.Status),
		TotalCount:     po.TotalCount,
		SucceededCount: po.SucceededCount,
		FailedCount:    po.FailedCount,
		RunningCount:   po.RunningCount,
		CreatedAt:      po.CreatedAt,
		StartedAt:      po.StartedAt,
		FinishedAt:     po.FinishedAt,
	}
}

func taskToPO(do *evaluation.Task) *models.EvalTaskPO {
	if do == nil {
		return nil
	}
	caseIDsJSON, _ := json.Marshal(do.CaseIDs)
	agentConfigIDsJSON, _ := json.Marshal(do.AgentConfigIDs)
	return &models.EvalTaskPO{
		ID:             do.ID,
		Name:           do.Name,
		CaseIDs:        datatypes.JSON(caseIDsJSON),
		AgentConfigIDs: datatypes.JSON(agentConfigIDsJSON),
		Status:         string(do.Status),
		TotalCount:     do.TotalCount,
		SucceededCount: do.SucceededCount,
		FailedCount:    do.FailedCount,
		RunningCount:   do.RunningCount,
		CreatedAt:      do.CreatedAt,
		StartedAt:      do.StartedAt,
		FinishedAt:     do.FinishedAt,
	}
}

// RunInstance 转换
func instanceToDO(po *models.EvalRunInstancePO) *evaluation.RunInstance {
	if po == nil {
		return nil
	}
	var caseSnapshot evaluation.Case
	if po.CaseSnapshot != nil {
		_ = json.Unmarshal(po.CaseSnapshot, &caseSnapshot)
	}
	return &evaluation.RunInstance{
		ID:                    po.ID,
		TaskID:                po.TaskID,
		CaseID:                po.CaseID,
		CaseSnapshot:          caseSnapshot,
		AgentConfigSnapshotID: po.AgentConfigSnapshotID,
		Status:                evaluation.InstanceStatus(po.Status),
		Attempt:               po.Attempt,
		ConversationID:        po.ConversationID,
		MessageID:             po.MessageID,
		TraceID:               po.TraceID,
		QueuedAt:              po.QueuedAt,
		StartedAt:             po.StartedAt,
		FinishedAt:            po.FinishedAt,
		HeartbeatAt:           po.HeartbeatAt,
		DeadlineAt:            po.DeadlineAt,
		WorkerID:              po.WorkerID,
		ErrorMessage:          po.ErrorMessage,
	}
}

func instanceToPO(do *evaluation.RunInstance) *models.EvalRunInstancePO {
	if do == nil {
		return nil
	}
	caseSnapshotJSON, _ := json.Marshal(do.CaseSnapshot)
	return &models.EvalRunInstancePO{
		ID:                    do.ID,
		TaskID:                do.TaskID,
		CaseID:                do.CaseID,
		CaseSnapshot:          datatypes.JSON(caseSnapshotJSON),
		AgentConfigSnapshotID: do.AgentConfigSnapshotID,
		Status:                string(do.Status),
		Attempt:               do.Attempt,
		ConversationID:        do.ConversationID,
		MessageID:             do.MessageID,
		TraceID:               do.TraceID,
		QueuedAt:              do.QueuedAt,
		StartedAt:             do.StartedAt,
		FinishedAt:            do.FinishedAt,
		HeartbeatAt:           do.HeartbeatAt,
		DeadlineAt:            do.DeadlineAt,
		WorkerID:              do.WorkerID,
		ErrorMessage:          do.ErrorMessage,
	}
}

// Result 转换
func resultToDO(po *models.EvalResultPO) *evaluation.Result {
	if po == nil {
		return nil
	}
	return &evaluation.Result{
		InstanceID:       po.InstanceID,
		Passed:           po.Passed,
		VerifyExitCode:   po.VerifyExitCode,
		VerifyStdout:     po.VerifyStdout,
		VerifyStderr:     po.VerifyStderr,
		PromptTokens:     po.PromptTokens,
		CompletionTokens: po.CompletionTokens,
		TotalTokens:      po.TotalTokens,
		AgentLatencyMs:   po.AgentLatencyMs,
		ErrorLog:         po.ErrorLog,
		FinishedAt:       po.FinishedAt,
	}
}

func resultToPO(do *evaluation.Result) *models.EvalResultPO {
	if do == nil {
		return nil
	}
	return &models.EvalResultPO{
		InstanceID:       do.InstanceID,
		Passed:           do.Passed,
		VerifyExitCode:   do.VerifyExitCode,
		VerifyStdout:     do.VerifyStdout,
		VerifyStderr:     do.VerifyStderr,
		PromptTokens:     do.PromptTokens,
		CompletionTokens: do.CompletionTokens,
		TotalTokens:      do.TotalTokens,
		AgentLatencyMs:   do.AgentLatencyMs,
		ErrorLog:         do.ErrorLog,
		FinishedAt:       do.FinishedAt,
	}
}

// AgentSnapshot 转换
func snapshotToDO(po *models.EvalAgentSnapshotPO) *evaluation.AgentSnapshot {
	if po == nil {
		return nil
	}
	var toolsConfig, mcpConfig, a2aConfig map[string]any
	if po.ToolsConfig != nil {
		_ = json.Unmarshal(po.ToolsConfig, &toolsConfig)
	}
	if po.MCPConfig != nil {
		_ = json.Unmarshal(po.MCPConfig, &mcpConfig)
	}
	if po.A2AConfig != nil {
		_ = json.Unmarshal(po.A2AConfig, &a2aConfig)
	}
	return &evaluation.AgentSnapshot{
		ID:                po.ID,
		SourceAppConfigID: po.SourceAppConfigID,
		Model:             po.Model,
		SystemPrompt:      po.SystemPrompt,
		ToolsConfig:       toolsConfig,
		MCPConfig:         mcpConfig,
		A2AConfig:         a2aConfig,
		CreatedAt:         po.CreatedAt,
	}
}

func snapshotToPO(do *evaluation.AgentSnapshot) *models.EvalAgentSnapshotPO {
	if do == nil {
		return nil
	}
	toolsConfigJSON, _ := json.Marshal(do.ToolsConfig)
	mcpConfigJSON, _ := json.Marshal(do.MCPConfig)
	a2aConfigJSON, _ := json.Marshal(do.A2AConfig)
	return &models.EvalAgentSnapshotPO{
		ID:                do.ID,
		SourceAppConfigID: do.SourceAppConfigID,
		Model:             do.Model,
		SystemPrompt:      do.SystemPrompt,
		ToolsConfig:       datatypes.JSON(toolsConfigJSON),
		MCPConfig:         datatypes.JSON(mcpConfigJSON),
		A2AConfig:         datatypes.JSON(a2aConfigJSON),
		CreatedAt:         do.CreatedAt,
	}
}
