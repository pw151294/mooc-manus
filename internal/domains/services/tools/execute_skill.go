package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mooc-manus/config"
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/services/tools/error_recovery"
	"mooc-manus/internal/infra/external/file_storage"
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
	storage     file_storage.FileStorage // T1 用：从 storage 下载 skill 文件到挂载目录
	executor    SkillExecutor            // 执行器抽象
	skillRefs   []agents.SkillRef
	messageId   string // T0=T1 关键：用于隔离不同消息的容器与工作目录
}

// NewExecuteSkillTool 创建 executeSkill 工具实例
// messageId 由 application 层从 sse.StartChat 生成并透传，用于隔离不同消息的执行环境
func NewExecuteSkillTool(
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	storage file_storage.FileStorage,
	executor SkillExecutor,
	skillRefs []agents.SkillRef,
	messageId string,
) Tool {
	return &ExecuteSkillTool{
		skillRepo:   skillRepo,
		versionRepo: versionRepo,
		storage:     storage,
		executor:    executor,
		skillRefs:   skillRefs,
		messageId:   messageId,
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

	// 步骤 2.5:内置错误恢复 Skill 无可执行脚本,拒绝 executeSkill 调用
	if error_recovery.IsBuiltInSkill(params.SkillName) {
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: %s 无可执行脚本,请按 SKILL.md 指导继续常规工具调用(fileRead/fileWrite/fileEdit/bashExec),不要 executeSkill 本内置 Skill", params.SkillName),
		}
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
	versionDO := models.ConvertSkillVersionPO2DO(versionPO)

	// 步骤4.5：准备 skill 工作目录（T1）
	// 同 messageId 内多次调用同一 skill 时，目录已存在且非空则跳过下载（D3' 缓存策略）
	if err := t.prepareSkillWorkDir(targetRef, versionDO); err != nil {
		logger.Error("executeSkill prepare work dir failed", zap.Error(err),
			zap.String("skill_name", params.SkillName),
			zap.String("message_id", t.messageId))
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Failed to prepare skill files: %s. Reason: %v", params.SkillName, err),
		}
	}

	// 步骤5：构建执行上下文（最小集）
	execCtx := SkillExecutionContext{
		SkillID:   targetRef.SkillID,
		Version:   targetRef.Version,
		MessageID: t.messageId, // T0：来自 application 层透传，与 SSE 流绑定
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

// skillWorkDir 计算单 skill 工作目录：${baseDir}/skills/${messageId}/${SkillID}-${Version}
func (t *ExecuteSkillTool) skillWorkDir(skillID, version string) string {
	return filepath.Join(
		config.Cfg.Skill.BaseDir,
		"skills",
		t.messageId,
		fmt.Sprintf("%s-%s", skillID, version),
	)
}

// prepareSkillWorkDir 把 SkillVersion 的文件下载到工作目录
// 同 messageId 内目录已存在且非空时跳过（D3' 缓存策略）
func (t *ExecuteSkillTool) prepareSkillWorkDir(ref *agents.SkillRef, versionDO models.SkillVersionDO) error {
	workDir := t.skillWorkDir(ref.SkillID, ref.Version)

	// 同 messageId 缓存检查
	if entries, err := os.ReadDir(workDir); err == nil && len(entries) > 0 {
		logger.Info("[skill-exec] workDir already prepared, skip download",
			zap.String("work_dir", workDir),
			zap.Int("file_count", len(entries)),
		)
		return nil
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("mkdir workDir failed: %w", err)
	}

	logger.Info("[skill-exec] start downloading skill files",
		zap.String("work_dir", workDir),
		zap.String("skill_id", ref.SkillID),
		zap.String("version", ref.Version),
		zap.Int("file_count", len(versionDO.SkillFiles)),
	)

	for _, file := range versionDO.SkillFiles {
		target, err := safeJoin(workDir, file.Path)
		if err != nil {
			return fmt.Errorf("invalid file path %q: %w", file.Path, err)
		}
		if err := t.downloadSkillFile(file.FileKey, target); err != nil {
			return fmt.Errorf("download %s failed: %w", file.Path, err)
		}
	}

	logger.Info("[skill-exec] skill files downloaded",
		zap.String("work_dir", workDir),
	)
	return nil
}

// downloadSkillFile 从 storage 下载单个文件到 target 路径（自动创建父目录）
func (t *ExecuteSkillTool) downloadSkillFile(fileKey, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("mkdir parent failed: %w", err)
	}

	rc, err := t.storage.GetObject(dtos.SkillBucketName, fileKey)
	if err != nil {
		return fmt.Errorf("storage.GetObject failed: %w", err)
	}
	defer rc.Close()

	out, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}
	return nil
}

// safeJoin 把 relPath 安全地拼到 base 上，拒绝指向 base 之外的路径
// 防御点：相对路径含 ../ 或绝对路径都会被规范化后检查前缀
func safeJoin(base, relPath string) (string, error) {
	cleanBase := filepath.Clean(base)
	full := filepath.Join(cleanBase, relPath)
	cleanFull := filepath.Clean(full)
	// 末尾加分隔符避免 /a/foo/bar 被认为是 /a/foo 的前缀绕过
	if !strings.HasPrefix(cleanFull+string(filepath.Separator), cleanBase+string(filepath.Separator)) &&
		cleanFull != cleanBase {
		return "", fmt.Errorf("path escapes base: relPath=%q resolved=%q", relPath, cleanFull)
	}
	return cleanFull, nil
}
