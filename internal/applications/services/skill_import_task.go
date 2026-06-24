package services

import (
	"mime/multipart"

	"mooc-manus/internal/applications/dtos"
	domainSvc "mooc-manus/internal/domains/services"
)

type SkillImportTaskApplicationService interface {
	ImportFromZip(file *multipart.FileHeader) (string, error)
	SubscribeTask(taskId string, emitFn domainSvc.SseEmitter) error
	List() ([]dtos.SkillImportTaskInfo, error)
	Delete(req dtos.ImportTaskDeleteRequest) error
}

type SkillImportTaskApplicationServiceImpl struct {
	importTaskDomainSvc domainSvc.SkillImportTaskDomainService
}

func NewSkillImportTaskApplicationService(importTaskDomainSvc domainSvc.SkillImportTaskDomainService) SkillImportTaskApplicationService {
	return &SkillImportTaskApplicationServiceImpl{importTaskDomainSvc: importTaskDomainSvc}
}

func (s *SkillImportTaskApplicationServiceImpl) ImportFromZip(file *multipart.FileHeader) (string, error) {
	return s.importTaskDomainSvc.ImportFromZipAsync(file)
}

func (s *SkillImportTaskApplicationServiceImpl) SubscribeTask(taskId string, emitFn domainSvc.SseEmitter) error {
	return s.importTaskDomainSvc.SubscribeTask(taskId, emitFn)
}

func (s *SkillImportTaskApplicationServiceImpl) List() ([]dtos.SkillImportTaskInfo, error) {
	dos, err := s.importTaskDomainSvc.ListTasks()
	if err != nil {
		return nil, err
	}
	result := make([]dtos.SkillImportTaskInfo, 0, len(dos))
	for _, do := range dos {
		result = append(result, dtos.ConvertTaskExecutionDO2Info(do))
	}
	return result, nil
}

func (s *SkillImportTaskApplicationServiceImpl) Delete(req dtos.ImportTaskDeleteRequest) error {
	return s.importTaskDomainSvc.DeleteTasks(req.TaskIDs)
}
