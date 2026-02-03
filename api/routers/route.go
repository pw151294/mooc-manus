package routers

import (
	"mooc-manus/api/handlers"
	"mooc-manus/internal/applications/services"
	app_svc "mooc-manus/internal/applications/services"
	domain_svc "mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/infra/external/health_checker"
	"mooc-manus/internal/infra/repositories"

	"github.com/gin-gonic/gin"
)

func InitRouter() *gin.Engine {
	r := gin.Default()

	// Initialize repositories
	appConfigRepo := repositories.NewAppConfigRepository()
	providerRepo := repositories.NewToolProviderRepository()
	functionRepo := repositories.NewToolFunctionRepository()

	// Initialize domain services
	providerDomainSvc := domain_svc.NewToolProviderDomainService(providerRepo, functionRepo)
	functionDomainSvc := domain_svc.NewToolFunctionDomainService(functionRepo, providerRepo)
	appConfigDomainSvc := domain_svc.NewAppConfigDomainService(appConfigRepo, functionDomainSvc)
	baseAgentDomainSvc := agents.NewBaseAgentDomainService(appConfigDomainSvc, providerDomainSvc, functionDomainSvc)

	// Initialize application services
	appConfigAppSvc := app_svc.NewAppConfigApplicationService(appConfigDomainSvc)
	providerAppSvc := app_svc.NewToolProviderApplicationService(providerDomainSvc)
	functionAppSvc := app_svc.NewTooLFunctionApplicationService(functionDomainSvc)
	baseAgentAppSvc := app_svc.NewBaseAgentApplicationService(baseAgentDomainSvc)

	// Initialize handlers
	toolHandler := handlers.NewToolHandler(providerAppSvc, functionAppSvc)
	agentHandler := handlers.NewAgentHandler(baseAgentAppSvc)

	status := r.Group("/api")
	{
		checkers := []health_checker.HealthChecker{
			&health_checker.RedisHealthChecker{},
			&health_checker.PostgresHealthChecker{},
		}
		statusAppSvc := services.NewStatusApplicationService(checkers...)
		statusHandler := handlers.NewStatusHandler(statusAppSvc)
		status.GET("/status", statusHandler.Check)
	}

	appConfig := r.Group("/api/app/config")
	{
		appConfigHandler := handlers.NewAppConfigHandler(appConfigAppSvc)
		appConfig.GET("/:id", appConfigHandler.Get)
		appConfig.PUT("/:id", appConfigHandler.Update)
		appConfig.POST("", appConfigHandler.Add)
		appConfig.DELETE("/:id", appConfigHandler.Delete)
		appConfig.GET("", appConfigHandler.List)
		appConfig.GET("/a2a/servers/:id", appConfigHandler.GetA2AServers)
		appConfig.POST("/a2a/servers", appConfigHandler.CreateA2AServers)
		appConfig.PUT("/a2a/servers", appConfigHandler.UpdateA2AServers)
		appConfig.DELETE("/a2a/servers", appConfigHandler.DeleteA2AServers)
	}

	tool := r.Group("/api/tools")
	{
		tool.POST("/provider", toolHandler.AddProvider)
		tool.PUT("/provider/:id", toolHandler.UpdateProvider)
		tool.DELETE("/provider/:id", toolHandler.DeleteProvider)
		tool.GET("/provider/list", toolHandler.ListProviders)

		tool.POST("/function", toolHandler.AddFunction)
		tool.POST("/function/mcp", toolHandler.AddMcpFunctions)
		tool.PUT("/function/:id", toolHandler.UpdateFunction)
		tool.DELETE("/function/:id", toolHandler.DeleteFunction)
		tool.GET("/function/list", toolHandler.ListFunctionsByProvider)
	}

	agent := r.Group("/api/agent")
	{
		agent.POST("/chat", agentHandler.Chat)
		agent.POST("/plan/create", agentHandler.CreatePlan)
		agent.POST("/plan/update", agentHandler.UpdatePlan)
	}

	return r
}
