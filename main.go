package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mooc-manus/api/routers"
	"mooc-manus/config"
	"mooc-manus/internal/domains/models/tracing"
	"mooc-manus/internal/infra/storage"
	"mooc-manus/pkg/logger"
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
	srv := &http.Server{Addr: ":8080", Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("start server failed: %v", err)
		}
	}()

	// 5. 实现服务的优雅退出：先停 HTTP server，再 flush tracer
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown err: %v", err)
	}
	if t := tracing.Global(); t != nil {
		if err := t.Shutdown(shutdownCtx); err != nil {
			log.Printf("tracer shutdown err: %v", err)
		}
	}
}
