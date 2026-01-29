package services

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/services"
)

type ToolFunctionApplicationService interface {
	Add(dtos.AddToolFunctionRequest) error
	AddMcpFunctions(dtos.AddMcpFunctionsRequest) error
	Update(dtos.UpdateToolFunctionRequest) error
	DeleteById(string) error
	List() ([]dtos.ToolFunctionDTO, error)
	ListByProviderId(string) ([]dtos.ToolFunctionDTO, error)
}

type ToolFunctionApplicationServiceImpl struct {
	functionDomainSvc services.ToolFunctionDomainService
}

func NewTooLFunctionApplicationService(functionDomainSvc services.ToolFunctionDomainService) ToolFunctionApplicationService {
	return &ToolFunctionApplicationServiceImpl{
		functionDomainSvc: functionDomainSvc,
	}
}

func (t *ToolFunctionApplicationServiceImpl) AddMcpFunctions(request dtos.AddMcpFunctionsRequest) error {
	provider := dtos.ConvertAddMcpFunctionsRequest2ProviderDO(request)
	return t.functionDomainSvc.AddMcpFunctions(provider)
}

func (t *ToolFunctionApplicationServiceImpl) Add(request dtos.AddToolFunctionRequest) error {
	do := dtos.ConvertAddToolFunctionRequest2DO(request)
	return t.functionDomainSvc.Create(do)
}

func (t *ToolFunctionApplicationServiceImpl) Update(request dtos.UpdateToolFunctionRequest) error {
	do := dtos.ConvertUpdateToolFuncRequest2DO(request)
	return t.functionDomainSvc.Update(do)
}

func (t *ToolFunctionApplicationServiceImpl) DeleteById(id string) error {
	return t.functionDomainSvc.DeleteById(id)
}

func (t *ToolFunctionApplicationServiceImpl) List() ([]dtos.ToolFunctionDTO, error) {
	dos, err := t.functionDomainSvc.List()
	if err != nil {
		return nil, err
	}
	var dtoList []dtos.ToolFunctionDTO
	for _, do := range dos {
		dto, err := dtos.ConvertToolFunctionDO2DTO(do)
		if err != nil {
			return nil, err
		}
		dtoList = append(dtoList, dto)
	}
	return dtoList, nil
}

func (t *ToolFunctionApplicationServiceImpl) ListByProviderId(providerId string) ([]dtos.ToolFunctionDTO, error) {
	dos, err := t.functionDomainSvc.ListByProviderId(providerId)
	if err != nil {
		return nil, err
	}
	var dtoList []dtos.ToolFunctionDTO
	for _, do := range dos {
		dto, err := dtos.ConvertToolFunctionDO2DTO(do)
		if err != nil {
			return nil, err
		}
		dtoList = append(dtoList, dto)
	}
	return dtoList, nil
}
