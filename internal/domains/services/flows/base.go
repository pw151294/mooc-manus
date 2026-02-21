package flows

import (
	agent "mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/domains/models/events"
)

type BaseFlow interface {
	Invoke(agent.ChatRequest, chan events.AgentEvent)
	Done() bool
}
