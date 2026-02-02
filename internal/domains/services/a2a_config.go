package services

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/repositories"
)

type A2AConfigDomainService interface {
	Create(config models.A2AConfigDO) error
	Update(config models.A2AConfigDO) error
	DeleteById(id string) error
	GetById(id string) (models.A2AConfigDO, error)
	ListByAppConfigId(appConfigId string) ([]models.A2AConfigDO, error)
}

type A2AConfigDomainServiceImpl struct {
	repo repositories.A2AConfigRepository
}

func NewA2AConfigDomainService(repo repositories.A2AConfigRepository) A2AConfigDomainService {
	return &A2AConfigDomainServiceImpl{repo: repo}
}

func (s *A2AConfigDomainServiceImpl) Create(config models.A2AConfigDO) error {
	po, err := models.ConvertA2AConfigDO2PO(config)
	if err != nil {
		return err
	}
	return s.repo.Create(po)
}

func (s *A2AConfigDomainServiceImpl) Update(config models.A2AConfigDO) error {
	po, err := models.ConvertA2AConfigDO2PO(config)
	if err != nil {
		return err
	}
	return s.repo.Update(po)
}

func (s *A2AConfigDomainServiceImpl) DeleteById(id string) error {
	return s.repo.DeleteById(id)
}

func (s *A2AConfigDomainServiceImpl) GetById(id string) (models.A2AConfigDO, error) {
	po, err := s.repo.GetById(id)
	if err != nil {
		return models.A2AConfigDO{}, err
	}
	return models.ConvertA2AConfigPO2DO(po)
}

func (s *A2AConfigDomainServiceImpl) ListByAppConfigId(appConfigId string) ([]models.A2AConfigDO, error) {
	pos, err := s.repo.ListByAppConfigId(appConfigId)
	if err != nil {
		return nil, err
	}
	dos := make([]models.A2AConfigDO, 0, len(pos))
	for _, po := range pos {
		do, err := models.ConvertA2AConfigPO2DO(po)
		if err != nil {
			return nil, err
		}
		dos = append(dos, do)
	}
	return dos, nil
}
