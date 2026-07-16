package evaluation

import "time"

type Case struct {
	ID           string
	Name         string
	Description  string
	InitScript   string   // 可空
	TaskPrompt   string
	VerifyScript string
	Tags         []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
