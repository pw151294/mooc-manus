package services

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/services"
)

type AppConfigApplicationService interface {
	CreateAppConfig(dtos.AppConfigCreateRequest) (string, error)
	UpdateAppConfig(dtos.AppConfigUpdateRequest) error
	LoadAppConfig(id string) (dtos.AppConfigDTO, error)
	DeleteAppConfig(id string) error
	GetAllAppConfigs() ([]dtos.AppConfigDTO, error)
}

type AppConfigApplicationServiceImpl struct {
	appConfigDomainSvc services.AppConfigDomainService
}

func NewAppConfigApplicationService(appConfigDomainSvc services.AppConfigDomainService) AppConfigApplicationService {
	return &AppConfigApplicationServiceImpl{appConfigDomainSvc: appConfigDomainSvc}
}

func (a *AppConfigApplicationServiceImpl) CreateAppConfig(request dtos.AppConfigCreateRequest) (string, error) {
	do := dtos.ConvertAppConfigCreateRequest2DO(request)
	err := a.appConfigDomainSvc.Create(do)
	return do.AppConfigID, err
}

func (a *AppConfigApplicationServiceImpl) UpdateAppConfig(request dtos.AppConfigUpdateRequest) error {
	do := dtos.ConvertAppConfigUpdateRequest2DO(request)
	return a.appConfigDomainSvc.Update(do)
}

func (a *AppConfigApplicationServiceImpl) LoadAppConfig(id string) (dtos.AppConfigDTO, error) {
	do, err := a.appConfigDomainSvc.GetById(id)
	if err != nil {
		return dtos.AppConfigDTO{}, err
	}
	return dtos.ConvertAppConfigDO2DTO(do), nil
}

func (a *AppConfigApplicationServiceImpl) DeleteAppConfig(id string) error {
	return a.appConfigDomainSvc.DeleteById(id)
}

func (a *AppConfigApplicationServiceImpl) GetAllAppConfigs() ([]dtos.AppConfigDTO, error) {
	dos, err := a.appConfigDomainSvc.List()
	if err != nil {
		return nil, err
	}
	var dtosList []dtos.AppConfigDTO
	for _, do := range dos {
		dtosList = append(dtosList, dtos.ConvertAppConfigDO2DTO(do))
	}
	return dtosList, nil
}
