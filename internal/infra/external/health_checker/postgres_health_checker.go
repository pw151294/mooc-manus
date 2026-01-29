package health_checker

import (
	"mooc-manus/internal/infra/storage"
)

type PostgresHealthChecker struct {
}

func (p *PostgresHealthChecker) Check() HealthStatus {
	status := HealthStatus{}
	status.Service = "postgres"
	dbCli := storage.GetPostgresClient()
	rawDB, err := dbCli.DB()
	if err != nil {
		status.Status = UnHealthyStatus
		status.Detail = err.Error()
		return status
	}

	if err := rawDB.Ping(); err != nil {
		status.Status = UnHealthyStatus
		status.Detail = err.Error()
		return status
	}
	status.Status = HealthyStatus
	return status
}
