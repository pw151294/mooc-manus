package tools

import (
	"encoding/json"
	"mooc-manus/internal/domains/models"
	"testing"
)

func TestMcpToolInit(t *testing.T) {
	provider := models.ToolProviderDO{
		ProviderID:        "prov-2",
		ProviderName:      "mcp-test",
		ProviderType:      "MCP",
		ProviderDesc:      "test",
		ProviderURL:       "http://127.0.0.1:27017/bing_search/mcp", // 无效端口，期望返回错误
		ProviderTransport: "streamable_http",
	}
	tool := NewMcpTool(provider, nil)

	if err := tool.Init(); err != nil {
		t.Fatalf("init tools failed with error: %v", err)
	}

	args := map[string]string{"query": "deepseek"}
	argsBytes, _ := json.Marshal(args)
	funcArgs := string(argsBytes)
	result := tool.Invoke("bing_search", funcArgs)
	content := result.Data.(string)
	t.Log(content)
}

func TestMcpToolInit_PanicOnUnsupportedTransport(t *testing.T) {
	provider := models.ToolProviderDO{
		ProviderID:        "prov-1",
		ProviderName:      "mcp-test",
		ProviderType:      "MCP",
		ProviderDesc:      "test",
		ProviderURL:       "http://localhost:12345",
		ProviderTransport: "websocket", // 非支持协议，期望 panic
	}
	tool := NewMcpTool(provider, nil)

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on unsupported transport, got no panic")
		}
	}()

	_ = tool.Init()
}

func TestMcpToolInit_ErrorOnBadURL(t *testing.T) {
	provider := models.ToolProviderDO{
		ProviderID:        "prov-2",
		ProviderName:      "mcp-test",
		ProviderType:      "MCP",
		ProviderDesc:      "test",
		ProviderURL:       "http://127.0.0.1:12345", // 无效端口，期望返回错误
		ProviderTransport: "streamable_http",
	}
	tool := NewMcpTool(provider, nil)

	if err := tool.Init(); err == nil {
		t.Fatalf("expected error when initializing with bad URL, got nil")
	}
}
