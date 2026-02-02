package models

import (
	"encoding/json"
	"mooc-manus/internal/infra/models"
)

type A2AServerConfigDO struct {
	ID          string
	A2AConfigID string
	BaseURL     string
	Enabled     bool
	ExtInfo     map[string]any
}

func ConvertA2AServerConfigPO2DO(po models.A2AServerConfigPO) (A2AServerConfigDO, error) {
	var extInfo map[string]any
	if err := json.Unmarshal([]byte(po.ExtInfo), &extInfo); err != nil {
		return A2AServerConfigDO{}, err
	}
	return A2AServerConfigDO{
		ID:          po.ID,
		A2AConfigID: po.A2AConfigID,
		BaseURL:     po.BaseURL,
		Enabled:     po.Enabled,
		ExtInfo:     extInfo,
	}, nil
}

func ConvertA2AServerConfigDO2PO(do A2AServerConfigDO) (models.A2AServerConfigPO, error) {
	extInfo, err := json.Marshal(do.ExtInfo)
	if err != nil {
		return models.A2AServerConfigPO{}, err
	}
	return models.A2AServerConfigPO{
		ID:          do.ID,
		A2AConfigID: do.A2AConfigID,
		BaseURL:     do.BaseURL,
		Enabled:     do.Enabled,
		ExtInfo:     string(extInfo),
	}, nil
}
