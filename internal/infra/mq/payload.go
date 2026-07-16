package mq

import "encoding/json"

// RunInstancePayload asynq TaskTypeRunInstance 任务体
// 字段设计上保持最小化：只带 instance_id + attempt + enqueued_at；
// 具体 instance 快照 / case 快照都通过 repository 二次查询，避免 payload 过大与陈旧。
type RunInstancePayload struct {
	InstanceID string `json:"instance_id"`
	Attempt    int    `json:"attempt"`     // 第几次重试，从 0 开始（首次入队为 0）
	EnqueuedAt int64  `json:"enqueued_at"` // 入队 unix 秒；用于观测排队延迟
}

// Marshal 序列化为 asynq Task.Payload
func (p *RunInstancePayload) Marshal() ([]byte, error) { return json.Marshal(p) }

// Unmarshal 从 asynq Task.Payload 还原
func (p *RunInstancePayload) Unmarshal(b []byte) error { return json.Unmarshal(b, p) }
