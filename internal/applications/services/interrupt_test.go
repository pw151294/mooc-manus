package services

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/services/agents"
)

func newTestSlot() *pendingSlot {
	return &pendingSlot{
		snapshot: agents.InterruptSnapshot{ToolCallID: "tc1"},
		ch:       make(chan agents.InterruptDecision, 1),
	}
}

func TestPendingSlot_ResolveFirstWins(t *testing.T) {
	slot := newTestSlot()
	if !slot.resolve(agents.InterruptDecision{Kind: agents.DecisionApprove}) {
		t.Fatal("first resolve should return true")
	}
	if slot.resolve(agents.InterruptDecision{Kind: agents.DecisionReject}) {
		t.Fatal("second resolve should return false")
	}
	d := <-slot.ch
	if d.Kind != agents.DecisionApprove {
		t.Fatalf("chan should hold approve, got %v", d.Kind)
	}
}

func TestPendingSlot_ConcurrentResolve(t *testing.T) {
	slot := newTestSlot()
	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kind := agents.DecisionApprove
			if i%2 == 0 {
				kind = agents.DecisionReject
			}
			if slot.resolve(agents.InterruptDecision{Kind: kind}) {
				atomic.AddInt32(&wins, 1)
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("exactly 1 winner expected, got %d", wins)
	}
	count := 0
	for range slot.ch {
		count++
	}
	if count != 1 {
		t.Fatalf("chan should have exactly 1 value, got %d", count)
	}
}

func newTestSvc(timeout time.Duration) *BaseAgentApplicationServiceImpl {
	return &BaseAgentApplicationServiceImpl{
		cancelFuncs:       make(map[string]context.CancelFunc),
		pendingInterrupts: make(map[string]*pendingSlot),
		waitTimeout:       timeout,
	}
}

func TestRegisterInterrupt_Success(t *testing.T) {
	s := newTestSvc(time.Minute)
	ch, err := s.RegisterInterrupt("m1", agents.InterruptSnapshot{ToolCallID: "tc1"})
	if err != nil || ch == nil {
		t.Fatalf("want success, got %v", err)
	}
	if _, ok := s.pendingInterrupts["m1"]; !ok {
		t.Fatal("slot not registered")
	}
}

func TestRegisterInterrupt_AlreadyPending(t *testing.T) {
	s := newTestSvc(time.Minute)
	_, _ = s.RegisterInterrupt("m1", agents.InterruptSnapshot{ToolCallID: "tc1"})
	_, err := s.RegisterInterrupt("m1", agents.InterruptSnapshot{ToolCallID: "tc2"})
	if !errors.Is(err, ErrAlreadyPending) {
		t.Fatalf("want ErrAlreadyPending, got %v", err)
	}
}

func TestRegisterInterrupt_TimerFires(t *testing.T) {
	s := newTestSvc(50 * time.Millisecond)
	ch, _ := s.RegisterInterrupt("m1", agents.InterruptSnapshot{ToolCallID: "tc1"})
	select {
	case d := <-ch:
		if d.Kind != agents.DecisionTimeout {
			t.Fatalf("want Timeout, got %v", d.Kind)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timer did not fire")
	}
	s.mu.Lock()
	_, ok := s.pendingInterrupts["m1"]
	s.mu.Unlock()
	if ok {
		t.Fatal("slot should be deleted after timer fire")
	}
}

func TestResume_Approve(t *testing.T) {
	s := newTestSvc(time.Minute)
	ch, _ := s.RegisterInterrupt("m1", agents.InterruptSnapshot{ToolCallID: "tc1"})
	go func() { <-ch }()
	r := s.Resume(dtos.ResumeClientRequest{MessageId: "m1", ToolCallId: "tc1", Decision: "approve"})
	if r.Status != "accepted" {
		t.Fatalf("want accepted, got %s", r.Status)
	}
}

func TestResume_WrongToolCallId(t *testing.T) {
	s := newTestSvc(time.Minute)
	_, _ = s.RegisterInterrupt("m1", agents.InterruptSnapshot{ToolCallID: "tc1"})
	r := s.Resume(dtos.ResumeClientRequest{MessageId: "m1", ToolCallId: "tcOther", Decision: "approve"})
	if r.Status != "not_found" {
		t.Fatalf("want not_found, got %s", r.Status)
	}
}

func TestResume_DoubleCall(t *testing.T) {
	s := newTestSvc(time.Minute)
	ch, _ := s.RegisterInterrupt("m1", agents.InterruptSnapshot{ToolCallID: "tc1"})
	go func() { <-ch }()
	_ = s.Resume(dtos.ResumeClientRequest{MessageId: "m1", ToolCallId: "tc1", Decision: "approve"})
	r := s.Resume(dtos.ResumeClientRequest{MessageId: "m1", ToolCallId: "tc1", Decision: "approve"})
	if r.Status != "not_found" {
		t.Fatalf("second call want not_found, got %s", r.Status)
	}
}

func TestResume_TimerBeatsResume(t *testing.T) {
	s := newTestSvc(20 * time.Millisecond)
	ch, _ := s.RegisterInterrupt("m1", agents.InterruptSnapshot{ToolCallID: "tc1"})
	<-ch // 让 timer 先 fire
	time.Sleep(30 * time.Millisecond)
	r := s.Resume(dtos.ResumeClientRequest{MessageId: "m1", ToolCallId: "tc1", Decision: "approve"})
	if r.Status != "not_found" {
		t.Fatalf("want not_found, got %s", r.Status)
	}
}
