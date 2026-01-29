package services

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/repositories"
)

type AppConfigDomainService interface {
	Create(models.AppConfigDO) error
	Update(models.AppConfigDO) error
	GetById(string) (models.AppConfigDO, error)
	DeleteById(string) error
	List() ([]models.AppConfigDO, error)
}

type AppConfigDomainServiceImpl struct {
	appConfigRepo repositories.AppConfigRepository
}

func NewAppConfigDomainService(appConfigRepo repositories.AppConfigRepository) AppConfigDomainService {
	return &AppConfigDomainServiceImpl{appConfigRepo: appConfigRepo}
}

func (a *AppConfigDomainServiceImpl) Create(do models.AppConfigDO) error {
	po := models.ConvertAppConfigDO2PO(do)
	return a.appConfigRepo.Create(po)
}

func (a *AppConfigDomainServiceImpl) Update(do models.AppConfigDO) error {
	po := models.ConvertAppConfigDO2PO(do)
	return a.appConfigRepo.Update(po)
}

func (a *AppConfigDomainServiceImpl) GetById(id string) (models.AppConfigDO, error) {
	po, err := a.appConfigRepo.GetById(id)
	if err != nil {
		return models.AppConfigDO{}, err
	}
	return models.ConvertAppConfigPO2DO(po), nil
}

func (a *AppConfigDomainServiceImpl) DeleteById(id string) error {
	return a.appConfigRepo.DeleteById(id)
}

func (a *AppConfigDomainServiceImpl) List() ([]models.AppConfigDO, error) {
	pos, err := a.appConfigRepo.List()
	if err != nil {
		return nil, err
	}
	var dos []models.AppConfigDO
	for _, po := range pos {
		dos = append(dos, models.ConvertAppConfigPO2DO(po))
	}
	return dos, nil
}
