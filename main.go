package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"chrelyonly-email-server-go/pkg/api"
	"chrelyonly-email-server-go/pkg/config"
	"chrelyonly-email-server-go/pkg/db"
	"chrelyonly-email-server-go/pkg/imap"
	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/models"
	"chrelyonly-email-server-go/pkg/smtp"
	"chrelyonly-email-server-go/pkg/storage"
	"chrelyonly-email-server-go/pkg/webhook"

	"go.uber.org/zap"
)

func main() {
	configPath := "configs/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.InitLogger(cfg.Logger)
	if err != nil {
		fmt.Printf("初始化日志器失败: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("正在启动 Chrelyonly 邮件服务器...", zap.String("服务名", cfg.Server.Name))

	database, err := db.InitDB(cfg.Database)
	if err != nil {
		log.Fatal("初始化数据库失败", zap.Error(err))
	}

	var userCount int64
	database.Model(&models.User{}).Count(&userCount)
	if userCount == 0 {
		defaultUser := models.User{
			Username:   "admin",
			Email:      fmt.Sprintf("admin@%s", cfg.SMTP.Hostname),
			IsActive:   true,
			QuotaBytes: 1073741824,
		}
		_ = defaultUser.SetPassword("123456")
		tx := database.Begin()
		if err := tx.Create(&defaultUser).Error; err == nil {
			_ = db.CreateDefaultMailboxes(tx, defaultUser.ID)
			tx.Commit()
			log.Info("已自动创建默认测试用户", zap.String("邮箱", defaultUser.Email), zap.String("初始密码", "123456"))
		} else {
			tx.Rollback()
		}
	}

	storageMgr, err := storage.NewStorageManager(cfg.Storage)
	if err != nil {
		log.Fatal("初始化存储管理器失败", zap.Error(err))
	}

	webhookMgr := webhook.NewWebhookManager(cfg.Webhook, database)

	smtpMgr := smtp.NewSMTPServerManager(cfg.SMTP, database, storageMgr, webhookMgr)
	if err := smtpMgr.Start(); err != nil {
		log.Fatal("启动 SMTP 服务失败", zap.Error(err))
	}

	imapMgr := imap.NewIMAPServerManager(cfg.IMAP, database, storageMgr)
	if err := imapMgr.Start(); err != nil {
		log.Fatal("启动 IMAP 服务失败", zap.Error(err))
	}

	httpMgr := api.NewHTTPServerManager(cfg.Server, database, storageMgr, webhookMgr)
	if err := httpMgr.Start(); err != nil {
		log.Fatal("启动 HTTP 服务失败", zap.Error(err))
	}

	log.Info("Chrelyonly 邮件服务器启动成功，运行中...")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("正在关闭 Chrelyonly 邮件服务器...")
	httpMgr.Stop()
	imapMgr.Stop()
	smtpMgr.Stop()
	log.Info("邮件服务器已安全退出。")
}
