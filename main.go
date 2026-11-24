package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chrelyonly-email-server-go/internal/api"
	"chrelyonly-email-server-go/internal/config"
	"chrelyonly-email-server-go/internal/database"
	"chrelyonly-email-server-go/internal/delivery"
	"chrelyonly-email-server-go/internal/logger"
	"chrelyonly-email-server-go/internal/queue"
	"chrelyonly-email-server-go/internal/repository"
	smtpserver "chrelyonly-email-server-go/internal/smtp"
	"chrelyonly-email-server-go/internal/storage"
	"chrelyonly-email-server-go/internal/webhook"

	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	cfg, err := config.LoadConfig("")
	if err != nil {
		panic("加载配置文件失败: " + err.Error())
	}

	// 2. 初始化日志
	log, err := logger.InitLogger(cfg.Logger)
	if err != nil {
		panic("初始化日志组件失败: " + err.Error())
	}
	defer log.Sync()

	log.Info("正在启动 Chrelyonly Mail Server 🍓...")

	// 3. 初始化数据库连接（支持失败自动重试）
	db, err := database.InitDB(cfg.Database, log)
	if err != nil {
		log.Fatal("建立数据库连接失败", zap.Error(err))
	}
	defer db.Close()
	log.Info("数据库连接就绪", zap.String("driver", cfg.Database.Driver))

	// 4. 初始化文件存储
	storageMgr, err := storage.NewStorageManager(cfg.Storage)
	if err != nil {
		log.Fatal("初始化文件存储目录失败", zap.Error(err))
	}
	log.Info("文件存储目录准备完毕")

	// 5. 初始化 Repository 数据访问层
	repo := repository.NewRepository(db)

	// 6. 初始化 Webhook 派发器并启动后台 Worker
	dispatcher := webhook.NewDispatcher(cfg.Webhook, repo, log)
	dispatcher.StartWorker()
	defer dispatcher.Stop()

	// 7. 初始化 SMTP 发送器与发送队列 Worker
	sender := delivery.NewSender(cfg.SMTP, log)
	queueWorker := queue.NewQueueWorker(repo, storageMgr, sender, dispatcher, log)
	queueWorker.Start()
	defer queueWorker.Stop()

	// 8. 启动 SMTP 服务端 (支持 25, 465, 587)
	smtpManager := smtpserver.NewSMTPServerManager(cfg.SMTP, cfg.Storage, repo, storageMgr, dispatcher, log)
	if err := smtpManager.Start(); err != nil {
		log.Error("启动 SMTP 服务端失败", zap.Error(err))
	}
	defer smtpManager.Stop()

	// 9. 启动内部 HTTP API 服务
	apiHandler := api.NewAPIHandler(repo, storageMgr, dispatcher, db, log)
	router := api.SetupRouter(apiHandler)

	httpServer := &http.Server{
		Addr:         cfg.Server.HTTPAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Info("内部 REST API 服务已启动", zap.String("addr", cfg.Server.HTTPAddr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("内部 REST API 启动失败", zap.Error(err))
		}
	}()

	// 10. 优雅退出处理
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("接收到退出信号，准备优雅关闭服务...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("关闭 HTTP API 异常", zap.Error(err))
	}

	log.Info("Chrelyonly Mail Server 已安全关闭！")
}
