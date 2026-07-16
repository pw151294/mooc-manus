package tracing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 确保 llm.io.*_units 系列 tag 不被脱敏正则误杀。
// 原计划使用 llm.usage.* 但因脱敏正则包含 "token" 关键词导致误杀，
// 根据 spec §4.2 风险清单，退化为 llm.io.*_units 命名。
func TestUsageTagNotMasked(t *testing.T) {
	s := newTestSpan("t", 1, 0, SpanTypeLLMCall, "")
	s.SetTag("llm.io.prompt_units", int64(123))
	s.SetTag("llm.io.completion_units", int64(456))
	s.SetTag("llm.io.total_units", int64(579))
	require.Equal(t, int64(123), s.tags["llm.io.prompt_units"])
	require.Equal(t, int64(456), s.tags["llm.io.completion_units"])
	require.Equal(t, int64(579), s.tags["llm.io.total_units"])
}

