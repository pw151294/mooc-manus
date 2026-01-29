package services

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/repositories"

	"github.com/google/uuid"
)

type ToolFunctionDomainService interface {
	AddMcpFunctions(models.ToolProviderDO) error
	Create(models.ToolFunctionDO) error
	Update(models.ToolFunctionDO) error
	DeleteById(string) error
	GetByIds([]string) ([]models.ToolFunctionDO, error)
	GetId2Provider([]string) (map[string]models.ToolProviderDO, error)
	ListByProviderId(string) ([]models.ToolFunctionDO, error)
	List() ([]models.ToolFunctionDO, error)
}

type ToolFunctionDomainServiceImpl struct {
	functionRepo repositories.ToolFunctionRepository
	providerRepo repositories.ToolProviderRepository
}

func NewToolFunctionDomainService(
	functionRepo repositories.ToolFunctionRepository,
	providerRepo repositories.ToolProviderRepository,
) ToolFunctionDomainService {
	return &ToolFunctionDomainServiceImpl{
		functionRepo: functionRepo,
		providerRepo: providerRepo,
	}
}

// AddMcpFunctions 新增MCP
func (t *ToolFunctionDomainServiceImpl) AddMcpFunctions(provider models.ToolProviderDO) error {
	return t.functionRepo.Transaction(func(txFuncRepo repositories.ToolFunctionRepository, txProviderRepo repositories.ToolProviderRepository) error {
		if provider.ProviderID == "" {
			provider.ProviderID = uuid.New().String()
			if err := txProviderRepo.Create(models.ConvertToolProviderDO2PO(provider)); err != nil {
				return err
			}
		} else {
			if err := txProviderRepo.Update(models.ConvertToolProviderDO2PO(provider)); err != nil {
				return err
			}
			funcPos, err := txFuncRepo.ListByProviderId(provider.ProviderID)
			if err != nil {
				return err
			}
			funcIds := make([]string, 0, len(funcPos))
			for _, po := range funcPos {
				funcIds = append(funcIds, po.ID)
			}
			if len(funcIds) > 0 {
				if err := txFuncRepo.DeleteByIds(funcIds); err != nil {
					return err
				}
			}
		}

		// Batch Create Functions
		functions, err := tools.ConvertMcpProvider2Functions(provider)
		if err != nil {
			return err
		}
		if len(functions) > 0 {
			pos, err := models.ConvertToolFunctionDo2POs(functions)
			if err != nil {
				return err
			}
			return txFuncRepo.BatchCreate(pos)
		}
		return nil
	})
}

func (t *ToolFunctionDomainServiceImpl) GetId2Provider(functionIds []string) (map[string]models.ToolProviderDO, error) {
	// 1. Get tools functions
	functions, err := t.functionRepo.GetByIds(functionIds)
	if err != nil {
		return nil, err
	}

	if len(functions) == 0 {
		return make(map[string]models.ToolProviderDO), nil
	}

	// 2. Extract unique provider IDs
	providerIdSet := make(map[string]struct{})
	for _, f := range functions {
		providerIdSet[f.ProviderID] = struct{}{}
	}

	providerIds := make([]string, 0, len(providerIdSet))
	for id := range providerIdSet {
		providerIds = append(providerIds, id)
	}

	// 3. Get providers
	providers, err := t.providerRepo.GetByIds(providerIds)
	if err != nil {
		return nil, err
	}

	// 4. Map provider ID to Provider DO
	providerMap := make(map[string]models.ToolProviderDO)
	for _, p := range providers {
		providerMap[p.ID] = models.ConvertToolProviderPO2DO(p)
	}

	// 5. Build result: FunctionID -> ProviderDO
	result := make(map[string]models.ToolProviderDO)
	for _, f := range functions {
		if provider, ok := providerMap[f.ProviderID]; ok {
			result[f.ID] = provider
		}
	}

	return result, nil
}

func (t *ToolFunctionDomainServiceImpl) GetByIds(ids []string) ([]models.ToolFunctionDO, error) {
	pos, err := t.functionRepo.GetByIds(ids)
	if err != nil {
		return nil, err
	}
	var dos []models.ToolFunctionDO
	for _, po := range pos {
		do, err := models.ConvertToolFunctionPO2DO(po)
		if err != nil {
			return nil, err
		}
		dos = append(dos, do)
	}
	return dos, nil
}

func (t *ToolFunctionDomainServiceImpl) Create(do models.ToolFunctionDO) error {
	po, err := models.ConvertToolFunctionDO2PO(do)
	if err != nil {
		return err
	}
	return t.functionRepo.Create(po)
}

func (t *ToolFunctionDomainServiceImpl) Update(do models.ToolFunctionDO) error {
	po, err := models.ConvertToolFunctionDO2PO(do)
	if err != nil {
		return err
	}
	return t.functionRepo.Update(po)
}

func (t *ToolFunctionDomainServiceImpl) DeleteById(id string) error {
	return t.functionRepo.DeleteById(id)
}

func (t *ToolFunctionDomainServiceImpl) ListByProviderId(providerId string) ([]models.ToolFunctionDO, error) {
	pos, err := t.functionRepo.ListByProviderId(providerId)
	if err != nil {
		return nil, err
	}
	var dos []models.ToolFunctionDO
	for _, po := range pos {
		do, err := models.ConvertToolFunctionPO2DO(po)
		if err != nil {
			return nil, err
		}
		dos = append(dos, do)
	}
	return dos, nil
}

func (t *ToolFunctionDomainServiceImpl) List() ([]models.ToolFunctionDO, error) {
	pos, err := t.functionRepo.List()
	if err != nil {
		return nil, err
	}
	var dos []models.ToolFunctionDO
	for _, po := range pos {
		do, err := models.ConvertToolFunctionPO2DO(po)
		if err != nil {
			return nil, err
		}
		dos = append(dos, do)
	}
	return dos, nil
}
