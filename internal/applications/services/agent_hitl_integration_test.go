package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/interrupt"
	"mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/tools"
)

// -----------------------------------------------------------------------------
// HITL 集成测试：验证 BaseAgent.InvokeToolCalls 与
// BaseAgentApplicationServiceImpl.RegisterInterrupt/Resume 的端到端协作。
//
// 每个用例都独占自己的 svc / agent / memory，保持相互隔离。
// -----------------------------------------------------------------------------

// buildAgent 构造一个可用于 HITL 集成测试的 BaseAgent。
// 传入的 pendingSink 可以为 nil，用于 I-13。
func buildAgent(mid string, sink agents.PendingSink, tool tools.Tool) *agents.BaseAgent {
	mem := memory.NewChatMemory()
	inv := &mockInvoker{}
	cfg := models.AgentConfig{MaxIterations: 3, MaxRetries: 1}
	toolList := []tools.Tool{}
	if tool != nil {
		toolList = append(toolList, tool)
	}
	opts := []agents.BaseAgentOption{agents.WithMessageId(mid)}
	if sink != nil {
		opts = append(opts, agents.WithPendingSink(sink))
	}
	return agents.NewBaseAgent(cfg, inv, mem, toolList, "sys", opts...)
}

// runInvokeToolCalls 在独立 goroutine 中跑 InvokeToolCalls，返回 tool messages 与消费到的事件。
// eventCh 由本函数创建并 close；调用方无需干预。
func runInvokeToolCalls(ctx context.Context, agent *agents.BaseAgent, toolCalls []llm.ToolCall) ([]llm.Message, []events.AgentEvent, chan struct{}) {
	eventCh := make(chan events.AgentEvent, 32)
	eventsDone := make(chan []events.AgentEvent, 1)
	go drainEvents(eventCh, eventsDone)

	msgs := make(chan []llm.Message, 1)
	finished := make(chan struct{})
	go func() {
		out := agent.InvokeToolCalls(ctx, toolCalls, eventCh)
		close(eventCh)
		msgs <- out
		close(finished)
	}()

	toolMsgs := <-msgs
	collected := <-eventsDone
	return toolMsgs, collected, finished
}

// argsDangerous 构造一个 risk_level=dangerous 的 arguments JSON
func argsDangerous(command string) string {
	return `{"command":"` + command + `","risk_level":"dangerous","risk_reason":"rm -rf 高危"}`
}
func argsSafe(command string) string {
	return `{"command":"` + command + `","risk_level":"safe","risk_reason":""}`
}
func argsNoRisk(command string) string {
	return `{"command":"` + command + `"}`
}

// waitForPending 轮询 svc.pendingInterrupts，直到 messageId 出现或超时。
// 用于同步：Register 发生在 Agent goroutine，主 goroutine 需等其入表后再 Resume。
func waitForPending(t *testing.T, svc *BaseAgentApplicationServiceImpl, mid string, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		_, ok := svc.pendingInterrupts[mid]
		svc.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("waitForPending: pending slot for %q not registered within %v", mid, d)
}

// -----------------------------------------------------------------------------
// I-01: 危险命令 + approve -> Agent 继续执行工具，正常收到 result
// -----------------------------------------------------------------------------
func TestHITL_I01_DangerousApproveExecutesTool(t *testing.T) {
	svc := newTestSvc(2 * time.Second)
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i01", svc, tool)

	toolCalls := []llm.ToolCall{{ID: "tc1", Name: "bashExec", Arguments: argsDangerous("rm -rf /tmp/x")}}

	done := make(chan struct{})
	var toolMsgs []llm.Message
	go func() {
		msgs, _, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)
		toolMsgs = msgs
		close(done)
	}()

	waitForPending(t, svc, "mid-i01", time.Second)
	r := svc.Resume(dtos.ResumeClientRequest{MessageId: "mid-i01", ToolCallId: "tc1", Decision: "approve"})
	if r.Status != "accepted" {
		t.Fatalf("Resume status = %q, want accepted", r.Status)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("InvokeToolCalls 未在预期时间内返回")
	}

	if tool.invokedTimes() != 1 {
		t.Fatalf("mockTool.Invoke 应被调用 1 次，got %d", tool.invokedTimes())
	}
	if len(toolMsgs) != 1 {
		t.Fatalf("toolMsgs len = %d, want 1: %+v", len(toolMsgs), toolMsgs)
	}
	if !strings.Contains(toolMsgs[0].Content, "mock tool executed") {
		t.Fatalf("tool result 应含正常执行结果，got %q", toolMsgs[0].Content)
	}
}

// -----------------------------------------------------------------------------
// I-02: 危险命令 + reject（无反馈）-> tool result = MsgUserReject
// -----------------------------------------------------------------------------
func TestHITL_I02_DangerousRejectNoFeedback(t *testing.T) {
	svc := newTestSvc(2 * time.Second)
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i02", svc, tool)

	toolCalls := []llm.ToolCall{{ID: "tc1", Name: "bashExec", Arguments: argsDangerous("rm -rf /")}}

	done := make(chan struct{})
	var toolMsgs []llm.Message
	go func() {
		msgs, _, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)
		toolMsgs = msgs
		close(done)
	}()

	waitForPending(t, svc, "mid-i02", time.Second)
	r := svc.Resume(dtos.ResumeClientRequest{MessageId: "mid-i02", ToolCallId: "tc1", Decision: "reject"})
	if r.Status != "accepted" {
		t.Fatalf("Resume status = %q, want accepted", r.Status)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("InvokeToolCalls 未在预期时间内返回")
	}

	if tool.invokedTimes() != 0 {
		t.Fatalf("mockTool.Invoke 应未被调用，got %d", tool.invokedTimes())
	}
	if len(toolMsgs) != 1 {
		t.Fatalf("toolMsgs len = %d, want 1", len(toolMsgs))
	}
	if toolMsgs[0].Content != interrupt.MsgUserReject {
		t.Fatalf("tool result Content = %q, want MsgUserReject", toolMsgs[0].Content)
	}
	if toolMsgs[0].ToolCallID != "tc1" {
		t.Fatalf("ToolCallID = %q, want tc1", toolMsgs[0].ToolCallID)
	}
}

// -----------------------------------------------------------------------------
// I-03: 危险命令 + reject（有反馈）-> tool result 含 feedback
// -----------------------------------------------------------------------------
func TestHITL_I03_DangerousRejectWithFeedback(t *testing.T) {
	svc := newTestSvc(2 * time.Second)
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i03", svc, tool)

	toolCalls := []llm.ToolCall{{ID: "tc1", Name: "bashExec", Arguments: argsDangerous("rm -rf /var")}}

	done := make(chan struct{})
	var toolMsgs []llm.Message
	go func() {
		msgs, _, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)
		toolMsgs = msgs
		close(done)
	}()

	waitForPending(t, svc, "mid-i03", time.Second)
	feedback := "换成 rm -rf /tmp/x 更安全"
	r := svc.Resume(dtos.ResumeClientRequest{
		MessageId:  "mid-i03",
		ToolCallId: "tc1",
		Decision:   "reject",
		Feedback:   feedback,
	})
	if r.Status != "accepted" {
		t.Fatalf("Resume status = %q, want accepted", r.Status)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("InvokeToolCalls 未在预期时间内返回")
	}

	if tool.invokedTimes() != 0 {
		t.Fatalf("mockTool.Invoke 应未被调用，got %d", tool.invokedTimes())
	}
	if len(toolMsgs) != 1 {
		t.Fatalf("toolMsgs len = %d, want 1", len(toolMsgs))
	}
	if !strings.Contains(toolMsgs[0].Content, feedback) {
		t.Fatalf("tool result Content 应含 feedback %q，实际 %q", feedback, toolMsgs[0].Content)
	}
}

// -----------------------------------------------------------------------------
// I-04: 危险命令 + timeout -> tool result = MsgTimeout
// -----------------------------------------------------------------------------
func TestHITL_I04_DangerousTimeoutRejects(t *testing.T) {
	// waitTimeout 设小一点，避免用例慢
	svc := newTestSvc(80 * time.Millisecond)
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i04", svc, tool)

	toolCalls := []llm.ToolCall{{ID: "tc1", Name: "bashExec", Arguments: argsDangerous("shutdown -h now")}}

	toolMsgs, _, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)

	if tool.invokedTimes() != 0 {
		t.Fatalf("mockTool.Invoke 应未被调用，got %d", tool.invokedTimes())
	}
	if len(toolMsgs) != 1 {
		t.Fatalf("toolMsgs len = %d, want 1", len(toolMsgs))
	}
	if toolMsgs[0].Content != interrupt.MsgTimeout {
		t.Fatalf("tool result Content = %q, want MsgTimeout", toolMsgs[0].Content)
	}
}

// -----------------------------------------------------------------------------
// I-05: 安全命令（risk_level=safe）-> 不触发 HITL 闸门，直接执行
// -----------------------------------------------------------------------------
func TestHITL_I05_SafeSkipsGate(t *testing.T) {
	svc := newTestSvc(2 * time.Second)
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i05", svc, tool)

	toolCalls := []llm.ToolCall{{ID: "tc1", Name: "bashExec", Arguments: argsSafe("ls /tmp")}}

	toolMsgs, evs, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)

	if tool.invokedTimes() != 1 {
		t.Fatalf("mockTool.Invoke 应被调用 1 次，got %d", tool.invokedTimes())
	}
	if len(toolMsgs) != 1 || !strings.Contains(toolMsgs[0].Content, "mock tool executed") {
		t.Fatalf("toolMsgs = %+v, 应含正常执行结果", toolMsgs)
	}
	// 不应有 interrupt 事件
	for _, ev := range evs {
		if _, ok := ev.(*events.ToolInterruptEvent); ok {
			t.Fatal("safe 命令不应触发 ToolInterruptEvent")
		}
	}
	// pending slot 也不应留下
	svc.mu.Lock()
	_, ok := svc.pendingInterrupts["mid-i05"]
	svc.mu.Unlock()
	if ok {
		t.Fatal("safe 命令不应注册 pending")
	}
}

// -----------------------------------------------------------------------------
// I-06: risk_level 缺失 -> 降级为直接执行（Warn 日志由 logger 承担）
// -----------------------------------------------------------------------------
func TestHITL_I06_MissingRiskDegrades(t *testing.T) {
	svc := newTestSvc(2 * time.Second)
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i06", svc, tool)

	toolCalls := []llm.ToolCall{{ID: "tc1", Name: "bashExec", Arguments: argsNoRisk("ls /tmp")}}

	toolMsgs, _, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)

	if tool.invokedTimes() != 1 {
		t.Fatalf("risk_level 缺失应降级为直接执行，got invoke=%d", tool.invokedTimes())
	}
	if len(toolMsgs) != 1 || !strings.Contains(toolMsgs[0].Content, "mock tool executed") {
		t.Fatalf("toolMsgs = %+v, 应含正常执行结果", toolMsgs)
	}
	svc.mu.Lock()
	_, ok := svc.pendingInterrupts["mid-i06"]
	svc.mu.Unlock()
	if ok {
		t.Fatal("risk_level 缺失不应注册 pending")
	}
}

// -----------------------------------------------------------------------------
// I-07: Stop 触发 Cancel -> pending 被解绑，Agent goroutine 从 InvokeToolCalls 退出
// 注：InvokeToolCalls 在 DecisionCancel 分支直接 return，不追加 tool message
// -----------------------------------------------------------------------------
func TestHITL_I07_StopCancelsPending(t *testing.T) {
	svc := newTestSvc(5 * time.Second)
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i07", svc, tool)

	toolCalls := []llm.ToolCall{{ID: "tc1", Name: "bashExec", Arguments: argsDangerous("rm -rf /")}}

	done := make(chan struct{})
	var toolMsgs []llm.Message
	go func() {
		msgs, _, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)
		toolMsgs = msgs
		close(done)
	}()

	waitForPending(t, svc, "mid-i07", time.Second)

	// 模拟 Stop 路径的 pending 解绑动作
	svc.mu.Lock()
	slot := svc.pendingInterrupts["mid-i07"]
	delete(svc.pendingInterrupts, "mid-i07")
	svc.mu.Unlock()
	if !slot.resolve(agents.InterruptDecision{Kind: agents.DecisionCancel}) {
		t.Fatal("首次 Cancel resolve 应成功")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Agent goroutine 未在预期时间内退出")
	}

	if tool.invokedTimes() != 0 {
		t.Fatalf("mockTool.Invoke 应未被调用，got %d", tool.invokedTimes())
	}
	// DecisionCancel 分支直接 return toolMessages（此时 slice 长度应为 0，Stop 路径由 app service 补齐孤儿）
	if len(toolMsgs) != 0 {
		t.Fatalf("Cancel 分支返回 toolMsgs len = %d, want 0（Stop 路径外部补齐），实际 %+v", len(toolMsgs), toolMsgs)
	}

	svc.mu.Lock()
	_, stillHas := svc.pendingInterrupts["mid-i07"]
	svc.mu.Unlock()
	if stillHas {
		t.Fatal("pending 应已解绑")
	}
}

// -----------------------------------------------------------------------------
// I-08: 同 messageId 二次 Register 返回 ErrAlreadyPending
// 单元测试已覆盖等价场景（TestRegisterInterrupt_AlreadyPending），
// 此处做端到端等价确认：并发第二次 Register 立即失败。
// -----------------------------------------------------------------------------
func TestHITL_I08_DoubleRegisterRejected(t *testing.T) {
	svc := newTestSvc(2 * time.Second)
	_, err := svc.RegisterInterrupt("mid-i08", agents.InterruptSnapshot{ToolCallID: "tc1"})
	if err != nil {
		t.Fatalf("first register err = %v", err)
	}
	_, err = svc.RegisterInterrupt("mid-i08", agents.InterruptSnapshot{ToolCallID: "tc2"})
	if !errors.Is(err, ErrAlreadyPending) {
		t.Fatalf("second register err = %v, want ErrAlreadyPending", err)
	}
}

// -----------------------------------------------------------------------------
// I-09: 一轮多 toolCall，其中第 2 个是危险且被拒 -> siblings 中第 3+ 追加 MsgSiblingSkipped
// -----------------------------------------------------------------------------
func TestHITL_I09_RejectAbortsFollowingSiblings(t *testing.T) {
	svc := newTestSvc(2 * time.Second)
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i09", svc, tool)

	toolCalls := []llm.ToolCall{
		{ID: "tc1", Name: "bashExec", Arguments: argsSafe("ls /tmp")},        // 应正常执行
		{ID: "tc2", Name: "bashExec", Arguments: argsDangerous("rm -rf /")}, // 危险 -> 触发 HITL 闸门
		{ID: "tc3", Name: "bashExec", Arguments: argsSafe("echo done")},    // 应被 SiblingSkipped
		{ID: "tc4", Name: "bashExec", Arguments: argsSafe("echo again")},   // 应被 SiblingSkipped
	}

	done := make(chan struct{})
	var toolMsgs []llm.Message
	go func() {
		msgs, _, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)
		toolMsgs = msgs
		close(done)
	}()

	waitForPending(t, svc, "mid-i09", time.Second)
	r := svc.Resume(dtos.ResumeClientRequest{MessageId: "mid-i09", ToolCallId: "tc2", Decision: "reject"})
	if r.Status != "accepted" {
		t.Fatalf("Resume status = %q, want accepted", r.Status)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("InvokeToolCalls 未在预期时间内返回")
	}

	// 期望 4 条：tc1 正常执行结果、tc2 reject、tc3/tc4 sibling skipped
	if len(toolMsgs) != 4 {
		t.Fatalf("toolMsgs len = %d, want 4, got %+v", len(toolMsgs), toolMsgs)
	}
	byID := map[string]llm.Message{}
	for _, m := range toolMsgs {
		byID[m.ToolCallID] = m
	}
	if !strings.Contains(byID["tc1"].Content, "mock tool executed") {
		t.Fatalf("tc1 应正常执行，got %q", byID["tc1"].Content)
	}
	if byID["tc2"].Content != interrupt.MsgUserReject {
		t.Fatalf("tc2 应为 MsgUserReject，got %q", byID["tc2"].Content)
	}
	if byID["tc3"].Content != interrupt.MsgSiblingSkipped {
		t.Fatalf("tc3 应为 MsgSiblingSkipped，got %q", byID["tc3"].Content)
	}
	if byID["tc4"].Content != interrupt.MsgSiblingSkipped {
		t.Fatalf("tc4 应为 MsgSiblingSkipped，got %q", byID["tc4"].Content)
	}
	// tool 只应被调用 1 次（tc1）
	if tool.invokedTimes() != 1 {
		t.Fatalf("mockTool.Invoke 应被调用 1 次（tc1），got %d", tool.invokedTimes())
	}
}

// -----------------------------------------------------------------------------
// I-10: Timer 先 fire，Resume 后到 -> Resume 返回 not_found
// -----------------------------------------------------------------------------
func TestHITL_I10_TimerBeatsResume(t *testing.T) {
	svc := newTestSvc(20 * time.Millisecond)
	ch, err := svc.RegisterInterrupt("mid-i10", agents.InterruptSnapshot{ToolCallID: "tc1"})
	if err != nil {
		t.Fatalf("Register err = %v", err)
	}
	// 等 timer fire 完成，此时 slot 已 resolve 且从 map 删除
	select {
	case d := <-ch:
		if d.Kind != agents.DecisionTimeout {
			t.Fatalf("首个决策应为 Timeout，got %v", d.Kind)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timer 未在预期时间内 fire")
	}

	r := svc.Resume(dtos.ResumeClientRequest{MessageId: "mid-i10", ToolCallId: "tc1", Decision: "approve"})
	if r.Status != "not_found" {
		t.Fatalf("Resume status = %q, want not_found", r.Status)
	}
}

// -----------------------------------------------------------------------------
// I-11: Resume 先到，Timer 后 fire -> Timer 不产生副作用
// -----------------------------------------------------------------------------
func TestHITL_I11_ResumeBeatsTimer(t *testing.T) {
	svc := newTestSvc(60 * time.Millisecond)
	ch, err := svc.RegisterInterrupt("mid-i11", agents.InterruptSnapshot{ToolCallID: "tc1"})
	if err != nil {
		t.Fatalf("Register err = %v", err)
	}
	// 后台消费者：收到 approve 后计数
	got := make(chan agents.InterruptDecision, 2)
	go func() {
		for d := range ch {
			got <- d
		}
		close(got)
	}()

	r := svc.Resume(dtos.ResumeClientRequest{MessageId: "mid-i11", ToolCallId: "tc1", Decision: "approve"})
	if r.Status != "accepted" {
		t.Fatalf("Resume status = %q, want accepted", r.Status)
	}

	// 等超过 timer 的时长，确认 timer 到期后没有产生额外决策
	time.Sleep(120 * time.Millisecond)

	// 排空 got：应恰好只有 1 条 approve
	count := 0
	timeout := time.After(50 * time.Millisecond)
drain:
	for {
		select {
		case d, ok := <-got:
			if !ok {
				break drain
			}
			count++
			if d.Kind != agents.DecisionApprove {
				t.Fatalf("决策 kind = %v, want Approve", d.Kind)
			}
		case <-timeout:
			break drain
		}
	}
	if count != 1 {
		t.Fatalf("chan 应恰好只有 1 条决策，got %d", count)
	}

	// pending 已删除
	svc.mu.Lock()
	_, ok := svc.pendingInterrupts["mid-i11"]
	svc.mu.Unlock()
	if ok {
		t.Fatal("Resume 后 pending 应已删除")
	}
}

// -----------------------------------------------------------------------------
// I-12: 并发多个 Resume 到同一 slot -> 只有一个 accepted，其余 not_found
// -----------------------------------------------------------------------------
func TestHITL_I12_ConcurrentResumeFirstWins(t *testing.T) {
	svc := newTestSvc(5 * time.Second)
	ch, err := svc.RegisterInterrupt("mid-i12", agents.InterruptSnapshot{ToolCallID: "tc1"})
	if err != nil {
		t.Fatalf("Register err = %v", err)
	}
	// 消费 ch 防止阻塞
	go func() {
		for range ch {
		}
	}()

	const N = 32
	var wg sync.WaitGroup
	var accepted int32
	var notFound int32
	var others int32
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			decision := "approve"
			if i%2 == 0 {
				decision = "reject"
			}
			r := svc.Resume(dtos.ResumeClientRequest{
				MessageId:  "mid-i12",
				ToolCallId: "tc1",
				Decision:   decision,
			})
			switch r.Status {
			case "accepted":
				atomic.AddInt32(&accepted, 1)
			case "not_found":
				atomic.AddInt32(&notFound, 1)
			default:
				atomic.AddInt32(&others, 1)
			}
		}(i)
	}
	wg.Wait()

	if accepted != 1 {
		t.Fatalf("accepted = %d, want exactly 1", accepted)
	}
	if notFound != N-1 {
		t.Fatalf("not_found = %d, want %d", notFound, N-1)
	}
	if others != 0 {
		t.Fatalf("others = %d, want 0", others)
	}
}

// -----------------------------------------------------------------------------
// I-13: pendingSink 为 nil（A2A 场景）-> InvokeToolCalls 跳过 HITL 闸门
// -----------------------------------------------------------------------------
func TestHITL_I13_NilPendingSinkSkipsGate(t *testing.T) {
	tool := newMockTool("bashExec", true)
	agent := buildAgent("mid-i13", nil, tool) // sink=nil

	// 危险命令，但 sink=nil 应绕过闸门直接执行
	toolCalls := []llm.ToolCall{{ID: "tc1", Name: "bashExec", Arguments: argsDangerous("rm -rf /")}}
	toolMsgs, evs, _ := runInvokeToolCalls(context.Background(), agent, toolCalls)

	if tool.invokedTimes() != 1 {
		t.Fatalf("sink=nil 时应直接执行 tool，got invoke=%d", tool.invokedTimes())
	}
	if len(toolMsgs) != 1 || !strings.Contains(toolMsgs[0].Content, "mock tool executed") {
		t.Fatalf("toolMsgs = %+v, 应含正常执行结果", toolMsgs)
	}
	for _, ev := range evs {
		if _, ok := ev.(*events.ToolInterruptEvent); ok {
			t.Fatal("sink=nil 不应触发 ToolInterruptEvent")
		}
	}
}

