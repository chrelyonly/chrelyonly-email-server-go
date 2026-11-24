package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"chrelyonly-email-server-go/pkg/config"
	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type WebhookPayload struct {
	Event     string       `json:"event"`
	Timestamp int64        `json:"timestamp"`
	Data      models.Email `json:"data"`
}

type WebhookManager struct {
	cfg config.WebhookConfig
	db  *gorm.DB
}

func NewWebhookManager(cfg config.WebhookConfig, db *gorm.DB) *WebhookManager {
	return &WebhookManager{
		cfg: cfg,
		db:  db,
	}
}

func (w *WebhookManager) TriggerEmailReceived(email models.Email) {
	if !w.cfg.Enabled || w.cfg.URL == "" {
		return
	}
	go w.sendEvent("email.received", email)
}

func (w *WebhookManager) TriggerEmailSent(email models.Email) {
	if !w.cfg.Enabled || w.cfg.URL == "" {
		return
	}
	go w.sendEvent("email.sent", email)
}

func (w *WebhookManager) sendEvent(eventType string, email models.Email) {
	payload := WebhookPayload{
		Event:     eventType,
		Timestamp: time.Now().Unix(),
		Data:      email,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Log.Error("序列化 Webhook 负载失败", zap.Error(err))
		return
	}

	maxRetries := 1
	if w.cfg.Retry && w.cfg.MaxRetries > 0 {
		maxRetries = w.cfg.MaxRetries
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", w.cfg.URL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			logger.Log.Error("创建 Webhook 请求失败", zap.Error(err))
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Chrelyonly-Mail-Server-Webhook/1.0")

		if w.cfg.Secret != "" {
			mac := hmac.New(sha256.New, []byte(w.cfg.Secret))
			mac.Write(bodyBytes)
			signature := hex.EncodeToString(mac.Sum(nil))
			req.Header.Set("X-Mail-Signature", signature)
		}

		resp, err := client.Do(req)
		var statusCode int
		var respStr string
		var errStr string

		if err != nil {
			errStr = err.Error()
		} else {
			statusCode = resp.StatusCode
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			respStr = string(b)
		}

		logRecord := models.WebhookLog{
			EventType:    eventType,
			EmailID:      email.ID,
			TargetURL:    w.cfg.URL,
			StatusCode:   statusCode,
			ResponseBody: respStr,
			Error:        errStr,
			RetryCount:   attempt - 1,
		}
		w.db.Create(&logRecord)

		if err == nil && statusCode >= 200 && statusCode < 300 {
			logger.Log.Info("Webhook 投递成功",
				zap.String("事件", eventType),
				zap.Int("状态码", statusCode),
				zap.Int("尝试次数", attempt))
			return
		}

		logger.Log.Warn("Webhook 投递失败，准备重试...",
			zap.String("事件", eventType),
			zap.Int("尝试次数", attempt),
			zap.String("错误", errStr))

		time.Sleep(time.Duration(attempt*2) * time.Second)
	}
}
