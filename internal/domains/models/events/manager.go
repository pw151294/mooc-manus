package events

import (
	"mooc-manus/internal/domains/models/agents"
	"sync"
)

var eventManager *EventManager

type EventManager struct {
	sync.RWMutex
	conversationId2Events map[string][]AgentEvent
}

func init() {
	eventManager = &EventManager{
		conversationId2Events: make(map[string][]AgentEvent),
	}
}

// Manager 返回包内的单例 EventManager（如确需直接调用方法可用此函数）
func Manager() *EventManager {
	return eventManager
}

// AddEvent 向指定会话追加事件（包级API）
func AddEvent(conversationId string, event AgentEvent) {
	eventManager.AddEvent(conversationId, event)
}

// GetLatestPlan 获取指定会话最新的 Plan（包级API）
func GetLatestPlan(conversationId string) agents.Plan {
	return eventManager.GetLatestPlan(conversationId)
}

// Delete 删除指定会话的事件记录（包级API）
func Delete(conversationId string) {
	eventManager.Delete(conversationId)
}

func (m *EventManager) AddEvent(conversationId string, event AgentEvent) {
	m.Lock()
	defer m.Unlock()

	if events, ok := m.conversationId2Events[conversationId]; ok {
		m.conversationId2Events[conversationId] = append(events, event)
	} else {
		m.conversationId2Events[conversationId] = []AgentEvent{event}
	}
}

func (m *EventManager) GetLatestPlan(conversationId string) agents.Plan {
	m.RLock()
	defer m.RUnlock()

	events := m.conversationId2Events[conversationId]
	if len(events) == 0 {
		return agents.Plan{}
	}
	for i := len(events) - 1; i >= 0; i-- {
		if event, ok := events[i].(*PlanEvent); ok {
			return event.Plan
		}
	}
	return agents.Plan{}
}

func (m *EventManager) Delete(conversationId string) {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.conversationId2Events[conversationId]; ok {
		delete(m.conversationId2Events, conversationId)
	}
}
