package models

import "time"

type EvalResultPO struct {
	InstanceID       string `gorm:"type:uuid;uniqueIndex;constraint:OnDelete:CASCADE"`
	Passed           bool
	VerifyExitCode   int
	VerifyStdout     string `gorm:"type:text"`
	VerifyStderr     string `gorm:"type:text"`
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	AgentLatencyMs   int64
	ErrorLog         string `gorm:"type:text"`
	FinishedAt       time.Time
}

func (EvalResultPO) TableName() string {
	return "eval_result"
}
