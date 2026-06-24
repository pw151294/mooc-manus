package services

import (
	"io"

	"mooc-manus/internal/applications/dtos"
	domainSvc "mooc-manus/internal/domains/services"
)

type SkillVersionApplicationService interface {
	Create(req dtos.VersionCreateRequest) (dtos.SkillVersionInfo, error)
	Validate(req dtos.VersionValidateRequest) (dtos.SkillVersionInfo, error)
	Delete(req dtos.VersionDeleteRequest) error
	List(req dtos.VersionListRequest) ([]dtos.SkillVersionInfo, error)
	Detail(req dtos.VersionDetailRequest) (dtos.SkillVersionInfo, error)
	Latest(req dtos.VersionLatestRequest) (*dtos.SkillVersionInfo, error)
	Rollback(req dtos.VersionRollbackRequest) (dtos.SkillVersionInfo, error)
	Export(req dtos.VersionExportRequest, w io.Writer) (filename string, err error)
}

type SkillVersionApplicationServiceImpl struct {
	versionDomainSvc domainSvc.SkillVersionDomainService
	skillDomainSvc   domainSvc.SkillDomainService
}

func NewSkillVersionApplicationService(
	versionDomainSvc domainSvc.SkillVersionDomainService,
	skillDomainSvc domainSvc.SkillDomainService,
) SkillVersionApplicationService {
	return &SkillVersionApplicationServiceImpl{
		versionDomainSvc: versionDomainSvc,
		skillDomainSvc:   skillDomainSvc,
	}
}

func (s *SkillVersionApplicationServiceImpl) Create(req dtos.VersionCreateRequest) (dtos.SkillVersionInfo, error) {
	do, err := s.versionDomainSvc.Create(req.SkillID, req.Version)
	if err != nil {
		return dtos.SkillVersionInfo{}, err
	}
	skillDO, _ := s.skillDomainSvc.GetById(req.SkillID)
	return dtos.ConvertSkillVersionDO2Info(do, skillDO.SkillName), nil
}

func (s *SkillVersionApplicationServiceImpl) Validate(req dtos.VersionValidateRequest) (dtos.SkillVersionInfo, error) {
	do, err := s.versionDomainSvc.Validate(req.VersionID)
	if err != nil {
		return dtos.SkillVersionInfo{}, err
	}
	skillDO, _ := s.skillDomainSvc.GetById(do.SkillID)
	return dtos.ConvertSkillVersionDO2Info(do, skillDO.SkillName), nil
}

func (s *SkillVersionApplicationServiceImpl) Delete(req dtos.VersionDeleteRequest) error {
	return s.versionDomainSvc.Delete(req.SkillID, req.Version)
}

func (s *SkillVersionApplicationServiceImpl) List(req dtos.VersionListRequest) ([]dtos.SkillVersionInfo, error) {
	dos, err := s.versionDomainSvc.ListReleased(req.SkillID)
	if err != nil {
		return nil, err
	}
	skillDO, _ := s.skillDomainSvc.GetById(req.SkillID)
	result := make([]dtos.SkillVersionInfo, 0, len(dos))
	for _, do := range dos {
		result = append(result, dtos.ConvertSkillVersionDO2Info(do, skillDO.SkillName))
	}
	return result, nil
}

func (s *SkillVersionApplicationServiceImpl) Detail(req dtos.VersionDetailRequest) (dtos.SkillVersionInfo, error) {
	do, err := s.versionDomainSvc.Detail(req.SkillID, req.Version)
	if err != nil {
		return dtos.SkillVersionInfo{}, err
	}
	skillDO, _ := s.skillDomainSvc.GetById(req.SkillID)
	return dtos.ConvertSkillVersionDO2Info(do, skillDO.SkillName), nil
}

func (s *SkillVersionApplicationServiceImpl) Latest(req dtos.VersionLatestRequest) (*dtos.SkillVersionInfo, error) {
	do, err := s.versionDomainSvc.Latest(req.SkillID)
	if err != nil || do == nil {
		return nil, err
	}
	skillDO, _ := s.skillDomainSvc.GetById(req.SkillID)
	info := dtos.ConvertSkillVersionDO2Info(*do, skillDO.SkillName)
	return &info, nil
}

func (s *SkillVersionApplicationServiceImpl) Rollback(req dtos.VersionRollbackRequest) (dtos.SkillVersionInfo, error) {
	do, err := s.versionDomainSvc.Rollback(req.SkillID, req.TargetVersion)
	if err != nil {
		return dtos.SkillVersionInfo{}, err
	}
	skillDO, _ := s.skillDomainSvc.GetById(req.SkillID)
	return dtos.ConvertSkillVersionDO2Info(do, skillDO.SkillName), nil
}

func (s *SkillVersionApplicationServiceImpl) Export(req dtos.VersionExportRequest, w io.Writer) (string, error) {
	skillName, err := s.versionDomainSvc.Export(req.SkillID, req.Version, w)
	return skillName, err
}
