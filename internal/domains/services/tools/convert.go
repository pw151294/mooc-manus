package tools

import (
	"context"
	"mooc-manus/internal/domains/models"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go"
)

func convertDO2Tool(do models.ToolFunctionDO) openai.ChatCompletionToolParam {
	function := openai.FunctionDefinitionParam{}
	function.Name = do.FunctionName
	function.Description = openai.String(do.FunctionDesc)
	function.Parameters = do.Schema.Parameters

	return openai.ChatCompletionToolParam{
		Function: function,
		Type:     "function",
	}
}

func ConvertListMcpTools2Functions(providerId string, listToolsResult *mcp.ListToolsResult) []models.ToolFunctionDO {
	tools := listToolsResult.Tools
	functions := make([]models.ToolFunctionDO, 0, len(tools))
	for _, tool := range tools {
		inputSchema := tool.InputSchema
		schema := models.ToolSchema{}
		schema.Name = tool.Name
		schema.Description = tool.Description
		params := make(map[string]any)
		params["type"] = inputSchema.Type
		params["properties"] = inputSchema.Properties
		params["required"] = inputSchema.Required
		schema.Parameters = params

		function := models.ToolFunctionDO{}
		function.FunctionID = uuid.New().String()
		function.ProviderID = providerId
		function.FunctionName = tool.Name
		function.FunctionDesc = tool.Description
		function.Schema = schema
		functions = append(functions, function)
	}

	return functions
}

func ConvertMcpProvider2Functions(provider models.ToolProviderDO) ([]models.ToolFunctionDO, error) {
	cli, err := client.NewStreamableHttpClient(provider.ProviderURL)
	if err != nil {
		return nil, err
	}
	timeoutCtx, cancelFunc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFunc()
	if err := cli.Start(timeoutCtx); err != nil {
		return nil, err
	}

	// 初始化MCP
	clientInfo := mcp.Implementation{}
	clientInfo.Name = provider.ProviderName
	clientInfo.Version = "1.0"
	params := mcp.InitializeParams{}
	params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	params.ClientInfo = clientInfo
	initReq := mcp.InitializeRequest{Params: params}
	if _, err := cli.Initialize(timeoutCtx, initReq); err != nil {
		return nil, err
	}

	// 解析mcp工具
	listToolsResult, err := cli.ListTools(timeoutCtx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	functions := ConvertListMcpTools2Functions(provider.ProviderID, listToolsResult)
	return functions, nil
}
