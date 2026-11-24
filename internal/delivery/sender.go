package delivery

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"chrelyonly-email-server-go/internal/config"
	"chrelyonly-email-server-go/pkg/utils"

	"go.uber.org/zap"
)

type DeliveryResult struct {
	MXHost   string
	SMTPCode int
	Response string
}

type Sender struct {
	cfg    config.SMTPConfig
	logger *zap.Logger
}

func NewSender(cfg config.SMTPConfig, logger *zap.Logger) *Sender {
	return &Sender{
		cfg:    cfg,
		logger: logger,
	}
}

// DeliverMail 执行 MX 记录查询与 SMTP 远程投递
func (s *Sender) DeliverMail(from string, to string, rawEML []byte) (*DeliveryResult, error) {
	targetDomain := utils.ExtractDomainFromEmail(to)
	if targetDomain == "" {
		return nil, fmt.Errorf("无效的目标收件人地址: %s", to)
	}

	// 1. 查询目标域名的 MX 记录
	mxHost, err := s.lookupMX(targetDomain)
	if err != nil {
		return nil, fmt.Errorf("查找 MX 记录失败 [%s]: %w", targetDomain, err)
	}

	s.logger.Info("准备向 MX 服务器发起投递", zap.String("to", to), zap.String("mxHost", mxHost))

	result := &DeliveryResult{
		MXHost: mxHost,
	}

	// 2. 建立 TCP 连接 (默认端口 25)
	addr := net.JoinHostPort(mxHost, "25")
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		result.Response = fmt.Sprintf("TCP Dial Failed: %v", err)
		return result, err
	}
	defer conn.Close()

	// 3. 创建 SMTP 客户端
	client, err := smtp.NewClient(conn, mxHost)
	if err != nil {
		result.Response = fmt.Sprintf("SMTP NewClient Failed: %v", err)
		return result, err
	}
	defer client.Quit()

	hostname := s.cfg.Hostname
	if hostname == "" {
		hostname = "mail.localhost"
	}
	if err := client.Hello(hostname); err != nil {
		result.Response = fmt.Sprintf("HELO Failed: %v", err)
		return result, err
	}

	// 4. 尝试 Upgrade STARTTLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, // 允许自签名或公网通用证书
			ServerName:         mxHost,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			s.logger.Warn("STARTTLS 协商失败，尝试继续明文传输", zap.String("mxHost", mxHost), zap.Error(err))
		}
	}

	// 5. 设置 MAIL FROM
	if err := client.Mail(from); err != nil {
		result.Response = fmt.Sprintf("MAIL FROM Failed: %v", err)
		return result, err
	}

	// 6. 设置 RCPT TO
	if err := client.Rcpt(to); err != nil {
		result.Response = fmt.Sprintf("RCPT TO Failed: %v", err)
		return result, err
	}

	// 7. 发送 DATA
	w, err := client.Data()
	if err != nil {
		result.Response = fmt.Sprintf("DATA command Failed: %v", err)
		return result, err
	}

	_, err = w.Write(rawEML)
	if err != nil {
		_ = w.Close()
		result.Response = fmt.Sprintf("Write Data Failed: %v", err)
		return result, err
	}

	if err := w.Close(); err != nil {
		result.Response = fmt.Sprintf("Close Data Writer Failed: %v", err)
		return result, err
	}

	result.SMTPCode = 250
	result.Response = "250 2.0.0 OK Mail Accepted"
	return result, nil
}

// lookupMX 查找最高优 MX 记录
func (s *Sender) lookupMX(domain string) (string, error) {
	mxs, err := net.LookupMX(domain)
	if err == nil && len(mxs) > 0 {
		// 选择 Preference 最小的记录
		bestMX := mxs[0].Host
		minPref := mxs[0].Pref
		for _, mx := range mxs {
			if mx.Pref < minPref {
				minPref = mx.Pref
				bestMX = mx.Host
			}
		}
		return strings.TrimSuffix(bestMX, "."), nil
	}

	// 如果没有找到 MX 记录，回退到域名直接地址
	return domain, nil
}
