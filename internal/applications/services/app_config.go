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
	CreateA2AServers(dtos.CreateA2AServersRequest) error
	UpdateA2AServers(dtos.UpdateA2AServersRequest) error
	DeleteA2AServers(request dtos.DeleteA2AServersRequest) error
	GetA2AServers(appConfigId string) ([]dtos.A2AServerConfigDTO, error)
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

func (a *AppConfigApplicationServiceImpl) CreateA2AServers(request dtos.CreateA2AServersRequest) error {
	appConfig := dtos.ConvertCreateA2AServersRequest2AppConfigDO(request)
	return a.appConfigDomainSvc.CreateA2AServers(appConfig)
}

func (a *AppConfigApplicationServiceImpl) UpdateA2AServers(request dtos.UpdateA2AServersRequest) error {
	appConfig := dtos.ConvertUpdateA2ARequest2AppConfigDO(request)
	return a.appConfigDomainSvc.UpdateA2AServers(appConfig)
}

func (a *AppConfigApplicationServiceImpl) DeleteA2AServers(request dtos.DeleteA2AServersRequest) error {
	return a.appConfigDomainSvc.DeleteA2AServers(request.A2AServerConfigIds)
}

func (a *AppConfigApplicationServiceImpl) GetA2AServers(appConfigId string) ([]dtos.A2AServerConfigDTO, error) {
	dos, err := a.appConfigDomainSvc.GetA2AServers(appConfigId)
	if err != nil {
		return nil, err
	}
	dtosList := make([]dtos.A2AServerConfigDTO, 0, len(dos))
	for _, do := range dos {
		dtosList = append(dtosList, dtos.ConvertA2AServerConfigDO2DTO(do))
	}
	return dtosList, nil
}
