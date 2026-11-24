package smtp

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"chrelyonly-email-server-go/pkg/config"
	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/storage"
	"chrelyonly-email-server-go/pkg/webhook"
	"github.com/emersion/go-smtp"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SMTPServerManager struct {
	cfg     config.SMTPConfig
	backend *Backend
	servers []*smtp.Server
}

func NewSMTPServerManager(cfg config.SMTPConfig, db *gorm.DB, storage *storage.StorageManager, webhook *webhook.WebhookManager) *SMTPServerManager {
	backend := NewBackend(db, storage, webhook)
	return &SMTPServerManager{
		cfg:     cfg,
		backend: backend,
	}
}

func (m *SMTPServerManager) Start() error {
	ports := map[string]int{
		"SMTP":       m.cfg.Ports.SMTP,
		"Submission": m.cfg.Ports.Submission,
	}
	if m.cfg.Ports.SMTPS > 0 {
		ports["SMTPS"] = m.cfg.Ports.SMTPS
	}

	for name, port := range ports {
		if port <= 0 {
			continue
		}

		s := smtp.NewServer(m.backend)
		s.Addr = fmt.Sprintf(":%d", port)
		s.Domain = m.cfg.Hostname
		s.ReadTimeout = 60 * time.Second
		s.WriteTimeout = 60 * time.Second
		s.MaxMessageBytes = 26214400 // 25MB
		s.MaxRecipients = 100
		s.AllowInsecureAuth = true

		if m.cfg.TLS.Enabled && m.cfg.TLS.CertFile != "" && m.cfg.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(m.cfg.TLS.CertFile, m.cfg.TLS.KeyFile)
			if err == nil {
				s.TLSConfig = &tls.Config{
					Certificates: []tls.Certificate{cert},
				}
			} else {
				logger.Log.Warn("加载 SMTP TLS 证书失败", zap.Error(err))
			}
		}

		m.servers = append(m.servers, s)

		go func(srv *smtp.Server, pName string, pNum int) {
			logger.Log.Info(fmt.Sprintf("正在启动 SMTP 服务 (%s)，监听端口 %d", pName, pNum))
			l, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				logger.Log.Error(fmt.Sprintf("监听 SMTP 端口 %d 失败", pNum), zap.Error(err))
				return
			}
			if err := srv.Serve(l); err != nil {
				logger.Log.Error(fmt.Sprintf("SMTP 服务 (%s) 已停止", pName), zap.Error(err))
			}
		}(s, name, port)
	}

	return nil
}

func (m *SMTPServerManager) Stop() {
	for _, s := range m.servers {
		s.Close()
	}
}
