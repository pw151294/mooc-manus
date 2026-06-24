package routers

import (
	"mooc-manus/api/handlers"
	"mooc-manus/config"
	"mooc-manus/internal/applications/services"
	app_svc "mooc-manus/internal/applications/services"
	domain_svc "mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/flows"
	"mooc-manus/internal/infra/external/file_storage"
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

	// Initialize domain services (without agent services yet - need skillRepo first)
	providerDomainSvc := domain_svc.NewToolProviderDomainService(providerRepo, functionRepo)
	functionDomainSvc := domain_svc.NewToolFunctionDomainService(functionRepo, providerRepo)
	appConfigDomainSvc := domain_svc.NewAppConfigDomainService(appConfigRepo, functionDomainSvc)

	// Initialize application services (without agent services yet)
	appConfigAppSvc := app_svc.NewAppConfigApplicationService(appConfigDomainSvc)
	providerAppSvc := app_svc.NewToolProviderApplicationService(providerDomainSvc)
	functionAppSvc := app_svc.NewTooLFunctionApplicationService(functionDomainSvc)

	// Initialize handlers (without agent handler yet)
	toolHandler := handlers.NewToolHandler(providerAppSvc, functionAppSvc)

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

	// ============================================================
	// Skill 模块（阶段 7）
	// ============================================================

	// 1) Repository
	skillProviderRepo := repositories.NewSkillProviderRepository()
	skillRepo := repositories.NewSkillRepository()
	skillVersionRepo := repositories.NewSkillVersionRepository()
	taskExecutionRepo := repositories.NewTaskExecutionRepository()

	// 2) FileStorage
	rootDir := "./data"
	if config.Cfg != nil {
		rootDir = config.Cfg.Storage.RootDir
	}
	fs := file_storage.NewLocalFileStorage(rootDir)

	// 3) Domain Service
	skillProviderDomainSvc := domain_svc.NewSkillProviderDomainService(skillProviderRepo, skillRepo)
	skillDomainSvc := domain_svc.NewSkillDomainService(skillRepo, skillVersionRepo, skillProviderRepo, fs)
	skillVersionDomainSvc := domain_svc.NewSkillVersionDomainService(skillVersionRepo, skillRepo, fs)
	skillImportTaskDomainSvc := domain_svc.NewSkillImportTaskDomainService(taskExecutionRepo, skillDomainSvc, skillProviderDomainSvc, fs)

	// 4) Application Service
	skillProviderAppSvc := app_svc.NewSkillProviderApplicationService(skillProviderDomainSvc)
	skillVersionAppSvc := app_svc.NewSkillVersionApplicationService(skillVersionDomainSvc, skillDomainSvc)
	skillImportTaskAppSvc := app_svc.NewSkillImportTaskApplicationService(skillImportTaskDomainSvc)
	skillAppSvc := app_svc.NewSkillApplicationService(skillDomainSvc, skillVersionDomainSvc, skillProviderDomainSvc)

	// 5) Handler（单一 SkillHandler 持有全部 4 个 ApplicationService）
	skillHandler := handlers.NewSkillHandler(skillAppSvc, skillProviderAppSvc, skillVersionAppSvc, skillImportTaskAppSvc)

	// ============================================================
	// Agent 模块（依赖 Skill 模块的 skillRepo / skillVersionRepo / fs）
	// ============================================================
	baseAgentDomainSvc := agents.NewBaseAgentDomainService(
		appConfigDomainSvc,
		providerDomainSvc,
		functionDomainSvc,
		skillRepo,
		skillVersionRepo,
		fs,
	)
	a2aDomainSvc := agents.NewA2ADomainService(baseAgentDomainSvc, appConfigDomainSvc, providerDomainSvc, functionDomainSvc)
	baseFlowDomainSvc := flows.NewBaseFlowDomainService(appConfigDomainSvc, providerDomainSvc, functionDomainSvc)

	baseAgentAppSvc := app_svc.NewBaseAgentApplicationService(baseAgentDomainSvc)
	a2aAppSvc := app_svc.NewA2AApplicationService(a2aDomainSvc)
	baseFlowAppSvc := app_svc.NewFlowApplicationService(baseFlowDomainSvc)

	agentHandler := handlers.NewAgentHandler(baseAgentAppSvc, a2aAppSvc)
	flowHandler := handlers.NewFlowHandler(baseFlowAppSvc)

	// ============================================================
	// 路由注册
	// ============================================================

	agent := r.Group("/api/agent")
	{
		agent.POST("/chat", agentHandler.Chat)
		agent.POST("/a2a/chat", agentHandler.A2AChat)
		agent.POST("/plan/create", agentHandler.CreatePlan)
		agent.POST("/plan/update", agentHandler.UpdatePlan)
	}

	flow := r.Group("/api/flow")
	{
		flow.POST("/run", flowHandler.Run)
	}

	// 6) 路由注册（4 个分组，27 个接口）

	// 6) 路由注册（4 个分组，27 个接口）
	skill := r.Group("/api/v1/skill")
	{
		skill.POST("/draft/save", skillHandler.DraftSave)
		skill.POST("/publish", skillHandler.Publish)
		skill.POST("/update", skillHandler.Update)
		skill.POST("/delete", skillHandler.Delete)
		skill.POST("/list", skillHandler.List)
		skill.POST("/listAll", skillHandler.ListAll)
		skill.POST("/detail", skillHandler.Detail)
		skill.POST("/with/version", skillHandler.WithVersion)
		skill.GET("/file/download", skillHandler.FileDownload)
	}
	skillProvider := r.Group("/api/v1/skill/provider")
	{
		skillProvider.POST("/import/git", skillHandler.ProviderImportGit)
		skillProvider.POST("/import/zip", skillHandler.ProviderImportZip)
		skillProvider.POST("/import/zip/legacy", skillHandler.ProviderImportZipLegacy)
		skillProvider.POST("/sync", skillHandler.ProviderSync)
		skillProvider.POST("/delete", skillHandler.ProviderDelete)
		skillProvider.POST("/list", skillHandler.ProviderList)
		skillProvider.POST("/detail", skillHandler.ProviderDetail)
	}
	skillImportTask := r.Group("/api/v1/skill/provider/import/task")
	{
		skillImportTask.POST("/detail", skillHandler.ImportTaskDetail)
		skillImportTask.POST("/list", skillHandler.ImportTaskList)
		skillImportTask.POST("/delete", skillHandler.ImportTaskDelete)
	}
	skillVersion := r.Group("/api/v1/skill/version")
	{
		skillVersion.POST("/create", skillHandler.VersionCreate)
		skillVersion.POST("/validate", skillHandler.VersionValidate)
		skillVersion.POST("/delete", skillHandler.VersionDelete)
		skillVersion.POST("/list", skillHandler.VersionList)
		skillVersion.POST("/detail", skillHandler.VersionDetail)
		skillVersion.POST("/latest", skillHandler.VersionLatest)
		skillVersion.POST("/rollback", skillHandler.VersionRollback)
		skillVersion.POST("/export", skillHandler.VersionExport)
	}

	return r
}
