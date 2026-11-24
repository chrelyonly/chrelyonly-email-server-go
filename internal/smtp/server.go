package smtp

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"chrelyonly-email-server-go/internal/config"
	mimeParser "chrelyonly-email-server-go/internal/mime"
	"chrelyonly-email-server-go/internal/model"
	"chrelyonly-email-server-go/internal/repository"
	"chrelyonly-email-server-go/internal/security"
	"chrelyonly-email-server-go/internal/storage"
	"chrelyonly-email-server-go/internal/webhook"
	"chrelyonly-email-server-go/pkg/utils"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
	"go.uber.org/zap"
)

type SMTPServerManager struct {
	cfg        config.SMTPConfig
	storageCfg config.StorageConfig
	repo       *repository.Repository
	storage    *storage.StorageManager
	dispatcher *webhook.Dispatcher
	logger     *zap.Logger
	servers    []*gosmtp.Server
}

func NewSMTPServerManager(
	cfg config.SMTPConfig,
	storageCfg config.StorageConfig,
	repo *repository.Repository,
	storage *storage.StorageManager,
	dispatcher *webhook.Dispatcher,
	logger *zap.Logger,
) *SMTPServerManager {
	return &SMTPServerManager{
		cfg:        cfg,
		storageCfg: storageCfg,
		repo:       repo,
		storage:    storage,
		dispatcher: dispatcher,
		logger:     logger,
	}
}

// Start 启动 SMTP 监听服务 (端口 25, 465, 587)
func (m *SMTPServerManager) Start() error {
	backend := &Backend{
		manager: m,
	}

	var tlsCert *tls.Certificate
	if m.cfg.TLS.Enabled && m.cfg.TLS.CertFile != "" && m.cfg.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(m.cfg.TLS.CertFile, m.cfg.TLS.KeyFile)
		if err == nil {
			tlsCert = &cert
			m.logger.Info("TLS 证书已加载", zap.String("cert", m.cfg.TLS.CertFile))
		} else {
			m.logger.Warn("加载 TLS 证书失败", zap.Error(err))
		}
	}

	// 监听 Standard SMTP (Port 25)
	if m.cfg.Ports.SMTP > 0 {
		s25 := m.createSMTPServer(backend, tlsCert)
		s25.Addr = fmt.Sprintf(":%d", m.cfg.Ports.SMTP)
		m.servers = append(m.servers, s25)
		go func() {
			m.logger.Info("SMTP 服务开始监听", zap.String("addr", s25.Addr))
			if err := s25.ListenAndServe(); err != nil {
				m.logger.Error("SMTP 25 服务挂掉", zap.Error(err))
			}
		}()
	}

	// 监听 Submission (Port 587)
	if m.cfg.Ports.Submission > 0 {
		s587 := m.createSMTPServer(backend, tlsCert)
		s587.Addr = fmt.Sprintf(":%d", m.cfg.Ports.Submission)
		s587.AllowInsecureAuth = true
		m.servers = append(m.servers, s587)
		go func() {
			m.logger.Info("SMTP Submission 服务开始监听", zap.String("addr", s587.Addr))
			if err := s587.ListenAndServe(); err != nil {
				m.logger.Error("SMTP 587 服务挂掉", zap.Error(err))
			}
		}()
	}

	// 监听 Implicit SMTPS (Port 465)
	if m.cfg.Ports.SMTPS > 0 && tlsCert != nil {
		s465 := m.createSMTPServer(backend, tlsCert)
		s465.Addr = fmt.Sprintf(":%d", m.cfg.Ports.SMTPS)
		m.servers = append(m.servers, s465)
		go func() {
			m.logger.Info("SMTPS 465 隐式 TLS 服务开始监听", zap.String("addr", s465.Addr))
			if err := s465.ListenAndServeTLS(); err != nil {
				m.logger.Error("SMTPS 465 服务挂掉", zap.Error(err))
			}
		}()
	}

	return nil
}

func (m *SMTPServerManager) createSMTPServer(backend *Backend, cert *tls.Certificate) *gosmtp.Server {
	s := gosmtp.NewServer(backend)
	s.Domain = m.cfg.Hostname
	s.ReadTimeout = 30 * time.Second
	s.WriteTimeout = 30 * time.Second
	s.MaxMessageBytes = m.storageCfg.MaxMessageSize
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	if cert != nil {
		s.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{*cert},
		}
	}
	return s
}

func (m *SMTPServerManager) Stop() {
	for _, s := range m.servers {
		_ = s.Close()
	}
}

// --- Backend ---

type Backend struct {
	manager *SMTPServerManager
}

func (b *Backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	remoteIP := ""
	if c != nil && c.Conn() != nil {
		host, _, _ := net.SplitHostPort(c.Conn().RemoteAddr().String())
		remoteIP = host
	}

	return &Session{
		manager:  b.manager,
		remoteIP: remoteIP,
	}, nil
}

// --- Session ---

type Session struct {
	manager           *SMTPServerManager
	remoteIP          string
	authenticated     bool
	authenticatedUser string
	from              string
	recipients        []string
}

func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.Login}
}

func (s *Session) authenticate(username, password string) error {
	acc, err := s.manager.repo.GetAccountByEmail(username)
	if err != nil || acc == nil {
		s.recordAuthLog(username, false, "Account not found")
		return gosmtp.ErrAuthFailed
	}

	if !acc.IsActive {
		s.recordAuthLog(username, false, "Account disabled")
		return gosmtp.ErrAuthFailed
	}

	if !security.VerifyPassword(password, acc.PasswordHash) {
		s.recordAuthLog(username, false, "Invalid password")
		return gosmtp.ErrAuthFailed
	}

	s.authenticated = true
	s.authenticatedUser = acc.Email
	s.recordAuthLog(username, true, "Success")
	return nil
}

// loginServer 自定义 SASL LOGIN 机制服务端实现
type loginServer struct {
	step          int
	username      string
	authenticator func(username, password string) error
}

func newLoginServer(authenticator func(username, password string) error) sasl.Server {
	return &loginServer{authenticator: authenticator}
}

func (s *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch s.step {
	case 0:
		s.step = 2
		if len(response) > 0 {
			s.username = string(response)
			return []byte("UGFzc3dvcmQ6"), false, nil
		}
		s.step = 1
		return []byte("VXNlcm5hbWU6"), false, nil
	case 1:
		s.username = string(response)
		s.step = 2
		return []byte("UGFzc3dvcmQ6"), false, nil
	case 2:
		password := string(response)
		err := s.authenticator(s.username, password)
		return nil, true, err
	default:
		return nil, true, sasl.ErrUnexpectedClientResponse
	}
}

func (s *Session) Auth(mech string, saslServer sasl.Server) error {
	var server sasl.Server

	switch mech {
	case sasl.Plain:
		server = sasl.NewPlainServer(func(identity, username, password string) error {
			return s.authenticate(username, password)
		})
	case sasl.Login:
		server = newLoginServer(func(username, password string) error {
			return s.authenticate(username, password)
		})
	default:
		return gosmtp.ErrAuthUnsupported
	}

	var toServer []byte
	for {
		fromServer, done, err := saslServer.Next(toServer)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		toServer, done, err = server.Next(fromServer)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

func (s *Session) recordAuthLog(username string, success bool, reason string) {
	_ = s.manager.repo.CreateAuthLog(&model.AuthLog{
		Username:  username,
		IPAddress: s.remoteIP,
		Success:   success,
		Reason:    reason,
	})
}

func (s *Session) Mail(from string, opts *gosmtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *Session) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	recipientDomain := utils.ExtractDomainFromEmail(to)
	if recipientDomain == "" {
		return &gosmtp.SMTPError{
			Code:         550,
			EnhancedCode: gosmtp.EnhancedCode{5, 1, 1},
			Message:      "Invalid recipient address",
		}
	}

	// 验证目标域名是否属于本服务器管理
	domainObj, err := s.manager.repo.GetDomainByName(recipientDomain)

	// 防止开放中继 (Open Relay): 未认证用户只能向本地管理域名账户投递
	if !s.authenticated {
		if err != nil || domainObj == nil || !domainObj.IsActive {
			return &gosmtp.SMTPError{
				Code:         550,
				EnhancedCode: gosmtp.EnhancedCode{5, 7, 1},
				Message:      "Relay access denied. Local domain required for unauthenticated delivery.",
			}
		}

		acc, err := s.manager.repo.GetAccountByEmail(to)
		if err != nil || acc == nil || !acc.IsActive {
			return &gosmtp.SMTPError{
				Code:         550,
				EnhancedCode: gosmtp.EnhancedCode{5, 1, 1},
				Message:      "Recipient address rejected: User unknown in local directory",
			}
		}
	}

	s.recipients = append(s.recipients, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	rawBytes, err := io.ReadAll(r)
	if err != nil {
		return &gosmtp.SMTPError{
			Code:         451,
			EnhancedCode: gosmtp.EnhancedCode{4, 3, 0},
			Message:      "Error reading message content",
		}
	}

	// 1. 持久化保存原始 EML 文件
	relPath, err := s.manager.storage.SaveRawMessage(rawBytes)
	if err != nil {
		return &gosmtp.SMTPError{
			Code:         451,
			EnhancedCode: gosmtp.EnhancedCode{4, 3, 0},
			Message:      "Error saving raw message",
		}
	}

	// 2. MIME 解析邮件
	parsed, err := mimeParser.ParseMessage(rawBytes)
	if err != nil {
		parsed = &mimeParser.ParsedEmail{
			MessageID: utils.GenerateMessageID("chrelyonly.cn"),
			From:      s.from,
			To:        s.recipients,
			Subject:   "(No Subject)",
		}
	}

	if parsed.From == "" {
		parsed.From = s.from
	}
	if len(parsed.To) == 0 {
		parsed.To = s.recipients
	}

	// 查找本地所属账户 ID
	var localAccountID *int64
	for _, rcpt := range s.recipients {
		acc, _ := s.manager.repo.GetAccountByEmail(rcpt)
		if acc != nil {
			localAccountID = &acc.ID
			break
		}
	}

	direction := "inbound"
	if s.authenticated {
		direction = "outbound"
	}

	// 3. 构建 Message 数据
	msg := &model.Message{
		MessageID:      parsed.MessageID,
		AccountID:      localAccountID,
		FromAddress:    parsed.From,
		Subject:        parsed.Subject,
		TextContent:    parsed.TextContent,
		HTMLContent:    parsed.HTMLContent,
		RawFilePath:    relPath,
		SizeBytes:      int64(len(rawBytes)),
		HasAttachments: len(parsed.Attachments) > 0,
		IsRead:         false,
		Direction:      direction,
		Status:         "received",
		ReceivedAt:     time.Now(),
	}

	// 4. 保存附件
	var dbAttachments []model.Attachment
	for _, att := range parsed.Attachments {
		attRelPath, attSize, attErr := s.manager.storage.SaveAttachment(att.Filename, bytes.NewReader(att.Data))
		if attErr == nil {
			dbAttachments = append(dbAttachments, model.Attachment{
				Filename:    att.Filename,
				ContentType: att.ContentType,
				SizeBytes:   attSize,
				FilePath:    attRelPath,
				ContentID:   att.ContentID,
			})
		}
	}

	// 5. 构建收件人明细
	var dbRecipients []model.Recipient
	for _, to := range parsed.To {
		dbRecipients = append(dbRecipients, model.Recipient{RecipientType: "to", EmailAddress: to})
	}
	for _, cc := range parsed.Cc {
		dbRecipients = append(dbRecipients, model.Recipient{RecipientType: "cc", EmailAddress: cc})
	}

	// 6. 保存到数据库
	if err := s.manager.repo.SaveMessage(msg, dbRecipients, dbAttachments); err != nil {
		s.manager.logger.Error("保存收到的邮件到数据库失败", zap.Error(err))
		return &gosmtp.SMTPError{
			Code:         451,
			EnhancedCode: gosmtp.EnhancedCode{4, 3, 0},
			Message:      "Database storage error",
		}
	}

	s.manager.logger.Info("成功接收并保存新邮件", zap.String("messageId", msg.MessageID), zap.String("from", msg.FromAddress))

	// 7. 派发 Webhook 事件 mail.received
	_ = s.manager.dispatcher.DispatchEvent("mail.received", msg.MessageID, map[string]interface{}{
		"messageId":      msg.MessageID,
		"from":           msg.FromAddress,
		"to":             parsed.To,
		"subject":        msg.Subject,
		"hasAttachments": msg.HasAttachments,
		"receivedAt":     msg.ReceivedAt.Format(time.RFC3339),
	})

	return nil
}

func (s *Session) Reset() {
	s.from = ""
	s.recipients = nil
}

func (s *Session) Logout() error {
	return nil
}
