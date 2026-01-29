package services

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/repositories"
)

type ToolProviderDomainService interface {
	Create(models.ToolProviderDO) error
	Update(models.ToolProviderDO) error
	GetById(string) (models.ToolProviderDO, error)
	GetByIds([]string) ([]models.ToolProviderDO, error)
	DeleteById(string) error
	Exists(string) bool
	List() ([]models.ToolProviderDO, error)
}

type ToolProviderDomainServiceImpl struct {
	providerRepo repositories.ToolProviderRepository
}

func (t *ToolProviderDomainServiceImpl) GetByIds(ids []string) ([]models.ToolProviderDO, error) {
	pos, err := t.providerRepo.GetByIds(ids)
	if err != nil {
		return nil, err
	}
	var dos []models.ToolProviderDO
	for _, po := range pos {
		dos = append(dos, models.ConvertToolProviderPO2DO(po))
	}
	return dos, nil
}

func (t *ToolProviderDomainServiceImpl) GetById(id string) (models.ToolProviderDO, error) {
	po, err := t.providerRepo.GetById(id)
	if err != nil {
		return models.ToolProviderDO{}, err
	}
	return models.ConvertToolProviderPO2DO(po), nil
}

func (t *ToolProviderDomainServiceImpl) Exists(id string) bool {
	return t.providerRepo.Exists(id)
}

func NewToolProviderDomainService(providerRepo repositories.ToolProviderRepository) ToolProviderDomainService {
	return &ToolProviderDomainServiceImpl{providerRepo: providerRepo}
}

func (t *ToolProviderDomainServiceImpl) Create(do models.ToolProviderDO) error {
	po := models.ConvertToolProviderDO2PO(do)
	return t.providerRepo.Create(po)
}

func (t *ToolProviderDomainServiceImpl) Update(do models.ToolProviderDO) error {
	po := models.ConvertToolProviderDO2PO(do)
	return t.providerRepo.Update(po)
}

func (t *ToolProviderDomainServiceImpl) DeleteById(id string) error {
	return t.providerRepo.DeleteById(id)
}

func (t *ToolProviderDomainServiceImpl) List() ([]models.ToolProviderDO, error) {
	pos, err := t.providerRepo.List()
	if err != nil {
		return nil, err
	}
	var dos []models.ToolProviderDO
	for _, po := range pos {
		dos = append(dos, models.ConvertToolProviderPO2DO(po))
	}
	return dos, nil
}
