package services

import (
	"errors"
	"fmt"
	"mooc-manus/internal/infra/external/health_checker"
)

type StatusApplicationService interface {
	Check() health_checker.HealthStatus
}

type StatusApplicationServiceImpl struct {
	checkers []health_checker.HealthChecker
}

func (s *StatusApplicationServiceImpl) Check() health_checker.HealthStatus {
	status := health_checker.HealthStatus{}
	status.Service = "all"
	status.Status = health_checker.HealthyStatus

	checkErrs := make([]error, 0, 0)
	for _, checker := range s.checkers {
		checkStatus := checker.Check()
		if checkStatus.Status == health_checker.UnHealthyStatus {
			status.Status = health_checker.UnHealthyStatus
			checkErrs = append(checkErrs, fmt.Errorf("服务%s健康检查不通过：%s", checkStatus.Service, checkStatus.Detail))
		}
	}
	if len(checkErrs) > 0 {
		checkRes := fmt.Errorf("健康检查不通过：%v", errors.Join(checkErrs...))
		status.Detail = checkRes.Error()
	}
	return status
}

func NewStatusApplicationService(checkers ...health_checker.HealthChecker) StatusApplicationService {
	return &StatusApplicationServiceImpl{checkers: checkers}
}
