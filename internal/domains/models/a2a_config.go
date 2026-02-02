package models

import (
	"encoding/json"
	"mooc-manus/internal/infra/models"
)

type A2AConfigDO struct {
	ID          string
	AppConfigID string
	ExtInfo     map[string]any
}

func ConvertA2AConfigPO2DO(po models.A2AConfigPO) (A2AConfigDO, error) {
	var extInfo map[string]any
	if err := json.Unmarshal([]byte(po.ExtInfo), &extInfo); err != nil {
		return A2AConfigDO{}, err
	}
	return A2AConfigDO{
		ID:          po.ID,
		AppConfigID: po.AppConfigID,
		ExtInfo:     extInfo,
	}, nil
}

func ConvertA2AConfigDO2PO(do A2AConfigDO) (models.A2AConfigPO, error) {
	extInfo, err := json.Marshal(do.ExtInfo)
	if err != nil {
		return models.A2AConfigPO{}, err
	}
	return models.A2AConfigPO{
		ID:          do.ID,
		AppConfigID: do.AppConfigID,
		ExtInfo:     string(extInfo),
	}, nil
}
