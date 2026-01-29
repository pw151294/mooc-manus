package services

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/services"
)

type ToolProviderApplicationService interface {
	Add(dtos.AddToolProviderRequest) error
	Update(dtos.UpdateToolProviderRequest) error
	Delete(string) error
	Exist(id string) bool
	List() ([]dtos.ToolProviderDTO, error)
}

type ToolProviderApplicationServiceImpl struct {
	providerDomainSvc services.ToolProviderDomainService
}

func (t *ToolProviderApplicationServiceImpl) Exist(id string) bool {
	return t.providerDomainSvc.Exists(id)
}

func NewToolProviderApplicationService(providerDomainSvc services.ToolProviderDomainService) ToolProviderApplicationService {
	return &ToolProviderApplicationServiceImpl{providerDomainSvc: providerDomainSvc}
}

func (t *ToolProviderApplicationServiceImpl) Add(request dtos.AddToolProviderRequest) error {
	do := dtos.ConvertAddToolProviderRequest2DO(request)
	return t.providerDomainSvc.Create(do)
}

func (t *ToolProviderApplicationServiceImpl) Update(request dtos.UpdateToolProviderRequest) error {
	do := dtos.ConvertUpdateToolProviderRequest2DO(request)
	return t.providerDomainSvc.Update(do)
}

func (t *ToolProviderApplicationServiceImpl) Delete(id string) error {
	return t.providerDomainSvc.DeleteById(id)
}

func (t *ToolProviderApplicationServiceImpl) List() ([]dtos.ToolProviderDTO, error) {
	dos, err := t.providerDomainSvc.List()
	if err != nil {
		return nil, err
	}
	var dtoList []dtos.ToolProviderDTO
	for _, do := range dos {
		dtoList = append(dtoList, dtos.ConvertToolProviderDO2DTO(do))
	}
	return dtoList, nil
}
