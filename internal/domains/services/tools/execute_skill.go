package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/infra/repositories"
	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

const (
	ExecuteSkillProviderID   = "builtin-skill-provider" // 与 loadSkill 共享 provider
	ExecuteSkillProviderName = "skill-provider"
	ExecuteSkillProviderType = "SKILL"
	ExecuteSkillFunctionID   = "builtin-execute-skill"
	ExecuteSkillFunctionName = "executeSkill"
	ExecuteSkillFunctionDesc = "执行指定Skill的脚本，传入参数并获取执行结果。【前置条件】调用此工具前必须先调用loadSkill加载Skill文档，了解其参数要求和使用方式，禁止未经loadSkill直接调用executeSkill。"
)

// ExecuteSkillTool executeSkill 内置工具
type ExecuteSkillTool struct {
	BaseTool
	skillRepo   repositories.SkillRepository
	versionRepo repositories.SkillVersionRepository
	executor    SkillExecutor // 执行器抽象
	skillRefs   []agents.SkillRef
}

// NewExecuteSkillTool 创建 executeSkill 工具实例
func NewExecuteSkillTool(
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	executor SkillExecutor,
	skillRefs []agents.SkillRef,
) Tool {
	return &ExecuteSkillTool{
		skillRepo:   skillRepo,
		versionRepo: versionRepo,
		executor:    executor,
		skillRefs:   skillRefs,
	}
}

// Init 初始化工具，构造 ToolFunctionDO
func (t *ExecuteSkillTool) Init() error {
	funcDO := models.ToolFunctionDO{
		FunctionID:   ExecuteSkillFunctionID,
		ProviderID:   ExecuteSkillProviderID,
		FunctionName: ExecuteSkillFunctionName,
		FunctionDesc: ExecuteSkillFunctionDesc,
		Schema: models.ToolSchema{
			Name:        ExecuteSkillFunctionName,
			Description: ExecuteSkillFunctionDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skillName": map[string]any{
						"type":        "string",
						"description": "要执行的Skill名称",
					},
					"bash": map[string]any{
						"type":        "string",
						"description": "Skill执行的Bash命令",
					},
				},
				"required": []string{"skillName", "bash"},
			},
		},
	}

	// 填充 BaseTool
	t.BaseTool.providerId = ExecuteSkillProviderID
	t.BaseTool.providerName = ExecuteSkillProviderName
	t.BaseTool.providerType = ExecuteSkillProviderType
	t.BaseTool.functions = []models.ToolFunctionDO{funcDO}

	return nil
}

// Invoke 工具调用入口
func (t *ExecuteSkillTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	// 步骤1：参数解析
	var params struct {
		SkillName string `json:"skillName"`
		Bash      string `json:"bash"`
	}
	if err := json.Unmarshal([]byte(funcArgs), &params); err != nil {
		logger.Error("unmarshal executeSkill args failed", zap.Error(err), zap.String("func_args", funcArgs))
		return models.ToolCallResult{Success: false, Message: "Error: 参数解析失败"}
	}

	// 步骤2：参数校验
	if params.SkillName == "" {
		return models.ToolCallResult{Success: false, Message: "Error: skillName parameter is required"}
	}
	if params.Bash == "" {
		return models.ToolCallResult{Success: false, Message: "Error: bash parameter is required"}
	}

	// 步骤3：从 skillRefs 中按 skillName 查找（利用新增的 SkillName 字段）
	var targetRef *agents.SkillRef
	for i := range t.skillRefs {
		if t.skillRefs[i].SkillName == params.SkillName {
			targetRef = &t.skillRefs[i]
			break
		}
	}
	if targetRef == nil {
		availList := t.getAvailableSkillNames()
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Skill not found: %s. Available skills: %s", params.SkillName, availList),
		}
	}

	// 步骤4：查询 SkillVersion
	versionPO, exists, err := t.versionRepo.GetBySkillIDAndVersion(targetRef.SkillID, targetRef.Version)
	if err != nil {
		logger.Error("executeSkill query skill version failed", zap.Error(err),
			zap.String("skill_id", targetRef.SkillID), zap.String("version", targetRef.Version))
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Failed to execute skill: %s. Reason: %v", params.SkillName, err),
		}
	}
	if !exists {
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Skill version not found: %s", targetRef.Version),
		}
	}
	// 此处不需要使用 versionPO，但为了保持与 loadSkill 的一致性仍执行查询
	_ = versionPO

	// 步骤5：构建执行上下文（最小集）
	execCtx := SkillExecutionContext{
		SkillID:   targetRef.SkillID,
		Version:   targetRef.Version,
		MessageID: "", // 当前占位实现不使用，但保留字段
	}

	// 步骤6：调用 SkillExecutor（同步执行）
	results, err := t.executor.Execute(execCtx, params.Bash)
	if err != nil {
		// 错误处理：占位实现会返回"未实现"错误
		logger.Error("executeSkill execution failed", zap.Error(err),
			zap.String("skill_name", params.SkillName), zap.String("bash", params.Bash))
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: %s", err.Error()),
		}
	}

	// 步骤7：聚合结果（当前占位实现返回 nil results，不会执行此分支）
	// 未来 Docker 实现后，此处聚合 stdout/stderr/outputFiles
	var stdout, stderr strings.Builder
	var outputFiles []string
	hasError := false

	for _, res := range results {
		if res.Stdout != "" {
			stdout.WriteString(res.Stdout)
			stdout.WriteString("\n")
		}
		if res.Stderr != "" {
			stderr.WriteString("Error: ")
			stderr.WriteString(res.Stderr)
			stderr.WriteString("\n")
			hasError = true
		}
		if res.Status != "" {
			stdout.WriteString(fmt.Sprintf("Status: %s\n", res.Status))
		}
		outputFiles = append(outputFiles, res.OutputFiles...)
	}

	// 步骤8：拼装响应（成功/失败分支）
	if hasError {
		message := stdout.String() + stderr.String() + "\n\nExecution failed. Please analyze the error above, fix the issue (e.g., install missing dependencies, correct the script), and retry by calling executeSkill again with the corrected bash command."
		return models.ToolCallResult{Success: false, Message: message}
	}

	if len(outputFiles) > 0 {
		stdout.WriteString("\nGenerated files (host paths):\n")
		for _, fp := range outputFiles {
			stdout.WriteString(fmt.Sprintf("- %s\n", fp))
		}
		stdout.WriteString("\nYou can use the artifactoryFileSave tool to upload these files to artifact storage.")
	}

	message := stdout.String()
	if message == "" {
		message = "Execution completed"
	}

	return models.ToolCallResult{Success: true, Data: message}
}

// getAvailableSkillNames 辅助方法：返回可用 Skill 名称列表
func (t *ExecuteSkillTool) getAvailableSkillNames() string {
	if len(t.skillRefs) == 0 {
		return "[]"
	}

	names := make([]string, 0, len(t.skillRefs))
	for _, ref := range t.skillRefs {
		if ref.SkillName != "" {
			names = append(names, ref.SkillName)
		}
	}

	return fmt.Sprintf("[%s]", strings.Join(names, ", "))
}
