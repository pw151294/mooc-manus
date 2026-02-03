package models

import "mooc-manus/internal/infra/models"

type A2AServerFunctionDO struct {
	ID                string
	A2AServerConfigID string
	FunctionID        string
}

func ConvertA2AServerFunctionPO2DO(po models.A2AServerFunctionPO) A2AServerFunctionDO {
	return A2AServerFunctionDO{
		ID:                po.ID,
		A2AServerConfigID: po.A2AServerConfigID,
		FunctionID:        po.FunctionID,
	}
}

func ConvertA2AServerFunctionDO2PO(do A2AServerFunctionDO) models.A2AServerFunctionPO {
	return models.A2AServerFunctionPO{
		ID:                do.ID,
		A2AServerConfigID: do.A2AServerConfigID,
		FunctionID:        do.FunctionID,
	}
}

func ConvertA2AServerFunctionDOs2POs(dos []A2AServerFunctionDO) []models.A2AServerFunctionPO {
	pos := make([]models.A2AServerFunctionPO, len(dos))
	for i, do := range dos {
		pos[i] = ConvertA2AServerFunctionDO2PO(do)
	}
	return pos
}
