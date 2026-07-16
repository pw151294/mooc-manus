package evaluation

import "time"

type Result struct {
	InstanceID       string
	Passed           bool
	VerifyExitCode   int
	VerifyStdout     string
	VerifyStderr     string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	AgentLatencyMs   int64
	ErrorLog         string
	FinishedAt       time.Time
}
