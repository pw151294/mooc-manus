// Package skillerr 提供 Skill 模块通用哨兵错误。
// 放在独立包以同时被 internal/domains/services 与 internal/applications/services 引用，
// 避免领域层反向依赖应用层造成的循环依赖。
//
// 用法：
//	Service 层：fmt.Errorf("skill not found: %w", skillerr.ErrNotFound)
//	Handler 层：errors.Is(err, skillerr.ErrNotFound) → 映射 HTTP 404
package skillerr

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrDuplicate    = errors.New("duplicate")
	ErrInvalidInput = errors.New("invalid input")
)
