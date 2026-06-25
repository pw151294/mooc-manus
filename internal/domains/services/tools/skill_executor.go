package tools

import "fmt"

// SkillExecutionContext 脚本执行上下文（最小集字段）
type SkillExecutionContext struct {
	SkillID   string // Skill ID
	Version   string // Skill 版本
	MessageID string // 容器复用标识（当前占位实现不使用，但保留语义）
}

// SkillExecutionResult 单次执行结果
type SkillExecutionResult struct {
	Stdout      string   // 标准输出
	Stderr      string   // 错误输出
	Status      string   // 执行状态（running / completed / failed）
	OutputFiles []string // 产物文件路径（宿主机路径）
}

// SkillExecutor 脚本执行器接口
// 当前提供占位实现 StubSkillExecutor，后续可扩展 DockerSkillExecutor 等容器化执行实现
type SkillExecutor interface {
	// Execute 执行 Skill 脚本，返回流式结果切片
	// 当前版本返回单条或多条结果；未来 Docker 实现可改为流式推送
	Execute(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error)
}

// StubSkillExecutor 占位实现，提示功能未实现
type StubSkillExecutor struct{}

// NewStubSkillExecutor 创建占位执行器实例
func NewStubSkillExecutor() SkillExecutor {
	return &StubSkillExecutor{}
}

// Execute 占位实现：返回"功能未实现"错误
func (e *StubSkillExecutor) Execute(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	return nil, fmt.Errorf("SkillExecutor not implemented: container execution capability is not available in current deployment. Please configure Docker-based executor to enable this feature")
}
