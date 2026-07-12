package services

import (
	"errors"
	"sync/atomic"
	"time"

	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

var ErrAlreadyPending = errors.New("pending interrupt already exists for messageId")

type pendingSlot struct {
	snapshot agents.InterruptSnapshot
	ch       chan agents.InterruptDecision
	resolved atomic.Bool
	timer    *time.Timer
}

// resolve 保证 chan 只被写入一次；返回 true 表示这次决策生效
func (p *pendingSlot) resolve(d agents.InterruptDecision) bool {
	if !p.resolved.CompareAndSwap(false, true) {
		return false
	}
	if p.timer != nil {
		p.timer.Stop()
	}
	p.ch <- d
	close(p.ch)
	return true
}

// RegisterInterrupt 由 BaseAgent.InvokeToolCalls 调用；本 messageId 已有 pending 时返回 ErrAlreadyPending
func (s *BaseAgentApplicationServiceImpl) RegisterInterrupt(
	messageId string, snap agents.InterruptSnapshot,
) (<-chan agents.InterruptDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.pendingInterrupts[messageId]; exists {
		return nil, ErrAlreadyPending
	}
	slot := &pendingSlot{
		snapshot: snap,
		ch:       make(chan agents.InterruptDecision, 1),
	}
	slot.timer = time.AfterFunc(s.waitTimeout, func() {
		s.mu.Lock()
		cur, ok := s.pendingInterrupts[messageId]
		if ok && cur == slot {
			delete(s.pendingInterrupts, messageId)
		}
		s.mu.Unlock()
		if ok && cur == slot {
			_ = slot.resolve(agents.InterruptDecision{Kind: agents.DecisionTimeout})
		}
	})
	s.pendingInterrupts[messageId] = slot
	return slot.ch, nil
}

// WaitTimeout 返回 HITL 等待用户决策的最大时长
func (s *BaseAgentApplicationServiceImpl) WaitTimeout() time.Duration {
	return s.waitTimeout
}

// Resume 处理用户决策回投
func (s *BaseAgentApplicationServiceImpl) Resume(req dtos.ResumeClientRequest) dtos.ResumeResult {
	s.mu.Lock()
	slot, ok := s.pendingInterrupts[req.MessageId]
	if !ok || slot.snapshot.ToolCallID != req.ToolCallId {
		s.mu.Unlock()
		logger.Info("HITL Resume 未匹配到 pending",
			zap.String("component", "hitl"),
			zap.String("mid", req.MessageId),
			zap.String("tcid", req.ToolCallId))
		return dtos.ResumeResult{Status: "not_found"}
	}
	delete(s.pendingInterrupts, req.MessageId)
	s.mu.Unlock()

	d := agents.InterruptDecision{
		Kind:     agents.InterruptDecisionKind(req.Decision),
		Feedback: req.Feedback,
	}
	if !slot.resolve(d) {
		logger.Info("HITL Resume 抢先失败（timer 已 resolve）",
			zap.String("component", "hitl"),
			zap.String("mid", req.MessageId))
		return dtos.ResumeResult{Status: "already_decided"}
	}
	logger.Info("HITL Resume 生效",
		zap.String("component", "hitl"),
		zap.String("mid", req.MessageId),
		zap.String("decision", req.Decision))
	return dtos.ResumeResult{Status: "accepted"}
}
