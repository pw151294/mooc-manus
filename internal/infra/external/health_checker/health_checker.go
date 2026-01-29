package health_checker

const HealthyStatus = "ok"
const UnHealthyStatus = "error"

type HealthStatus struct {
	Service string
	Status  string
	Detail  string
}
type HealthChecker interface {
	Check() HealthStatus
}
