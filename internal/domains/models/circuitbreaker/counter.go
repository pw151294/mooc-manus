package circuitbreaker

// ToolCallCounter 会话级工具调用失败计数器
// 生命周期：绑定单个 conversationId，与 ChatMemory 同步创建/销毁
// 并发约束：所有方法非线程安全，调用方必须保证同一时刻仅有一个 goroutine 访问
// （BaseAgent.Invoke / StreamingInvoke 每轮 wg.Wait 已保证）
type ToolCallCounter struct {
	// Key: 工具名+参数指纹哈希，Value: 连续失败次数
	failureCounts map[string]int

	// 记录 Key 到工具元信息的映射（用于生成干预提示）
	keyMetadata map[string]ToolCallMetadata
}

// ToolCallMetadata 工具调用元信息
type ToolCallMetadata struct {
	ToolName      string
	ParamsPreview string
}

// FailureRecord 失败记录，用于生成干预提示
type FailureRecord struct {
	ToolName      string
	ParamsPreview string
	FailCount     int
}

// NewToolCallCounter 创建计数器实例
func NewToolCallCounter() *ToolCallCounter {
	return &ToolCallCounter{
		failureCounts: make(map[string]int),
		keyMetadata:   make(map[string]ToolCallMetadata),
	}
}

// RecordFailure 记录单次失败，返回当前累计次数
func (c *ToolCallCounter) RecordFailure(key string, metadata ToolCallMetadata) int {
	c.failureCounts[key]++
	c.keyMetadata[key] = metadata

	// 防止单 Key 失败次数异常累积
	if c.failureCounts[key] > 1000 {
		c.failureCounts[key] = 1000
	}
	return c.failureCounts[key]
}

// StartNewRound 开始新一轮工具调用（清零上一轮未重复的失败记录）
func (c *ToolCallCounter) StartNewRound(currentRoundKeys []string) {
	currentKeys := make(map[string]bool, len(currentRoundKeys))
	for _, k := range currentRoundKeys {
		currentKeys[k] = true
	}

	// 清零策略：如果上一轮的 Key 在本轮未出现，清零该 Key
	for k := range c.failureCounts {
		if !currentKeys[k] {
			delete(c.failureCounts, k)
			delete(c.keyMetadata, k)
		}
	}
}

// GetTriggeredRecords 返回所有达到阈值的失败记录
func (c *ToolCallCounter) GetTriggeredRecords(threshold int) []FailureRecord {
	records := make([]FailureRecord, 0)
	for key, count := range c.failureCounts {
		if count >= threshold {
			meta := c.keyMetadata[key]
			records = append(records, FailureRecord{
				ToolName:      meta.ToolName,
				ParamsPreview: meta.ParamsPreview,
				FailCount:     count,
			})
		}
	}
	return records
}
