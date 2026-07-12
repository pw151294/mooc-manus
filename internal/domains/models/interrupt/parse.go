package interrupt

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrParseJSON   = errors.New("arguments JSON 解析失败")
	ErrMissingRisk = errors.New("缺少 risk_level 字段")
	ErrInvalidRisk = errors.New("risk_level 值非法")
)

// ParseRiskFromArgs 仅解析 risk_level / risk_reason；不影响 command 原样透传。
// 返回错误时，调用方应降级为"直接执行"（不拦截、Warn 日志）。
func ParseRiskFromArgs(argsJSON string) (level, reason string, err error) {
	var m map[string]interface{}
	if e := json.Unmarshal([]byte(argsJSON), &m); e != nil {
		return "", "", fmt.Errorf("%w: %v", ErrParseJSON, e)
	}
	lv, ok := m["risk_level"].(string)
	if !ok {
		return "", "", ErrMissingRisk
	}
	if lv != "safe" && lv != "dangerous" {
		return "", "", fmt.Errorf("%w: %s", ErrInvalidRisk, lv)
	}
	r, _ := m["risk_reason"].(string) // 允许空
	return lv, r, nil
}
