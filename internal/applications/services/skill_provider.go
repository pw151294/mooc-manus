package services

import (
	"mooc-manus/internal/applications/dtos"
	domainSvc "mooc-manus/internal/domains/services"
)

type SkillProviderApplicationService interface {
	ImportGit(req dtos.ImportGitRequest) (dtos.SkillProviderInfo, error)
	ImportZipLegacy(req dtos.ImportZipLegacyRequest) (dtos.SkillProviderInfo, error)
	Sync(req dtos.ProviderSyncRequest) (dtos.SkillProviderInfo, error)
	Delete(req dtos.ProviderDeleteRequest) error
	List(req dtos.ProviderListRequest) ([]dtos.SkillProviderInfo, error)
	Detail(req dtos.ProviderDetailRequest) (dtos.SkillProviderInfo, error)
}

type SkillProviderApplicationServiceImpl struct {
	providerDomainSvc domainSvc.SkillProviderDomainService
}

func NewSkillProviderApplicationService(providerDomainSvc domainSvc.SkillProviderDomainService) SkillProviderApplicationService {
	return &SkillProviderApplicationServiceImpl{providerDomainSvc: providerDomainSvc}
}

func (s *SkillProviderApplicationServiceImpl) ImportGit(req dtos.ImportGitRequest) (dtos.SkillProviderInfo, error) {
	do := dtos.ConvertImportGitRequest2DO(req)
	id, err := s.providerDomainSvc.Create(do)
	if err != nil {
		return dtos.SkillProviderInfo{}, err
	}
	do.SkillProviderID = id
	return dtos.ConvertSkillProviderDO2Info(do, 0), nil
}

func (s *SkillProviderApplicationServiceImpl) ImportZipLegacy(req dtos.ImportZipLegacyRequest) (dtos.SkillProviderInfo, error) {
	do := dtos.ConvertImportZipLegacyRequest2DO(req)
	id, err := s.providerDomainSvc.Create(do)
	if err != nil {
		return dtos.SkillProviderInfo{}, err
	}
	do.SkillProviderID = id
	return dtos.ConvertSkillProviderDO2Info(do, 0), nil
}

func (s *SkillProviderApplicationServiceImpl) Sync(req dtos.ProviderSyncRequest) (dtos.SkillProviderInfo, error) {
	do, err := s.providerDomainSvc.Sync(req.ProviderID)
	if err != nil {
		return dtos.SkillProviderInfo{}, err
	}
	count, _ := s.providerDomainSvc.CountSkillsByProviderId(req.ProviderID)
	return dtos.ConvertSkillProviderDO2Info(do, int(count)), nil
}

func (s *SkillProviderApplicationServiceImpl) Delete(req dtos.ProviderDeleteRequest) error {
	return s.providerDomainSvc.DeleteById(req.ProviderID)
}

func (s *SkillProviderApplicationServiceImpl) List(req dtos.ProviderListRequest) ([]dtos.SkillProviderInfo, error) {
	dos, err := s.providerDomainSvc.List(req.ProviderType, req.Status)
	if err != nil {
		return nil, err
	}
	result := make([]dtos.SkillProviderInfo, 0, len(dos))
	for _, do := range dos {
		count, _ := s.providerDomainSvc.CountSkillsByProviderId(do.SkillProviderID)
		result = append(result, dtos.ConvertSkillProviderDO2Info(do, int(count)))
	}
	return result, nil
}

func (s *SkillProviderApplicationServiceImpl) Detail(req dtos.ProviderDetailRequest) (dtos.SkillProviderInfo, error) {
	do, err := s.providerDomainSvc.GetById(req.ProviderID)
	if err != nil {
		return dtos.SkillProviderInfo{}, err
	}
	count, _ := s.providerDomainSvc.CountSkillsByProviderId(req.ProviderID)
	return dtos.ConvertSkillProviderDO2Info(do, int(count)), nil
}
