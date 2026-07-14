package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
)

func TestSubagentTool_ExecuteSuccess(t *testing.T) {
	parentEventCh := make(chan events.AgentEvent, 100)
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		// 模拟工具调用事件
		eventCh <- &events.ToolEvent{
			FunctionName: "fileRead",
			Status:       events.ToolEventStatusCalling,
		}
		eventCh <- &events.ToolEvent{
			FunctionName: "fileRead",
			Status:       events.ToolEventStatusCompleted,
		}
		// 模拟最终消息
		eventCh <- &events.MessageEvent{Message: "文件内容已读取完成"}
		close(eventCh)
	}

	baseTools := []Tool{&subagentMockTool{toolNames: []string{"fileRead"}}}
	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"msg-001",
		parentEventCh,
		mockRunner,
	)

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "读取文件内容",
		AllowedTools:    []string{"fileRead"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))

	if !result.Success {
		t.Fatalf("期望成功, got: %s", result.Message)
	}

	var subResult SubagentResult
	if err := json.Unmarshal([]byte(result.Data.(string)), &subResult); err != nil {
		t.Fatalf("解析结果失败: %v", err)
	}
	if !subResult.Success {
		t.Fatalf("期望 SubagentResult.Success=true, got error: %s", subResult.Error)
	}
	if subResult.Output != "文件内容已读取完成" {
		t.Fatalf("期望输出为'文件内容已读取完成', got: %s", subResult.Output)
	}
	if len(subResult.ToolCalls) != 1 || subResult.ToolCalls[0] != "fileRead" {
		t.Fatalf("期望 ToolCalls=[fileRead], got: %v", subResult.ToolCalls)
	}
}

func TestSubagentTool_ExecuteTimeout(t *testing.T) {
	parentEventCh := make(chan events.AgentEvent, 100)
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		<-ctx.Done()
		close(eventCh)
	}

	baseTools := []Tool{&subagentMockTool{toolNames: []string{"fileRead"}}}
	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"msg-002",
		parentEventCh,
		mockRunner,
	)
	tool.timeout = 100 * time.Millisecond

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "永不完成的任务",
		AllowedTools:    []string{"fileRead"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))

	if result.Success {
		t.Fatal("期望超时失败")
	}
	var subResult SubagentResult
	if err := json.Unmarshal([]byte(result.Data.(string)), &subResult); err != nil {
		t.Fatalf("解析结果失败: %v", err)
	}
	if subResult.Success {
		t.Fatal("期望 SubagentResult.Success=false (timeout)")
	}
	if subResult.Error == "" {
		t.Fatal("期望超时有错误信息")
	}
}

func TestSubagentTool_ExecuteContextCancelled(t *testing.T) {
	parentEventCh := make(chan events.AgentEvent, 100)
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		<-ctx.Done()
		close(eventCh)
	}

	baseTools := []Tool{&subagentMockTool{toolNames: []string{"fileRead"}}}
	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"msg-003",
		parentEventCh,
		mockRunner,
	)
	tool.timeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "被取消的任务",
		AllowedTools:    []string{"fileRead"},
	})
	result := tool.InvokeWithContext(ctx, SubagentToolName, string(args))

	if result.Success {
		t.Fatal("期望取消后失败")
	}
	var subResult SubagentResult
	if err := json.Unmarshal([]byte(result.Data.(string)), &subResult); err != nil {
		t.Fatalf("解析结果失败: %v", err)
	}
	if subResult.Success {
		t.Fatal("期望 SubagentResult.Success=false (cancelled)")
	}
	if subResult.Error == "" {
		t.Fatal("期望取消有错误信息")
	}
}

func TestSubagentTool_EventBridgeForwarding(t *testing.T) {
	parentEventCh := make(chan events.AgentEvent, 100)
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		eventCh <- &events.ToolEvent{
			FunctionName: "bashExec",
			Status:       events.ToolEventStatusCalling,
		}
		eventCh <- &events.ToolEvent{
			FunctionName: "bashExec",
			Status:       events.ToolEventStatusCompleted,
		}
		eventCh <- &events.MessageEvent{Message: "执行完毕"}
		close(eventCh)
	}

	baseTools := []Tool{&subagentMockTool{toolNames: []string{"bashExec"}}}
	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"msg-004",
		parentEventCh,
		mockRunner,
	)

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "执行命令",
		AllowedTools:    []string{"bashExec"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))
	if !result.Success {
		t.Fatalf("期望成功, got: %s", result.Message)
	}

	// 验证 parentEventCh 收到了转发的事件
	var receivedEvents []events.AgentEvent
	for {
		select {
		case evt := <-parentEventCh:
			receivedEvents = append(receivedEvents, evt)
		default:
			goto done
		}
	}
done:

	if len(receivedEvents) < 3 {
		t.Fatalf("期望至少 3 个转发事件, got %d", len(receivedEvents))
	}

	// 验证 ToolEvent 注入了 subagent metadata
	toolEvt, ok := receivedEvents[0].(*events.ToolEvent)
	if !ok {
		t.Fatal("第一个事件应为 ToolEvent")
	}
	if toolEvt.Metadata == nil {
		t.Fatal("ToolEvent 应包含 subagent metadata")
	}
	if toolEvt.Metadata["is_subagent"] != true {
		t.Fatal("metadata.is_subagent 应为 true")
	}
}