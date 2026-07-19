package evaluation

import (
	"os"
	"testing"

	"mooc-manus/config"
	"mooc-manus/pkg/logger"
)

// TestMain 初始化全局 logger，让 executor / service 内部 logger.Info/Warn/Error 不再 panic。
// 与 tracing / sse / tools / applications 包的 TestMain 保持同一模式。
func TestMain(m *testing.M) {
	tmpLogDir, _ := os.MkdirTemp("", "evaluation-test-log-*")
	_ = logger.InitGlobalLogger(config.LoggerConfig{
		Level:  "info",
		Format: "console",
		Output: "stdout",
		LogDir: tmpLogDir,
	})
	code := m.Run()
	_ = os.RemoveAll(tmpLogDir)
	os.Exit(code)
}
