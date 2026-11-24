package api

import (
	"net/http"

	"chrelyonly-email-server-go/pkg/config"
	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/storage"
	"chrelyonly-email-server-go/pkg/webhook"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type HTTPServerManager struct {
	cfg    config.ServerConfig
	router *gin.Engine
	srv    *http.Server
}

func NewHTTPServerManager(cfg config.ServerConfig, db *gorm.DB, storage *storage.StorageManager, webhook *webhook.WebhookManager) *HTTPServerManager {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ginLoggerMiddleware())

	sysHandler := NewSystemHandler(db)
	userHandler := NewUserHandler(db)
	emailHandler := NewEmailHandler(db, storage, webhook)

	r.GET("/health", sysHandler.Health)

	apiV1 := r.Group("/api/v1")
	{
		apiV1.GET("/system/status", sysHandler.Status)

		apiV1.POST("/users", userHandler.CreateUser)
		apiV1.GET("/users", userHandler.ListUsers)
		apiV1.GET("/users/:id", userHandler.GetUser)
		apiV1.PUT("/users/:id/password", userHandler.UpdatePassword)
		apiV1.DELETE("/users/:id", userHandler.DeleteUser)

		apiV1.POST("/emails/send", emailHandler.SendEmail)
		apiV1.GET("/emails", emailHandler.ListEmails)
		apiV1.GET("/emails/:id", emailHandler.GetEmail)
		apiV1.GET("/emails/:id/raw", emailHandler.DownloadRawEmail)
		apiV1.DELETE("/emails/:id", emailHandler.DeleteEmail)

		apiV1.GET("/attachments/:id/download", emailHandler.DownloadAttachment)

		apiV1.POST("/webhooks/test", emailHandler.TestWebhook)
		apiV1.GET("/webhooks/logs", emailHandler.ListWebhookLogs)
	}

	return &HTTPServerManager{
		cfg:    cfg,
		router: r,
	}
}

func (m *HTTPServerManager) Start() error {
	m.srv = &http.Server{
		Addr:    m.cfg.HTTPAddr,
		Handler: m.router,
	}

	go func() {
		logger.Log.Info("正在启动 HTTP 服务", zap.String("地址", m.cfg.HTTPAddr))
		if err := m.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Error("HTTP 服务发生错误停止", zap.Error(err))
		}
	}()

	return nil
}

func (m *HTTPServerManager) Stop() {
	if m.srv != nil {
		m.srv.Close()
	}
}

func ginLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		logger.Log.Info("HTTP 请求",
			zap.String("方法", c.Request.Method),
			zap.String("路径", c.Request.URL.Path),
			zap.Int("状态码", c.Writer.Status()),
			zap.String("客户端 IP", c.ClientIP()),
		)
	}
}
