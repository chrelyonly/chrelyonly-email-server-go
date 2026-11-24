package smtp

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"chrelyonly-email-server-go/pkg/logger"
	"go.uber.org/zap"
)

type MailRelayer struct{}

func NewMailRelayer() *MailRelayer {
	return &MailRelayer{}
}

func (r *MailRelayer) RelayMail(from string, to string, rawData []byte) error {
	parts := strings.Split(to, "@")
	if len(parts) != 2 {
		return fmt.Errorf("无效的收件人地址: %s", to)
	}
	domain := parts[1]

	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		return fmt.Errorf("查询域名 %s 的 MX 记录失败: %v", domain, err)
	}

	var lastErr error
	for _, mx := range mxRecords {
		host := strings.TrimSuffix(mx.Host, ".")
		addr := fmt.Sprintf("%s:25", host)

		logger.Log.Info("正在尝试 SMTP 中继转发", zap.String("MX", host), zap.String("收件人", to))
		err := smtp.SendMail(addr, nil, from, []string{to}, rawData)
		if err == nil {
			logger.Log.Info("SMTP 中继转发成功", zap.String("收件人", to), zap.String("MX", host))
			return nil
		}
		lastErr = err
		logger.Log.Warn("SMTP 中继转发到主机失败", zap.String("MX", host), zap.Error(err))
	}

	return fmt.Errorf("域 %s 的所有 MX 服务器转发均失败: %v", domain, lastErr)
}
