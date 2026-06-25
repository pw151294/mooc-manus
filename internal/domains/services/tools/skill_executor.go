package tools

// SkillExecutionContext 脚本执行上下文（最小集字段）
type SkillExecutionContext struct {
	SkillID   string // Skill ID
	Version   string // Skill 版本
	MessageID string // 容器复用标识（空字符串触发一次性容器模式）
}

// SkillExecutionResult 单次执行结果
type SkillExecutionResult struct {
	Stdout      string   // 标准输出
	Stderr      string   // 错误输出
	Status      string   // 执行状态（completed / failed）
	OutputFiles []string // 产物文件路径（宿主机绝对路径）
}

// SkillExecutor 脚本执行器接口
// 当前提供 DockerSkillExecutor，基于 Docker 容器沙箱实现资源隔离与脚本执行
type SkillExecutor interface {
	// Execute 执行 Skill 脚本，返回执行结果切片
	// 当 ctx.MessageID 为空时使用一次性容器，非空时使用容器池复用
	Execute(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error)

	// CleanupMessage 清理指定 messageID 关联的容器与 skills 目录（保留 outputs）
	// 应在对话/消息生命周期结束时调用；messageID 为空时为 no-op
	CleanupMessage(messageID string) error
}
