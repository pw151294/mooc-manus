package dtos

import (
	"mooc-manus/internal/domains/models"

	"github.com/google/uuid"
)

type AddToolProviderRequest struct {
	ProviderName      string `json:"providerName" binding:"required"`
	ProviderDesc      string `json:"providerDesc"`
	ProviderType      string `json:"providerType" binding:"required"`
	ProviderURL       string `json:"providerUrl"`
	ProviderTransport string `json:"providerTransport"`
}
type UpdateToolProviderRequest struct {
	ProviderID        string `json:"providerId" binding:"required"`
	ProviderName      string `json:"providerName" binding:"required"`
	ProviderDesc      string `json:"providerDesc"`
	ProviderType      string `json:"providerType" binding:"required"`
	ProviderURL       string `json:"providerUrl"`
	ProviderTransport string `json:"providerTransport"`
}
type ToolProviderDTO struct {
	ProviderID        string `json:"providerId"`
	ProviderName      string `json:"providerName"`
	ProviderDesc      string `json:"providerDesc"`
	ProviderType      string `json:"providerType"`
	ProviderURL       string `json:"providerUrl"`
	ProviderTransport string `json:"providerTransport"`
}

func ConvertAddToolProviderRequest2DO(request AddToolProviderRequest) models.ToolProviderDO {
	return models.ToolProviderDO{
		ProviderID:        uuid.New().String(),
		ProviderName:      request.ProviderName,
		ProviderType:      request.ProviderType,
		ProviderDesc:      request.ProviderDesc,
		ProviderURL:       request.ProviderURL,
		ProviderTransport: request.ProviderTransport,
	}
}

func ConvertUpdateToolProviderRequest2DO(request UpdateToolProviderRequest) models.ToolProviderDO {
	return models.ToolProviderDO{
		ProviderID:        request.ProviderID,
		ProviderName:      request.ProviderName,
		ProviderType:      request.ProviderType,
		ProviderDesc:      request.ProviderDesc,
		ProviderURL:       request.ProviderURL,
		ProviderTransport: request.ProviderTransport,
	}
}

func ConvertToolProviderDO2DTO(do models.ToolProviderDO) ToolProviderDTO {
	return ToolProviderDTO{
		ProviderID:        do.ProviderID,
		ProviderName:      do.ProviderName,
		ProviderDesc:      do.ProviderDesc,
		ProviderType:      do.ProviderType,
		ProviderURL:       do.ProviderURL,
		ProviderTransport: do.ProviderTransport,
	}
}
