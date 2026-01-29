package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"mooc-manus/internal/domains/models"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type McpTool struct {
	BaseTool
	mcpTransport string
	mcpURL       string
	mcpCli       client.MCPClient
}

func NewMcpTool(provider models.ToolProviderDO, functions []models.ToolFunctionDO) Tool {
	mcpTool := &McpTool{}
	mcpTool.providerId = provider.ProviderID
	mcpTool.providerType = "MCP"
	mcpTool.providerName = provider.ProviderName
	mcpTool.mcpTransport = provider.ProviderTransport
	mcpTool.mcpURL = provider.ProviderURL
	mcpTool.functions = functions
	return mcpTool
}

func (t *McpTool) Init() error {
	if t.mcpTransport != "streamable_http" {
		// 目前默认只有一种streamable-http的transport类型
		panic(fmt.Sprintf("本系统的MCP不支持%s通信协议", t.mcpTransport))
	}
	cli, err := client.NewStreamableHttpClient(t.mcpURL)
	if err != nil {
		return err
	}
	timeoutCtx, cancelFunc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFunc()

	if err := cli.Start(timeoutCtx); err != nil {
		return err
	}
	//初始化MCP
	clientInfo := mcp.Implementation{
		Name:    t.providerName,
		Version: "1.0.0",
	}
	params := mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      clientInfo,
	}
	initReq := mcp.InitializeRequest{
		Params: params,
	}
	if _, err := cli.Initialize(timeoutCtx, initReq); err != nil {
		return err
	}

	t.mcpCli = cli
	return nil
}

func (t *McpTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	result := models.ToolCallResult{}
	arguments := make(map[string]any)
	if err := json.Unmarshal([]byte(funcArgs), &arguments); err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("工具参数反序列化失败，不符合格式：%s", err.Error())
		return result
	}
	timeoutCtx, cancelFunc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFunc()

	request := mcp.CallToolRequest{}
	request.Params.Name = funcName
	request.Params.Arguments = arguments
	mcpResult, err := t.mcpCli.CallTool(timeoutCtx, request)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("调用工具%s失败：%s", funcName, err.Error())
		return result
	}
	result.Success = true
	contents := mcpResult.Content
	if len(contents) > 0 {
		result.Data = contents[0].(mcp.TextContent).Text
	}
	return result
}
