package prompts

import (
	_ "embed"
	"sync"
)

//go:embed system
var systemPrompt string

//go:embed plan/plan_system
var planSystemPrompt string

//go:embed plan/plan_create
var planCreatePrompt string

//go:embed plan/plan_update
var planUpdatePrompt string

//go:embed react/react_system
var reActSystemPrompt string

//go:embed react/execution
var executionPrompt string

//go:embed react/summarize
var summarizePrompt string

type PromptManager struct {
	sync.Mutex
	systemPrompt      string
	planSystemPrompt  string
	planCreatePrompt  string
	planUpdatePrompt  string
	reActSystemPrompt string
	executionPrompt   string
	summarizePrompt   string
}

var pm *PromptManager
var once sync.Once

func init() {
	once.Do(func() {
		pm = &PromptManager{
			systemPrompt:      systemPrompt,
			planSystemPrompt:  planSystemPrompt,
			planCreatePrompt:  planCreatePrompt,
			planUpdatePrompt:  planUpdatePrompt,
			reActSystemPrompt: reActSystemPrompt,
			executionPrompt:   executionPrompt,
			summarizePrompt:   summarizePrompt,
		}
	})
}

func GetSystemPrompt() string {
	pm.Lock()
	defer pm.Unlock()
	return pm.systemPrompt
}

func GetPlanSystemPrompt() string {
	pm.Lock()
	defer pm.Unlock()
	return pm.planSystemPrompt
}

func GetPlanCreatePrompt() string {
	pm.Lock()
	defer pm.Unlock()
	return pm.planCreatePrompt
}

func GetPlanUpdatePrompt() string {
	pm.Lock()
	defer pm.Unlock()
	return pm.planUpdatePrompt
}

func GetReActSystemPrompt() string {
	pm.Lock()
	defer pm.Unlock()
	return pm.reActSystemPrompt
}

func GetExecutionPrompt() string {
	pm.Lock()
	defer pm.Unlock()
	return pm.executionPrompt
}

func GetSummarizePrompt() string {
	pm.Lock()
	defer pm.Unlock()
	return pm.summarizePrompt
}
