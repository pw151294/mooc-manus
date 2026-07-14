package tools

import (
	"mooc-manus/internal/domains/models/events"
)

// SubagentEventBridge 负责将子智能体产出的事件透传到主智能体事件流，
// 并为 ToolEvent 附加 subagent metadata 以便前端识别来源。
type SubagentEventBridge struct {
	parentEventCh chan<- events.AgentEvent
	subagentId    string
	taskDesc      string
	taskContext   string
}

// NewSubagentEventBridge 创建事件桥接器
func NewSubagentEventBridge(
	parentCh chan<- events.AgentEvent,
	subagentId, taskDesc, taskContext string,
) *SubagentEventBridge {
	return &SubagentEventBridge{
		parentEventCh: parentCh,
		subagentId:    subagentId,
		taskDesc:      taskDesc,
		taskContext:   taskContext,
	}
}

// ForwardEvent 将子智能体事件转发到主智能体事件通道。
// 对 ToolEvent 注入 subagent metadata；其他事件类型直接转发。
// 如果 parentEventCh 为 nil，跳过转发（降级模式：子智能体事件不透传）。
func (b *SubagentEventBridge) ForwardEvent(event events.AgentEvent) {
	if b.parentEventCh == nil {
		return
	}
	if toolEvent, ok := event.(*events.ToolEvent); ok {
		toolEvent.Metadata = map[string]interface{}{
			"subagent_id":      b.subagentId,
			"is_subagent":      true,
			"subagent_task":    b.taskDesc,
			"subagent_context": b.taskContext,
		}
		b.parentEventCh <- toolEvent
		return
	}
	b.parentEventCh <- event
}
