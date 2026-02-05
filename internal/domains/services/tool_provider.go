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
	GetByFunctionAndProviderIds(functionIds []string, providerIds []string) ([]models.ToolProviderDO, error)
	DeleteById(string) error
	Exists(string) bool
	List() ([]models.ToolProviderDO, error)
}

type ToolProviderDomainServiceImpl struct {
	providerRepo repositories.ToolProviderRepository
	functionRepo repositories.ToolFunctionRepository
}

func NewToolProviderDomainService(providerRepo repositories.ToolProviderRepository, functionRepo repositories.ToolFunctionRepository) ToolProviderDomainService {
	return &ToolProviderDomainServiceImpl{
		providerRepo: providerRepo,
		functionRepo: functionRepo,
	}
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

func (t *ToolProviderDomainServiceImpl) GetByFunctionAndProviderIds(functionIds []string, providerIds []string) ([]models.ToolProviderDO, error) {
	providerIdMap := make(map[string]struct{})
	for _, pid := range providerIds {
		providerIdMap[pid] = struct{}{}
	}

	if len(functionIds) > 0 {
		// 1. 根据 functionIds 查询出 functions
		functionPOs, err := t.functionRepo.GetByIds(functionIds)
		if err != nil {
			return nil, err
		}

		// 2. 提取出不重复的 providerId
		for _, po := range functionPOs {
			providerIdMap[po.ProviderID] = struct{}{}
		}
	}

	if len(providerIdMap) == 0 {
		return []models.ToolProviderDO{}, nil
	}

	allProviderIds := make([]string, 0, len(providerIdMap))
	for id := range providerIdMap {
		allProviderIds = append(allProviderIds, id)
	}

	// 3. 根据 providerIds 查询出 providers
	return t.GetByIds(allProviderIds)
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

func (t *ToolProviderDomainServiceImpl) Create(do models.ToolProviderDO) error {
	po := models.ConvertToolProviderDO2PO(do)
	return t.providerRepo.Create(po)
}

func (t *ToolProviderDomainServiceImpl) Update(do models.ToolProviderDO) error {
	po := models.ConvertToolProviderDO2PO(do)
	return t.providerRepo.Update(po)
}

func (t *ToolProviderDomainServiceImpl) DeleteById(id string) error {
	return t.functionRepo.Transaction(func(functionRepo repositories.ToolFunctionRepository, providerRepo repositories.ToolProviderRepository) error {
		if err := t.providerRepo.DeleteById(id); err != nil {
			return err
		}
		if err := t.functionRepo.DeleteByProviderId(id); err != nil {
			return err
		}
		return nil
	})
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
