package circuitbreaker

import (
	"fmt"
	"sort"
	"strings"
)

// BuildInterventionPrompt 生成干预提示
func BuildInterventionPrompt(records []FailureRecord) string {
	sort.Slice(records, func(i, j int) bool {
		return records[i].FailCount > records[j].FailCount
	})

	var builder strings.Builder
	builder.WriteString("⚠️ **系统干预提示**：以下工具调用已连续失败达到上限，禁止继续重试相同参数：\n\n")

	maxDisplay := 10
	if len(records) > maxDisplay {
		builder.WriteString(fmt.Sprintf("【注意：共 %d 个工具失败，仅展示前 %d 个】\n\n",
			len(records), maxDisplay))
		records = records[:maxDisplay]
	}

	for i, r := range records {
		builder.WriteString(fmt.Sprintf("%d. **%s** - 已失败 %d 次\n",
			i+1, r.ToolName, r.FailCount))
		preview := r.ParamsPreview
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		builder.WriteString(fmt.Sprintf("   参数预览：`%s`\n\n", preview))
	}

	builder.WriteString("**请选择以下操作之一**：\n\n")
	builder.WriteString("1. **向用户反馈失败原因**，说明当前子任务无法完成，结束本轮对话\n")
	builder.WriteString("2. **重新规划任务**，基于已有信息更换工具或修改参数后继续\n")
	builder.WriteString("3. **明确告知用户需要人工介入**（如文件不存在、权限不足等环境问题）\n\n")
	builder.WriteString("❌ **严禁**：继续调用上述列出的工具+参数组合\n")

	return builder.String()
}
