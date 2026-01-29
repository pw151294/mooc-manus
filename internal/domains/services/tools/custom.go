package tools

import (
	"mooc-manus/internal/domains/models"
)

type CustomTool struct {
	BaseTool
}

func (c *CustomTool) Init() error {
	// todo 待实现
	panic("当前系统不支持自定义工具")
}

func NewCustomTool(provider models.ToolProviderDO, functions []models.ToolFunctionDO) Tool {
	// todo 待实现
	panic("当前系统不支持自定义工具")
}

func (c *CustomTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	//TODO 待实现
	panic("当前系统不支持自定义工具")
}
