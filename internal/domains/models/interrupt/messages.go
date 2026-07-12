package interrupt

const (
	MsgUserReject                = "用户拒绝执行此工具调用。"
	MsgUserRejectWithFeedbackTpl = "用户拒绝执行此工具调用。用户反馈:%s"
	MsgTimeout                   = "用户在 5 分钟内未确认此工具调用,已按拒绝处理。"
	MsgUserStop                  = "用户中止了本次对话,此工具调用未执行。"
	MsgSiblingSkipped            = "因用户拒绝了本轮的高危调用,此工具调用未执行。"
)
