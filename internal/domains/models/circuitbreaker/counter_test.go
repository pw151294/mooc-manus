package circuitbreaker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// UT-01 相同 path 生成相同 Key，不同 path 生成不同 Key
func TestGenerateKey_FileRead(t *testing.T) {
	key1, err := GenerateKey("fileRead", `{"path":"/tmp/a.txt"}`)
	assert.NoError(t, err)
	key2, err := GenerateKey("fileRead", `{"path":"/tmp/a.txt"}`)
	assert.NoError(t, err)
	assert.Equal(t, key1, key2, "相同 path 应生成相同 Key")

	key3, err := GenerateKey("fileRead", `{"path":"/tmp/b.txt"}`)
	assert.NoError(t, err)
	assert.NotEqual(t, key1, key3, "不同 path 应生成不同 Key")
}

// UT-02 fileEdit：old_string 前 100 字符相同即相同 Key；path 不同则不同 Key
func TestGenerateKey_FileEdit(t *testing.T) {
	oldStr1 := strings.Repeat("a", 100) + "different"
	oldStr2 := strings.Repeat("a", 100) + "other"

	args1 := fmt.Sprintf(`{"path":"/tmp/x.txt","old_string":%q,"new_string":"new1"}`, oldStr1)
	args2 := fmt.Sprintf(`{"path":"/tmp/x.txt","old_string":%q,"new_string":"new1"}`, oldStr2)

	key1, err := GenerateKey("fileEdit", args1)
	assert.NoError(t, err)
	key2, err := GenerateKey("fileEdit", args2)
	assert.NoError(t, err)
	assert.Equal(t, key1, key2, "old_string 前 100 字符相同应生成相同 Key")

	args3 := fmt.Sprintf(`{"path":"/tmp/y.txt","old_string":%q,"new_string":"new1"}`, oldStr1)
	key3, err := GenerateKey("fileEdit", args3)
	assert.NoError(t, err)
	assert.NotEqual(t, key1, key3, "不同 path 应生成不同 Key")
}

// UT-03 bashExec：完整 command 哈希，多一个空格即不同 Key
func TestGenerateKey_BashExec(t *testing.T) {
	key1, err := GenerateKey("bashExec", `{"command":"ls -la"}`)
	assert.NoError(t, err)
	key2, err := GenerateKey("bashExec", `{"command":"ls -la"}`)
	assert.NoError(t, err)
	assert.Equal(t, key1, key2, "相同 command 应生成相同 Key")

	key3, err := GenerateKey("bashExec", `{"command":"ls  -la"}`)
	assert.NoError(t, err)
	assert.NotEqual(t, key1, key3, "command 多一个空格应生成不同 Key")
}

// UT-04 未知工具：完整参数哈希，任一字段变化即不同 Key
func TestGenerateKey_UnknownTool(t *testing.T) {
	key1, err := GenerateKey("customTool", `{"foo":"bar","n":1}`)
	assert.NoError(t, err)
	key2, err := GenerateKey("customTool", `{"foo":"bar","n":1}`)
	assert.NoError(t, err)
	assert.Equal(t, key1, key2, "相同参数应生成相同 Key")

	key3, err := GenerateKey("customTool", `{"foo":"baz","n":1}`)
	assert.NoError(t, err)
	assert.NotEqual(t, key1, key3, "字段值变化应生成不同 Key")

	key4, err := GenerateKey("customTool", `{"foo":"bar","n":2}`)
	assert.NoError(t, err)
	assert.NotEqual(t, key1, key4, "另一字段变化应生成不同 Key")
}

// UT-05 非法 JSON：返回 error，且 message 包含"解析参数失败"
func TestGenerateKey_InvalidJSON(t *testing.T) {
	_, err := GenerateKey("fileRead", `{not-json`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "解析参数失败")
}

// UT-06 连续三次调用返回 1/2/3
func TestRecordFailure_NormalIncrement(t *testing.T) {
	counter := NewToolCallCounter()
	meta := ToolCallMetadata{ToolName: "fileRead", ParamsPreview: "path=/tmp/a.txt"}

	assert.Equal(t, 1, counter.RecordFailure("keyA", meta))
	assert.Equal(t, 2, counter.RecordFailure("keyA", meta))
	assert.Equal(t, 3, counter.RecordFailure("keyA", meta))
}

// UT-07 不同 Key 独立计数
func TestRecordFailure_IndependentKeys(t *testing.T) {
	counter := NewToolCallCounter()
	metaA := ToolCallMetadata{ToolName: "fileRead", ParamsPreview: "path=/tmp/a.txt"}
	metaB := ToolCallMetadata{ToolName: "bashExec", ParamsPreview: "command=ls"}

	counter.RecordFailure("keyA", metaA)
	counter.RecordFailure("keyA", metaA)
	counter.RecordFailure("keyB", metaB)

	assert.Equal(t, 2, counter.failureCounts["keyA"])
	assert.Equal(t, 1, counter.failureCounts["keyB"])
}

// UT-08 StartNewRound 清零未在本轮出现的 Key
func TestStartNewRound_ClearUnusedKeys(t *testing.T) {
	counter := NewToolCallCounter()
	meta := ToolCallMetadata{ToolName: "fileRead", ParamsPreview: "path=/tmp/a.txt"}

	counter.RecordFailure("keyA", meta)
	counter.RecordFailure("keyA", meta)
	counter.RecordFailure("keyA", meta)
	assert.Equal(t, 3, counter.failureCounts["keyA"])

	counter.StartNewRound([]string{"keyB"})

	assert.Equal(t, 0, counter.failureCounts["keyA"], "上轮未出现的 keyA 应被清零")
	records := counter.GetTriggeredRecords(3)
	assert.Empty(t, records, "清零后不应触发任何记录")
}

// UT-09 StartNewRound 保留本轮再次出现的 Key
func TestStartNewRound_KeepRepeatedKeys(t *testing.T) {
	counter := NewToolCallCounter()
	meta := ToolCallMetadata{ToolName: "fileRead", ParamsPreview: "path=/tmp/a.txt"}

	counter.RecordFailure("keyA", meta)
	counter.RecordFailure("keyA", meta)
	counter.RecordFailure("keyA", meta)
	assert.Equal(t, 3, counter.failureCounts["keyA"])

	counter.StartNewRound([]string{"keyA"})

	assert.Equal(t, 3, counter.failureCounts["keyA"], "本轮再次出现的 keyA 应保留计数")
}

// UT-10 GetTriggeredRecords 按阈值过滤
func TestGetTriggeredRecords_Threshold(t *testing.T) {
	counter := NewToolCallCounter()
	meta2 := ToolCallMetadata{ToolName: "tool2", ParamsPreview: "p=2"}
	meta3 := ToolCallMetadata{ToolName: "tool3", ParamsPreview: "p=3"}
	meta4 := ToolCallMetadata{ToolName: "tool4", ParamsPreview: "p=4"}

	// key2 失败 2 次
	counter.RecordFailure("key2", meta2)
	counter.RecordFailure("key2", meta2)
	// key3 失败 3 次
	counter.RecordFailure("key3", meta3)
	counter.RecordFailure("key3", meta3)
	counter.RecordFailure("key3", meta3)
	// key4 失败 4 次
	counter.RecordFailure("key4", meta4)
	counter.RecordFailure("key4", meta4)
	counter.RecordFailure("key4", meta4)
	counter.RecordFailure("key4", meta4)

	records := counter.GetTriggeredRecords(3)
	assert.Len(t, records, 2, "阈值 3 应返回 2 条记录（key3 / key4）")
}

// UT-11 GetTriggeredRecords 多 Key 场景
func TestGetTriggeredRecords_MultipleKeys(t *testing.T) {
	counter := NewToolCallCounter()
	metaA := ToolCallMetadata{ToolName: "fileRead", ParamsPreview: "path=/tmp/a.txt"}
	metaB := ToolCallMetadata{ToolName: "bashExec", ParamsPreview: "command=ls"}
	metaC := ToolCallMetadata{ToolName: "fileWrite", ParamsPreview: "path=/tmp/c.txt"}

	// keyA 失败 3 次
	counter.RecordFailure("keyA", metaA)
	counter.RecordFailure("keyA", metaA)
	counter.RecordFailure("keyA", metaA)
	// keyB 失败 4 次
	counter.RecordFailure("keyB", metaB)
	counter.RecordFailure("keyB", metaB)
	counter.RecordFailure("keyB", metaB)
	counter.RecordFailure("keyB", metaB)
	// keyC 失败 2 次
	counter.RecordFailure("keyC", metaC)
	counter.RecordFailure("keyC", metaC)

	records := counter.GetTriggeredRecords(3)
	assert.Len(t, records, 2)

	toolNames := make([]string, 0, len(records))
	for _, r := range records {
		toolNames = append(toolNames, r.ToolName)
	}
	assert.Contains(t, toolNames, "fileRead")
	assert.Contains(t, toolNames, "bashExec")
}

// UT-12 BuildInterventionPrompt 单条记录
func TestBuildInterventionPrompt_SingleRecord(t *testing.T) {
	records := []FailureRecord{
		{ToolName: "fileRead", ParamsPreview: "path=/test.txt", FailCount: 3},
	}
	prompt := BuildInterventionPrompt(records)

	assert.Contains(t, prompt, "系统干预提示")
	assert.Contains(t, prompt, "fileRead")
	assert.Contains(t, prompt, "已失败 3 次")
	assert.Contains(t, prompt, "path=/test.txt")
	assert.Contains(t, prompt, "向用户反馈失败原因")
	assert.Contains(t, prompt, "严禁")
}

// UT-13 BuildInterventionPrompt 超过 10 条时截断
func TestBuildInterventionPrompt_TruncateTo10(t *testing.T) {
	records := make([]FailureRecord, 0, 15)
	for i := 0; i < 15; i++ {
		records = append(records, FailureRecord{
			ToolName:      fmt.Sprintf("tool%d", i),
			ParamsPreview: fmt.Sprintf("p=%d", i),
			FailCount:     i + 1,
		})
	}

	prompt := BuildInterventionPrompt(records)

	assert.Contains(t, prompt, "共 15 个工具失败")
	assert.Contains(t, prompt, "仅展示前 10 个")
	// 按 FailCount 降序，最高的 tool14 (15 次) / tool13 (14 次) 应保留
	assert.Contains(t, prompt, "tool14")
	assert.Contains(t, prompt, "tool13")
	// FailCount 最小的 tool0 (1 次) 应被截断
	assert.NotContains(t, prompt, "tool0")
}

// UT-14 GenerateKey 关键字段缺失时回落到完整参数哈希
func TestGenerateKey_FallbackWhenFieldMissing(t *testing.T) {
	// fileRead 缺 path
	k1, err := GenerateKey("fileRead", `{"other":"x"}`)
	assert.NoError(t, err)
	k2, err := GenerateKey("fileRead", `{"other":"y"}`)
	assert.NoError(t, err)
	assert.NotEqual(t, k1, k2, "fileRead 无 path 时应按完整参数哈希，字段差异应产生不同 Key")

	// fileEdit 缺 path
	k3, err := GenerateKey("fileEdit", `{"old_string":"a","new_string":"b"}`)
	assert.NoError(t, err)
	k4, err := GenerateKey("fileEdit", `{"old_string":"a","new_string":"c"}`)
	assert.NoError(t, err)
	assert.NotEqual(t, k3, k4, "fileEdit 无 path 时应按完整参数哈希")

	// bashExec 缺 command
	k5, err := GenerateKey("bashExec", `{"foo":"bar"}`)
	assert.NoError(t, err)
	k6, err := GenerateKey("bashExec", `{"foo":"baz"}`)
	assert.NoError(t, err)
	assert.NotEqual(t, k5, k6, "bashExec 无 command 时应按完整参数哈希")
}

// UT-15 RecordFailure 计数上限为 1000
func TestRecordFailure_CapAt1000(t *testing.T) {
	counter := NewToolCallCounter()
	meta := ToolCallMetadata{ToolName: "fileRead", ParamsPreview: "path=/tmp/a.txt"}

	// 直接把内部计数抬到临界值，避免调用 1000+ 次
	counter.failureCounts["keyA"] = 999
	assert.Equal(t, 1000, counter.RecordFailure("keyA", meta))
	// 再调一次应仍返回 1000
	assert.Equal(t, 1000, counter.RecordFailure("keyA", meta))
	assert.Equal(t, 1000, counter.failureCounts["keyA"])
}

// UT-16 GenerateParamsPreview 覆盖各类型工具
func TestGenerateParamsPreview(t *testing.T) {
	// fileRead / fileWrite / fileEdit：取 path
	assert.Equal(t, "path=/tmp/a.txt",
		GenerateParamsPreview("fileRead", `{"path":"/tmp/a.txt"}`))
	assert.Equal(t, "path=/tmp/b.txt",
		GenerateParamsPreview("fileWrite", `{"path":"/tmp/b.txt"}`))
	assert.Equal(t, "path=/tmp/c.txt",
		GenerateParamsPreview("fileEdit", `{"path":"/tmp/c.txt","old_string":"x"}`))

	// bashExec：取 command 前 80 字符
	assert.Equal(t, "command=ls -la",
		GenerateParamsPreview("bashExec", `{"command":"ls -la"}`))
	longCmd := strings.Repeat("a", 100)
	preview := GenerateParamsPreview("bashExec", fmt.Sprintf(`{"command":%q}`, longCmd))
	assert.Equal(t, "command="+strings.Repeat("a", 80), preview, "过长 command 应截断到 80 字符")

	// 未知工具：截断整个 argsJSON 到 100
	longArgs := `{"foo":"` + strings.Repeat("z", 200) + `"}`
	previewUnknown := GenerateParamsPreview("customTool", longArgs)
	assert.Len(t, previewUnknown, 100)

	// 非法 JSON：走 fallback，截断原字符串
	badJSON := `{not-json`
	assert.Equal(t, badJSON, GenerateParamsPreview("fileRead", badJSON))

	// 关键工具但缺字段：走 default 分支，截断整个 argsJSON
	missing := `{"other":"x"}`
	assert.Equal(t, missing, GenerateParamsPreview("fileRead", missing))
}
