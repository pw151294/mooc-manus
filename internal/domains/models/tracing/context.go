package tracing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

type ctxKey struct{}

func contextWithSpan(ctx context.Context, s *Span) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// SpanFromContext 取 ctx 里存的当前 span；无则返回 no-op（不 panic）
func SpanFromContext(ctx context.Context) *Span {
	if ctx == nil {
		return newNoopSpan()
	}
	v := ctx.Value(ctxKey{})
	if s, ok := v.(*Span); ok && s != nil {
		return s
	}
	return newNoopSpan()
}

// StartSpanFromContext 由 domain 层埋点调用；tracer 未初始化时返回 no-op
func StartSpanFromContext(ctx context.Context, spanType SpanType, opName string) (context.Context, *Span) {
	t := Global()
	if t == nil {
		return ctx, newNoopSpan()
	}
	return t.StartSpan(ctx, spanType, opName)
}

// Sha256Prefix 返回给定文本的 sha256 十六进制前缀，用于 system_prompt.hash 等
func Sha256Prefix(text string, n int) string {
	if n <= 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	hexStr := hex.EncodeToString(sum[:])
	if n > len(hexStr) {
		n = len(hexStr)
	}
	return hexStr[:n]
}
