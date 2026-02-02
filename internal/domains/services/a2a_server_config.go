package services

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/repositories"
)

type A2AServerConfigDomainService interface {
	Create(config models.A2AServerConfigDO) error
	Update(config models.A2AServerConfigDO) error
	DeleteById(id string) error
	GetById(id string) (models.A2AServerConfigDO, error)
	ListByA2AConfigId(a2aConfigId string) ([]models.A2AServerConfigDO, error)
	ListByAppConfigId(appConfigId string) ([]models.A2AServerConfigDO, error)
}

type A2AServerConfigDomainServiceImpl struct {
	repo repositories.A2AServerConfigRepository
}

func NewA2AServerConfigDomainService(repo repositories.A2AServerConfigRepository) A2AServerConfigDomainService {
	return &A2AServerConfigDomainServiceImpl{repo: repo}
}

func (s *A2AServerConfigDomainServiceImpl) Create(config models.A2AServerConfigDO) error {
	po, err := models.ConvertA2AServerConfigDO2PO(config)
	if err != nil {
		return err
	}
	return s.repo.Create(po)
}

func (s *A2AServerConfigDomainServiceImpl) Update(config models.A2AServerConfigDO) error {
	po, err := models.ConvertA2AServerConfigDO2PO(config)
	if err != nil {
		return err
	}
	return s.repo.Update(po)
}

func (s *A2AServerConfigDomainServiceImpl) DeleteById(id string) error {
	return s.repo.DeleteById(id)
}

func (s *A2AServerConfigDomainServiceImpl) GetById(id string) (models.A2AServerConfigDO, error) {
	po, err := s.repo.GetById(id)
	if err != nil {
		return models.A2AServerConfigDO{}, err
	}
	return models.ConvertA2AServerConfigPO2DO(po)
}

func (s *A2AServerConfigDomainServiceImpl) ListByA2AConfigId(a2aConfigId string) ([]models.A2AServerConfigDO, error) {
	pos, err := s.repo.ListByA2AConfigId(a2aConfigId)
	if err != nil {
		return nil, err
	}
	dos := make([]models.A2AServerConfigDO, 0, len(pos))
	for _, po := range pos {
		do, err := models.ConvertA2AServerConfigPO2DO(po)
		if err != nil {
			return nil, err
		}
		dos = append(dos, do)
	}
	return dos, nil
}

func (s *A2AServerConfigDomainServiceImpl) ListByAppConfigId(appConfigId string) ([]models.A2AServerConfigDO, error) {
	pos, err := s.repo.ListByAppConfigId(appConfigId)
	if err != nil {
		return nil, err
	}
	dos := make([]models.A2AServerConfigDO, 0, len(pos))
	for _, po := range pos {
		do, err := models.ConvertA2AServerConfigPO2DO(po)
		if err != nil {
			return nil, err
		}
		dos = append(dos, do)
	}
	return dos, nil
}
