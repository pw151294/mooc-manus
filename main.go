package main

import (
	"log"
	"mooc-manus/api/routers"
	"mooc-manus/config"
	"mooc-manus/internal/infra/storage"
	"mooc-manus/pkg/logger"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 1. 初始化全局配置和日志配置
	config.InitConfig()
	if err := logger.InitGlobalLogger(config.Cfg.LoggerConfig); err != nil {
		log.Fatalf("init logger error: %s", err.Error())
	}

	// 2. 初始化Redis
	if err := storage.InitRedis(); err != nil {
		log.Fatalf("init redis failed: %v", err)
	}

	// 3. 初始化Postgres
	if err := storage.InitStorage(); err != nil {
		log.Fatalf("init postgres failed: %v", err)
	}

	// 4. 初始化路由并启动服务
	r := routers.InitRouter()
	go func() {
		if err := r.Run(":8080"); err != nil {
			log.Fatalf("start server failed: %v", err)
		}
	}()

	// 实现服务的优雅退出
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down server...")
}
