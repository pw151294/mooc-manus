package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

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
	LoadSkillProviderID   = "builtin-skill-provider"
	LoadSkillProviderName = "skill-provider"
	LoadSkillProviderType = "SKILL"
	LoadSkillFunctionID   = "builtin-load-skill"
	LoadSkillFunctionName = "loadSkill"
	LoadSkillFunctionDesc = "加载指定Skill的SKILL.md文件内容，了解Skill的使用说明、参数定义和执行方式。【重要】在调用executeSkill之前，必须先调用此工具了解Skill的详细信息，不允许跳过此步骤直接执行。"
	LoadSkillDefaultFile  = "SKILL.md"
)

type LoadSkillTool struct {
	BaseTool
	skillRepo   repositories.SkillRepository
	versionRepo repositories.SkillVersionRepository
	storage     file_storage.FileStorage
	skillRefs   []agents.SkillRef
}

func NewLoadSkillTool(
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	storage file_storage.FileStorage,
	skillRefs []agents.SkillRef,
) Tool {
	return &LoadSkillTool{
		skillRepo:   skillRepo,
		versionRepo: versionRepo,
		storage:     storage,
		skillRefs:   skillRefs,
	}
}

func (t *LoadSkillTool) Init() error {
	// 构造内存 ToolFunctionDO
	funcDO := models.ToolFunctionDO{
		FunctionID:   LoadSkillFunctionID,
		ProviderID:   LoadSkillProviderID,
		FunctionName: LoadSkillFunctionName,
		FunctionDesc: LoadSkillFunctionDesc,
		Schema: models.ToolSchema{
			Name:        LoadSkillFunctionName,
			Description: LoadSkillFunctionDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skillName": map[string]any{
						"type":        "string",
						"description": "要加载的Skill名称",
					},
					"filepath": map[string]any{
						"type":        "string",
						"description": "要查看的SKILL的文件的相对路径,默认查看SKILL.md",
					},
				},
				"required": []string{"skillName"},
			},
		},
	}

	// 填充 BaseTool
	t.BaseTool.providerId = LoadSkillProviderID
	t.BaseTool.providerName = LoadSkillProviderName
	t.BaseTool.providerType = LoadSkillProviderType
	t.BaseTool.functions = []models.ToolFunctionDO{funcDO}

	return nil
}

func (t *LoadSkillTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	// 1. 参数解析
	var params struct {
		SkillName string `json:"skillName"`
		Filepath  string `json:"filepath"`
	}
	if err := json.Unmarshal([]byte(funcArgs), &params); err != nil {
		logger.Error("unmarshal loadSkill args failed", zap.Error(err), zap.String("func_args", funcArgs))
		return models.ToolCallResult{Success: false, Message: "Error: 参数解析失败"}
	}

	filepath := params.Filepath
	if filepath == "" {
		filepath = LoadSkillDefaultFile
	}

	// 2. skillName 空检查
	if params.SkillName == "" {
		return models.ToolCallResult{Success: false, Message: "Error: skillName parameter is required"}
	}

	// 2.5 内置错误恢复 Skill 短路:直接返回 embed 内容,不查 DB/OSS
	if error_recovery.IsBuiltInSkill(params.SkillName) {
		if filepath != LoadSkillDefaultFile {
			return models.ToolCallResult{
				Success: false,
				Message: fmt.Sprintf("Error: 内置 Skill %s 仅提供 SKILL.md,不支持额外文件 %s", params.SkillName, filepath),
			}
		}
		logger.Info("loadSkill served from embed builtin", zap.String("skill_name", params.SkillName))
		return models.ToolCallResult{Success: true, Data: error_recovery.SkillMD()}
	}

	// 3. 按 SkillName 查找 Skill
	skillPO, exists, err := t.skillRepo.GetByName(params.SkillName)
	if err != nil {
		logger.Error("loadSkill query skill by name failed", zap.Error(err), zap.String("skill_name", params.SkillName))
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Failed to load skill: %s. Reason: %v", params.SkillName, err),
		}
	}
	if !exists {
		availList := t.getAvailableSkillNames()
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Skill not found: %s. Available skills: %s", params.SkillName, availList),
		}
	}
	skillDO := models.ConvertSkillPO2DO(skillPO)

	// 4. 校验 skillDO.SkillID 是否在 skillRefs 里
	found := false
	var targetVersion string
	for _, ref := range t.skillRefs {
		if ref.SkillID == skillDO.SkillID {
			found = true
			targetVersion = ref.Version
			break
		}
	}
	if !found {
		availList := t.getAvailableSkillNames()
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Skill not found: %s. Available skills: %s", params.SkillName, availList),
		}
	}

	// 5. 按 (skillId, version) 查版本
	versionPO, exists, err := t.versionRepo.GetBySkillIDAndVersion(skillDO.SkillID, targetVersion)
	if err != nil {
		logger.Error("loadSkill query skill version failed", zap.Error(err),
			zap.String("skill_id", skillDO.SkillID), zap.String("version", targetVersion))
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Failed to load skill: %s. Reason: %v", params.SkillName, err),
		}
	}
	if !exists {
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Skill version not found: %s", targetVersion),
		}
	}
	versionDO := models.ConvertSkillVersionPO2DO(versionPO)

	// 6. 在 SkillFiles 里找 filepath
	var targetFile *models.SkillFile
	for i := range versionDO.SkillFiles {
		if versionDO.SkillFiles[i].Path == filepath {
			targetFile = &versionDO.SkillFiles[i]
			break
		}
	}
	if targetFile == nil {
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: File not found: %s", filepath),
		}
	}

	// 7. 读文件内容
	rc, err := t.storage.GetObject(dtos.SkillBucketName, targetFile.FileKey)
	if err != nil {
		logger.Error("loadSkill get object failed", zap.Error(err),
			zap.String("file_key", targetFile.FileKey))
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Failed to load skill: %s. Reason: cannot read file: %v", params.SkillName, err),
		}
	}
	defer rc.Close()

	contentBytes, err := io.ReadAll(rc)
	if err != nil {
		logger.Error("loadSkill read file failed", zap.Error(err),
			zap.String("file_key", targetFile.FileKey))
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: Failed to load skill: %s. Reason: read failed: %v", params.SkillName, err),
		}
	}

	return models.ToolCallResult{Success: true, Data: string(contentBytes)}
}

func (t *LoadSkillTool) getAvailableSkillNames() string {
	if len(t.skillRefs) == 0 {
		return "[]"
	}

	ids := make([]string, 0, len(t.skillRefs))
	for _, ref := range t.skillRefs {
		ids = append(ids, ref.SkillID)
	}

	pos, err := t.skillRepo.GetByIds(ids)
	if err != nil {
		return "[]"
	}

	names := make([]string, 0, len(pos))
	for _, po := range pos {
		names = append(names, po.SkillName)
	}

	return fmt.Sprintf("[%s]", strings.Join(names, ", "))
}
