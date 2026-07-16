package routers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"mooc-manus/api/handlers"
	"mooc-manus/config"
	app_svc "mooc-manus/internal/applications/services"
	"mooc-manus/internal/domains/models/tracing"
	domain_svc "mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/evaluation"
	"mooc-manus/internal/domains/services/flows"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/external/health_checker"
	"mooc-manus/internal/infra/mq"
	"mooc-manus/internal/infra/repositories"
	"mooc-manus/internal/infra/scheduler"
	"mooc-manus/internal/infra/storage"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
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

	// 1.3 Tracing 模块 Repository + 全局 Tracer 单例
	aiSpanRepo := repositories.NewAiSpanRepository()
	// tracing：初始化 Tracer 单例（异步 batch flush 落盘）
	tracer := tracing.NewTracer(aiSpanRepo)
	tracing.SetGlobal(tracer)

	// 1.4 FileStorage（基础设施，与 Repository 同级别）
	rootDir := "./data"
	if config.Cfg != nil {
		rootDir = config.Cfg.Storage.RootDir
	}
	fs := file_storage.NewLocalFileStorage(rootDir)

	// 1.5 评测模块 Repository
	evalCaseRepo := repositories.NewEvalCaseRepository(storage.GetPostgresClient())
	evalTaskRepo := repositories.NewEvalTaskRepository(storage.GetPostgresClient())
	evalInstRepo := repositories.NewEvalRunInstanceRepository(storage.GetPostgresClient())
	evalResultRepo := repositories.NewEvalResultRepository(storage.GetPostgresClient())
	evalSnapshotRepo := repositories.NewEvalAgentSnapshotRepository(storage.GetPostgresClient())

	// 1.6 评测 Asynq 客户端（Enabled=false 时 nil，Application 层降级为跳过 enqueue）
	var asynqClient *mq.Client
	if config.Cfg != nil && config.Cfg.Evaluation.Enabled {
		asynqClient = mq.NewClient(config.Cfg.Asynq)
	}

	// 1.7 评测装配用的原始 zap logger（domain 层要求 *zap.Logger）
	// 使用 zap.NewProduction 作为主 logger；若初始化失败则回落 NopLogger。
	evalZap, err := zap.NewProduction()
	if err != nil {
		evalZap = zap.NewNop()
	}

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

	// 2.2.6 NATIVE 内置工具 Provider 装配（fileRead / fileEdit / bashExec）
	// 详细约束见 .harness/rules/49-native-builtin.md
	// provider 内部完成默认值回退与 NativeWorkspace/BashDenyList 装配
	nativeToolsProvider := tools.NewNativeToolsProvider(config.Cfg.Native, rootDir)

	// 2.3 Agent 模块 Domain Service（依赖 Skill repo，放在 Skill Domain Service 之后）
	baseAgentDomainSvc := agents.NewBaseAgentDomainService(
		appConfigDomainSvc,
		providerDomainSvc,
		functionDomainSvc,
		skillRepo,
		skillVersionRepo,
		skillExecutor,
		fs,
		nativeToolsProvider,
	)
	a2aDomainSvc := agents.NewA2ADomainService(baseAgentDomainSvc, appConfigDomainSvc, providerDomainSvc, functionDomainSvc)
	baseFlowDomainSvc := flows.NewBaseFlowDomainService(appConfigDomainSvc, providerDomainSvc, functionDomainSvc)

	// 2.4 评测模块 Domain 依赖
	// 参数值来自 EvaluationConfig：verify 脚本超时 / 心跳间隔 / 实例总超时；
	// snapshot 冻结 + M×N instance 装配都由 domain 层负责，此处仅注入依赖。
	evalCfg := config.EvaluationConfig{}
	if config.Cfg != nil {
		evalCfg = config.Cfg.Evaluation
	}
	verifyRunner := evaluation.NewVerifyRunner(
		time.Duration(evalCfg.VerifyScriptTimeoutSec)*time.Second,
		evalCfg.VerifyOutputCapBytes,
	)
	traceAggregator := evaluation.NewTraceAggregator(aiSpanRepo, evalZap)
	internalChatRunner := evaluation.NewInternalChatRunner(baseAgentDomainSvc)

	workerID := hostname() + "-" + strconv.Itoa(os.Getpid())
	instanceExecutor := evaluation.NewInstanceExecutor(
		evalInstRepo, evalTaskRepo, evalResultRepo, evalSnapshotRepo,
		verifyRunner, internalChatRunner, traceAggregator, tracer,
		skillExecutor, nativeToolsProvider,
		workerID,
		time.Duration(evalCfg.HeartbeatIntervalSec)*time.Second,
		time.Duration(evalCfg.InstanceTotalTimeoutSec)*time.Second,
		evalZap,
	)

	// AppConfigDomainService.GetById 已经满足 evaluation.AppConfigLoader 接口，直接注入。
	// dlqInspector 目前 nil（M9 后续补 asynq.Inspector 适配器），ArchiveDeadTasks 会降级返回 (0, nil)。
	evalDomainSvc := evaluation.NewEvaluationDomainService(
		evalCaseRepo, evalTaskRepo, evalInstRepo, evalResultRepo, evalSnapshotRepo,
		appConfigDomainSvc, instanceExecutor, nil, evalZap,
	)

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
	baseAgentAppSvc := app_svc.NewBaseAgentApplicationService(baseAgentDomainSvc, skillExecutor, nativeToolsProvider)
	a2aAppSvc := app_svc.NewA2AApplicationService(a2aDomainSvc)
	baseFlowAppSvc := app_svc.NewFlowApplicationService(baseFlowDomainSvc)

	// 3.4 Tracing 模块 Application Service
	traceAppSvc := app_svc.NewTraceApplicationService(aiSpanRepo)

	// 3.5 评测 Application Service
	// mq 允许 nil（Evaluation.Enabled=false 或 asynq 未装配时），此时 CreateTask/Retry 会跳过 enqueue，
	// 由 cron 巡检层兜底把长时间未被消费的实例重新推进。
	var evalMQ app_svc.MQEnqueuer
	if asynqClient != nil {
		evalMQ = asynqClient
	}
	evalAppSvc := app_svc.NewEvaluationApplicationService(
		evalDomainSvc, evalCaseRepo, evalTaskRepo, evalInstRepo, evalResultRepo,
		appConfigDomainSvc, evalMQ, evalCfg, evalZap,
	)

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

	// 4.4 Tracing 模块 Handler
	traceHandler := handlers.NewTraceHandler(traceAppSvc)

	// 4.5 评测模块 Handler
	evalHandler := handlers.NewEvalHandler(evalAppSvc)

	// ============================================================
	// 评测基础设施启动：Asynq server + Cron 巡检
	// ============================================================
	var asynqSrv *asynq.Server
	if config.Cfg != nil && config.Cfg.Evaluation.Enabled && asynqClient != nil {
		rdb := redis.NewClient(&redis.Options{
			Addr:     config.Cfg.Asynq.RedisAddr,
			Password: config.Cfg.Asynq.RedisPassword,
			DB:       config.Cfg.Asynq.RedisDB,
		})
		// 每个 case 的并发上限；TTL 兜底：实例总超时 + 60s 缓冲，避免异常退出令牌泄漏
		gate := mq.NewCaseTokenGate(rdb,
			evalCfg.CaseConcurrencyLimit,
			time.Duration(evalCfg.InstanceTotalTimeoutSec+60)*time.Second,
		)
		runInstanceHandler := mq.NewRunInstanceHandler(instanceExecutor, evalInstRepo, gate)
		srv, serr := mq.StartServer(config.Cfg.Asynq, evalCfg, runInstanceHandler)
		if serr != nil {
			evalZap.Error("asynq server 启动失败", zap.Error(serr))
		} else {
			asynqSrv = srv
			evalZap.Info("asynq server started",
				zap.String("redis_addr", config.Cfg.Asynq.RedisAddr))
		}
	}
	_ = asynqSrv // 目前 InitRouter 无优雅停机；保留变量供后续 shutdown 钩子使用

	// Cron 巡检（M7）：sweeper / reconciler / dlq_archiver
	if config.Cfg != nil && config.Cfg.Evaluation.Enabled {
		sched := scheduler.New(evalZap)
		if evalCfg.CronSweeperIntervalSec > 0 {
			_ = sched.AddFunc(
				fmt.Sprintf("*/%d * * * * *", evalCfg.CronSweeperIntervalSec),
				scheduler.NewSweeperJob(evalDomainSvc, evalZap).Run,
			)
		}
		if evalCfg.CronReconcilerIntervalSec > 0 {
			_ = sched.AddFunc(
				fmt.Sprintf("*/%d * * * * *", evalCfg.CronReconcilerIntervalSec),
				scheduler.NewReconcilerJob(evalDomainSvc, evalZap).Run,
			)
		}
		if evalCfg.CronDLQArchiveIntervalSec > 0 {
			_ = sched.AddFunc(
				fmt.Sprintf("*/%d * * * * *", evalCfg.CronDLQArchiveIntervalSec),
				scheduler.NewDLQArchiverJob(evalDomainSvc, evalZap).Run,
			)
		}
		sched.Start()
		evalZap.Info("eval cron scheduler started")
	}

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

	trace := r.Group("/api")
	{
		trace.GET("/trace/:trace_id", traceHandler.GetDetail)
		trace.GET("/traces", traceHandler.List)
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
		agent.POST("/message/stop", agentHandler.StopMessage)
		agent.POST("/conversation/stop", agentHandler.StopConversation)
		agent.POST("/resume", agentHandler.Resume)
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
	eval := r.Group("/api/eval")
	{
		eval.POST("/cases/upload-content", evalHandler.UploadContent)
		eval.POST("/cases", evalHandler.CreateCase)
		eval.GET("/cases", evalHandler.ListCases)
		eval.GET("/cases/:id", evalHandler.GetCase)
		eval.PUT("/cases/:id", evalHandler.UpdateCase)
		eval.DELETE("/cases/:id", evalHandler.DeleteCase)

		eval.POST("/tasks", evalHandler.CreateTask)
		eval.GET("/tasks", evalHandler.ListTasks)
		eval.GET("/tasks/:id", evalHandler.GetTask)
		eval.POST("/tasks/:id/retry", evalHandler.RetryTask)
		eval.DELETE("/tasks/:id", evalHandler.DeleteTask)
		eval.GET("/tasks/:id/instances", evalHandler.ListInstances)

		eval.GET("/instances/:id", evalHandler.GetInstance)
		eval.GET("/instances/:id/trace", evalHandler.GetInstanceTrace)
		eval.POST("/instances/:id/retry", evalHandler.RetryInstance)
		eval.DELETE("/instances/:id", evalHandler.DeleteInstance)

		eval.GET("/agent-configs", evalHandler.ListAgentConfigs)
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

// hostname 返回稳定的主机名；失败或空值时返回 "unknown"。
// 用于组装评测 worker 的 workerID（hostname-pid），便于排障。
func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}
