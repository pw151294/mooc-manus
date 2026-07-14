package tracing

// newNoopSpan 用于 ctx 无 parent 或 tracer 未初始化时返回
// 所有 Span 方法可无害调用；End 因 ended=true 直接返回，不会 commit
func newNoopSpan() *Span {
	s := &Span{
		tags: map[string]interface{}{},
	}
	s.ended.Store(true) // 提前标记 ended，避免误 commit
	return s
}
