package services

import (
	"io"
	"mime/multipart"

	"mooc-manus/internal/applications/dtos"
	domainSvc "mooc-manus/internal/domains/services"
)

type SkillApplicationService interface {
	DraftSave(req *dtos.SkillDraftSaveRequest, files []*multipart.FileHeader) (dtos.SkillInfo, error)
	Publish(req *dtos.SkillPublishRequest, files []*multipart.FileHeader) (dtos.SkillInfo, error)
	Update(req dtos.SkillUpdateRequest) (dtos.SkillInfo, error)
	Delete(req dtos.SkillDeleteRequest) error
	List(req *dtos.SkillListRequest) (dtos.SkillPageDTO, error)
	ListAll(req *dtos.SkillListAllRequest) ([]dtos.SkillInfo, error)
	Detail(req dtos.SkillDetailRequest) (dtos.SkillInfo, error)
	WithVersion() ([]dtos.SkillWithVersionInfo, error)
	FileDownload(query dtos.SkillFileDownloadQuery) (io.ReadCloser, string, error)
}

type SkillApplicationServiceImpl struct {
	skillDomainSvc    domainSvc.SkillDomainService
	versionDomainSvc  domainSvc.SkillVersionDomainService
	providerDomainSvc domainSvc.SkillProviderDomainService
}

func NewSkillApplicationService(
	skillDomainSvc domainSvc.SkillDomainService,
	versionDomainSvc domainSvc.SkillVersionDomainService,
	providerDomainSvc domainSvc.SkillProviderDomainService,
) SkillApplicationService {
	return &SkillApplicationServiceImpl{
		skillDomainSvc:    skillDomainSvc,
		versionDomainSvc:  versionDomainSvc,
		providerDomainSvc: providerDomainSvc,
	}
}

// assembleSkillInfo 将 SkillDO 组装为完整的 SkillInfo（含 ProviderName / LatestVersion / Versions / Files / VersionCount）
func (s *SkillApplicationServiceImpl) assembleSkillInfo(skillID string, light bool) (dtos.SkillInfo, error) {
	do, err := s.skillDomainSvc.GetById(skillID)
	if err != nil {
		return dtos.SkillInfo{}, err
	}
	providerName := ""
	providerDO, pErr := s.providerDomainSvc.GetById(do.SkillProviderID)
	if pErr == nil {
		providerName = providerDO.ProviderName
	}
	info := dtos.ConvertSkillDO2Info(do, providerName)

	releasedVersions, _ := s.skillDomainSvc.GetReleasedVersions(skillID)
	info.VersionCount = len(releasedVersions)

	latestDO, _ := s.skillDomainSvc.GetLatestVersion(skillID)
	if latestDO != nil {
		v := dtos.ConvertSkillVersionDO2Info(*latestDO, do.SkillName)
		info.LatestVersion = &v
	}

	if !light {
		versionInfos := make([]dtos.SkillVersionInfo, 0, len(releasedVersions))
		for _, vdo := range releasedVersions {
			versionInfos = append(versionInfos, dtos.ConvertSkillVersionDO2Info(vdo, do.SkillName))
		}
		info.Versions = versionInfos
		// files：优先正式版本，否则 draft
		if latestDO != nil {
			info.Files = latestDO.SkillFiles
		}
	}
	return info, nil
}

func (s *SkillApplicationServiceImpl) DraftSave(req *dtos.SkillDraftSaveRequest, files []*multipart.FileHeader) (dtos.SkillInfo, error) {
	do, err := s.skillDomainSvc.DraftSave(req, files)
	if err != nil {
		return dtos.SkillInfo{}, err
	}
	return s.assembleSkillInfo(do.SkillID, false)
}

func (s *SkillApplicationServiceImpl) Publish(req *dtos.SkillPublishRequest, files []*multipart.FileHeader) (dtos.SkillInfo, error) {
	do, err := s.skillDomainSvc.Publish(req, files)
	if err != nil {
		return dtos.SkillInfo{}, err
	}
	return s.assembleSkillInfo(do.SkillID, false)
}

func (s *SkillApplicationServiceImpl) Update(req dtos.SkillUpdateRequest) (dtos.SkillInfo, error) {
	do, err := s.skillDomainSvc.Update(&req)
	if err != nil {
		return dtos.SkillInfo{}, err
	}
	return s.assembleSkillInfo(do.SkillID, true)
}

func (s *SkillApplicationServiceImpl) Delete(req dtos.SkillDeleteRequest) error {
	return s.skillDomainSvc.Delete(req.SkillID)
}

func (s *SkillApplicationServiceImpl) List(req *dtos.SkillListRequest) (dtos.SkillPageDTO, error) {
	dos, total, err := s.skillDomainSvc.List(req)
	if err != nil {
		return dtos.SkillPageDTO{}, err
	}
	providerIDs := make([]string, 0, len(dos))
	for _, do := range dos {
		providerIDs = append(providerIDs, do.SkillProviderID)
	}
	providerMap, _ := s.providerDomainSvc.GetByIds(providerIDs)

	// 批量加载最新版本
	skillIDs := make([]string, 0, len(dos))
	for _, do := range dos {
		skillIDs = append(skillIDs, do.SkillID)
	}

	records := make([]dtos.SkillInfo, 0, len(dos))
	for _, do := range dos {
		providerName := ""
		if p, ok := providerMap[do.SkillProviderID]; ok {
			providerName = p.ProviderName
		}
		info := dtos.ConvertSkillDO2Info(do, providerName)
		latestDO, _ := s.skillDomainSvc.GetLatestVersion(do.SkillID)
		if latestDO != nil {
			v := dtos.ConvertSkillVersionDO2Info(*latestDO, do.SkillName)
			info.LatestVersion = &v
		}
		releasedVersions, _ := s.skillDomainSvc.GetReleasedVersions(do.SkillID)
		info.VersionCount = len(releasedVersions)
		records = append(records, info)
	}
	return dtos.SkillPageDTO{
		Total:    total,
		PageSize: req.PageSize,
		PageNum:  req.PageNum,
		Records:  records,
	}, nil
}

func (s *SkillApplicationServiceImpl) ListAll(req *dtos.SkillListAllRequest) ([]dtos.SkillInfo, error) {
	dos, err := s.skillDomainSvc.ListAll(req)
	if err != nil {
		return nil, err
	}
	providerIDs := make([]string, 0, len(dos))
	for _, do := range dos {
		providerIDs = append(providerIDs, do.SkillProviderID)
	}
	providerMap, _ := s.providerDomainSvc.GetByIds(providerIDs)
	result := make([]dtos.SkillInfo, 0, len(dos))
	for _, do := range dos {
		providerName := ""
		if p, ok := providerMap[do.SkillProviderID]; ok {
			providerName = p.ProviderName
		}
		info := dtos.ConvertSkillDO2Info(do, providerName)
		result = append(result, info)
	}
	return result, nil
}

func (s *SkillApplicationServiceImpl) Detail(req dtos.SkillDetailRequest) (dtos.SkillInfo, error) {
	return s.assembleSkillInfo(req.SkillID, false)
}

func (s *SkillApplicationServiceImpl) WithVersion() ([]dtos.SkillWithVersionInfo, error) {
	dos, err := s.skillDomainSvc.WithVersion()
	if err != nil {
		return nil, err
	}
	providerIDs := make([]string, 0, len(dos))
	for _, do := range dos {
		providerIDs = append(providerIDs, do.SkillProviderID)
	}
	providerMap, _ := s.providerDomainSvc.GetByIds(providerIDs)
	result := make([]dtos.SkillWithVersionInfo, 0, len(dos))
	for _, do := range dos {
		providerName := ""
		if p, ok := providerMap[do.SkillProviderID]; ok {
			providerName = p.ProviderName
		}
		releasedVersions, _ := s.skillDomainSvc.GetReleasedVersions(do.SkillID)
		versionInfos := make([]dtos.SkillVersionInfo, 0, len(releasedVersions))
		for _, vdo := range releasedVersions {
			versionInfos = append(versionInfos, dtos.ConvertSkillVersionDO2Info(vdo, do.SkillName))
		}
		result = append(result, dtos.ConvertSkillDO2WithVersion(do, providerName, versionInfos))
	}
	return result, nil
}

func (s *SkillApplicationServiceImpl) FileDownload(query dtos.SkillFileDownloadQuery) (io.ReadCloser, string, error) {
	return s.skillDomainSvc.FileDownload(query.FileKey)
}
