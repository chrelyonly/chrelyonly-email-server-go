package imap

import (
	"fmt"

	"chrelyonly-email-server-go/pkg/config"
	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/storage"
	"github.com/emersion/go-imap/server"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type IMAPServerManager struct {
	cfg     config.IMAPConfig
	backend *Backend
	server  *server.Server
}

func NewIMAPServerManager(cfg config.IMAPConfig, db *gorm.DB, storage *storage.StorageManager) *IMAPServerManager {
	backend := NewBackend(db, storage)
	return &IMAPServerManager{
		cfg:     cfg,
		backend: backend,
	}
}

func (m *IMAPServerManager) Start() error {
	if !m.cfg.Enabled {
		logger.Log.Info("IMAP 服务在配置中已禁用")
		return nil
	}

	s := server.New(m.backend)
	s.Addr = fmt.Sprintf(":%d", m.cfg.Port)
	s.AllowInsecureAuth = true

	m.server = s

	go func() {
		logger.Log.Info("正在启动 IMAP 服务", zap.Int("端口", m.cfg.Port))
		if err := s.ListenAndServe(); err != nil {
			logger.Log.Error("IMAP 服务已停止", zap.Error(err))
		}
	}()

	return nil
}

func (m *IMAPServerManager) Stop() {
	if m.server != nil {
		m.server.Close()
	}
}
