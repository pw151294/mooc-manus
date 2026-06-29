package routers

import (
	"mooc-manus/api/handlers"
	"mooc-manus/config"
	app_svc "mooc-manus/internal/applications/services"
	domain_svc "mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/flows"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/external/health_checker"
	"mooc-manus/internal/infra/repositories"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// corsMiddleware 处理跨域请求，含 OPTIONS 预检
// 浏览器对 Content-Type: application/json 等非简单请求会先发 OPTIONS 预检，
// 未放行时 Gin 会回 404，导致前端 GET/POST 实际未发出
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, Origin, X-Requested-With")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func InitRouter() *gin.Engine {
	r := gin.Default()
	r.Use(corsMiddleware())

	// ============================================================
	// 第一层：Repository 层（按依赖拓扑顺序初始化）
	// ============================================================

	// 1.1 基础模块 Repository（无外部依赖）
	appConfigRepo := repositories.NewAppConfigRepository()
	providerRepo := repositories.NewToolProviderRepository()
	functionRepo := repositories.NewToolFunctionRepository()

	// 1.2 Skill 模块 Repository（无外部依赖）
	skillProviderRepo := repositories.NewSkillProviderRepository()
	skillRepo := repositories.NewSkillRepository()
	skillVersionRepo := repositories.NewSkillVersionRepository()
	taskExecutionRepo := repositories.NewTaskExecutionRepository()

	// 1.3 FileStorage（基础设施，与 Repository 同级别）
	rootDir := "./data"
	if config.Cfg != nil {
		rootDir = config.Cfg.Storage.RootDir
	}
	fs := file_storage.NewLocalFileStorage(rootDir)

	// ============================================================
	// 第二层：Domain Service 层（按依赖拓扑顺序初始化）
	// ============================================================

	// 2.1 基础模块 Domain Service（无跨模块依赖）
	providerDomainSvc := domain_svc.NewToolProviderDomainService(providerRepo, functionRepo)
	functionDomainSvc := domain_svc.NewToolFunctionDomainService(functionRepo, providerRepo)
	appConfigDomainSvc := domain_svc.NewAppConfigDomainService(appConfigRepo, functionDomainSvc)

	// 2.2 Skill 模块 Domain Service（无跨模块依赖）
	skillProviderDomainSvc := domain_svc.NewSkillProviderDomainService(skillProviderRepo, skillRepo)
	skillDomainSvc := domain_svc.NewSkillDomainService(skillRepo, skillVersionRepo, skillProviderRepo, fs)
	skillVersionDomainSvc := domain_svc.NewSkillVersionDomainService(skillVersionRepo, skillRepo, fs)
	skillImportTaskDomainSvc := domain_svc.NewSkillImportTaskDomainService(taskExecutionRepo, skillDomainSvc, skillProviderDomainSvc, fs)

	// 2.2.5 Skill 执行器（Docker 容器沙箱，基础设施层）
	skillExecutor := tools.NewDockerSkillExecutor(
		config.Cfg.Skill.BaseDir,
		config.Cfg.Skill.HostBaseDir,
		config.Cfg.Skill.DockerHost,
		config.Cfg.Skill.DockerImage,
		config.Cfg.Skill.Env, // 注入静态环境变量配置
	)

	// 2.2.6 NATIVE 内置工具基础设施（fileRead / fileEdit / bashExec）
	// 详细约束见 .harness/rules/49-native-builtin.md
	// WorkspaceBaseDir 为空时回退到 ${storage.root_dir}/native-workspace
	nativeWorkspaceDir := config.Cfg.Native.WorkspaceBaseDir
	if nativeWorkspaceDir == "" {
		nativeWorkspaceDir = filepath.Join(rootDir, "native-workspace")
	}
	nativeWorkspace := tools.NewNativeWorkspace(
		nativeWorkspaceDir,
		config.Cfg.Native.SensitivePathDenyList,
		config.Cfg.Native.MaxFileReadBytes,
	)
	bashDenyList := tools.NewBashDenyList(config.Cfg.Native.BashCommandDenyList)

	// 2.3 Agent 模块 Domain Service（依赖 Skill repo，放在 Skill Domain Service 之后）
	baseAgentDomainSvc := agents.NewBaseAgentDomainService(
		appConfigDomainSvc,
		providerDomainSvc,
		functionDomainSvc,
		skillRepo,
		skillVersionRepo,
		skillExecutor,
		fs,
		nativeWorkspace,
		bashDenyList,
		config.Cfg.Native.BashTimeoutDefault,
		config.Cfg.Native.BashTimeoutMax,
		config.Cfg.Native.BashOutputCap,
		config.Cfg.Native.BashConcurrency,
	)
	a2aDomainSvc := agents.NewA2ADomainService(baseAgentDomainSvc, appConfigDomainSvc, providerDomainSvc, functionDomainSvc)
	baseFlowDomainSvc := flows.NewBaseFlowDomainService(appConfigDomainSvc, providerDomainSvc, functionDomainSvc)

	// ============================================================
	// 第三层：Application Service 层（全模块并列）
	// ============================================================

	// 3.1 基础模块 Application Service
	appConfigAppSvc := app_svc.NewAppConfigApplicationService(appConfigDomainSvc)
	providerAppSvc := app_svc.NewToolProviderApplicationService(providerDomainSvc)
	functionAppSvc := app_svc.NewTooLFunctionApplicationService(functionDomainSvc)

	// 3.2 Skill 模块 Application Service
	skillProviderAppSvc := app_svc.NewSkillProviderApplicationService(skillProviderDomainSvc)
	skillVersionAppSvc := app_svc.NewSkillVersionApplicationService(skillVersionDomainSvc, skillDomainSvc)
	skillImportTaskAppSvc := app_svc.NewSkillImportTaskApplicationService(skillImportTaskDomainSvc)
	skillAppSvc := app_svc.NewSkillApplicationService(skillDomainSvc, skillVersionDomainSvc, skillProviderDomainSvc)

	// 3.3 Agent 模块 Application Service
	baseAgentAppSvc := app_svc.NewBaseAgentApplicationService(baseAgentDomainSvc, skillExecutor, nativeWorkspace)
	a2aAppSvc := app_svc.NewA2AApplicationService(a2aDomainSvc)
	baseFlowAppSvc := app_svc.NewFlowApplicationService(baseFlowDomainSvc)

	// ============================================================
	// 第四层：Handler 层（全模块并列）
	// ============================================================

	// 4.1 基础模块 Handler
	toolHandler := handlers.NewToolHandler(providerAppSvc, functionAppSvc)
	appConfigHandler := handlers.NewAppConfigHandler(appConfigAppSvc)

	// 4.2 Skill 模块 Handler（单一 Handler 持有 4 个 ApplicationService）
	skillHandler := handlers.NewSkillHandler(skillAppSvc, skillProviderAppSvc, skillVersionAppSvc, skillImportTaskAppSvc)

	// 4.3 Agent 模块 Handler
	agentHandler := handlers.NewAgentHandler(baseAgentAppSvc, a2aAppSvc)
	flowHandler := handlers.NewFlowHandler(baseFlowAppSvc)

	// ============================================================
	// 路由注册
	// ============================================================

	status := r.Group("/api")
	{
		checkers := []health_checker.HealthChecker{
			&health_checker.RedisHealthChecker{},
			&health_checker.PostgresHealthChecker{},
		}
		statusAppSvc := app_svc.NewStatusApplicationService(checkers...)
		statusHandler := handlers.NewStatusHandler(statusAppSvc)
		status.GET("/status", statusHandler.Check)
	}

	appConfig := r.Group("/api/app/config")
	{
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
		agent.POST("/a2a/chat", agentHandler.A2AChat)
		agent.POST("/plan/create", agentHandler.CreatePlan)
		agent.POST("/plan/update", agentHandler.UpdatePlan)
	}

	flow := r.Group("/api/flow")
	{
		flow.POST("/run", flowHandler.Run)
	}

	skill := r.Group("/api/skill")
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
	skillProvider := r.Group("/api/skill/provider")
	{
		skillProvider.POST("/import/git", skillHandler.ProviderImportGit)
		skillProvider.POST("/import/zip", skillHandler.ProviderImportZip)
		skillProvider.POST("/import/zip/legacy", skillHandler.ProviderImportZipLegacy)
		skillProvider.POST("/sync", skillHandler.ProviderSync)
		skillProvider.POST("/delete", skillHandler.ProviderDelete)
		skillProvider.POST("/list", skillHandler.ProviderList)
		skillProvider.POST("/detail", skillHandler.ProviderDetail)
	}
	skillImportTask := r.Group("/api/skill/provider/import/task")
	{
		skillImportTask.POST("/detail", skillHandler.ImportTaskDetail)
		skillImportTask.POST("/list", skillHandler.ImportTaskList)
		skillImportTask.POST("/delete", skillHandler.ImportTaskDelete)
	}
	skillVersion := r.Group("/api/skill/version")
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
