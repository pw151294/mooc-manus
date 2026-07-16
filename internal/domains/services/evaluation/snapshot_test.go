package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "mooc-manus/internal/domains/models"
)

// TestFreezeAppConfig_深拷贝语义
// spec §2.5：Snapshot 冻结后必须与源 AppConfigDO 完全解耦，源侧任何修改都不得穿透到 snapshot。
func TestFreezeAppConfig_深拷贝语义(t *testing.T) {
	src := &appconfig.AppConfigDO{
		AppConfigID: "cfg-1",
		ModelConfig: appconfig.ModelConfig{
			Provider:    "openai",
			BaseUrl:     "https://api.openai.com/v1",
			ApiKey:      "sk-x",
			ModelName:   "gpt-4o",
			Temperature: 0.7,
			MaxTokens:   4096,
		},
		A2AServerConfigs: []appconfig.A2AServerConfig{
			{ID: "a1", Name: "test", Url: "http://a1", Description: "d1"},
		},
	}
	snap, err := FreezeAppConfig(src)
	require.NoError(t, err)
	require.NotNil(t, snap)

	// 深拷贝断言：修改源不影响 snapshot
	src.ModelConfig.ApiKey = "sk-y"
	src.A2AServerConfigs[0].Name = "changed"

	assert.Equal(t, "sk-x", snap.Model.ApiKey, "深拷贝后修改源 ApiKey 不应影响 snapshot")
	assert.Equal(t, "openai", snap.Model.Provider)
	assert.Equal(t, "gpt-4o", snap.Model.ModelName)
	assert.InDelta(t, 0.7, snap.Model.Temperature, 1e-9)
	assert.Equal(t, int64(4096), snap.Model.MaxTokens)

	require.Len(t, snap.A2AConfig, 1)
	assert.Equal(t, "test", snap.A2AConfig[0].Name, "深拷贝后修改源 A2A Name 不应影响 snapshot")
	assert.Equal(t, "a1", snap.A2AConfig[0].ID)
	assert.Equal(t, "http://a1", snap.A2AConfig[0].URL)
	assert.Equal(t, "d1", snap.A2AConfig[0].Description)

	assert.Equal(t, "cfg-1", snap.SourceAppConfigID)
	assert.NotEmpty(t, snap.ID)
	assert.False(t, snap.CreatedAt.IsZero(), "CreatedAt 必须被赋值")
}

// TestFreezeAppConfig_NilInput
// nil 入参应返回错误而非 panic。
func TestFreezeAppConfig_NilInput(t *testing.T) {
	_, err := FreezeAppConfig(nil)
	require.Error(t, err)
}

// TestFreezeAppConfig_空A2AList
// A2A 列表为空时应生成长度为 0 的切片而非 nil，方便调用方无脑 range。
func TestFreezeAppConfig_空A2AList(t *testing.T) {
	src := &appconfig.AppConfigDO{
		AppConfigID: "cfg-2",
		ModelConfig: appconfig.ModelConfig{ModelName: "gpt-4"},
	}
	snap, err := FreezeAppConfig(src)
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.NotNil(t, snap.A2AConfig)
	assert.Len(t, snap.A2AConfig, 0)
}
