package models

import (
	infra "mooc-manus/internal/infra/models"

	"github.com/google/uuid"
)

type ToolProviderDO struct {
	ProviderID        string
	ProviderName      string
	ProviderType      string
	ProviderDesc      string
	ProviderURL       string
	ProviderTransport string
}

func ConvertToolProviderDO2PO(do ToolProviderDO) infra.ToolProviderPO {
	if do.ProviderID == "" {
		do.ProviderID = uuid.New().String()
	}
	return infra.ToolProviderPO{
		ID:                do.ProviderID,
		ProviderName:      do.ProviderName,
		ProviderType:      do.ProviderType,
		ProviderDesc:      do.ProviderDesc,
		ProviderURL:       do.ProviderURL,
		ProviderTransport: do.ProviderTransport,
	}
}

func ConvertToolProviderPO2DO(po infra.ToolProviderPO) ToolProviderDO {
	return ToolProviderDO{
		ProviderID:        po.ID,
		ProviderName:      po.ProviderName,
		ProviderType:      po.ProviderType,
		ProviderDesc:      po.ProviderDesc,
		ProviderURL:       po.ProviderURL,
		ProviderTransport: po.ProviderTransport,
	}
}
