package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"chrelyonly-email-server-go/internal/config"
	"chrelyonly-email-server-go/internal/model"
	"chrelyonly-email-server-go/internal/repository"
	"chrelyonly-email-server-go/pkg/utils"

	"go.uber.org/zap"
)

type Dispatcher struct {
	cfg        config.WebhookConfig
	repo       *repository.Repository
	logger     *zap.Logger
	httpClient *http.Client
	stopChan   chan struct{}
}

func NewDispatcher(cfg config.WebhookConfig, repo *repository.Repository, logger *zap.Logger) *Dispatcher {
	return &Dispatcher{
		cfg:        cfg,
		repo:       repo,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		stopChan:   make(chan struct{}),
	}
}

// DispatchEvent 提交一个 Webhook 事件到队列表
func (d *Dispatcher) DispatchEvent(eventType string, messageID string, data interface{}) error {
	if !d.cfg.Enabled || d.cfg.URL == "" {
		return nil
	}

	eventID := utils.GenerateID("evt")
	now := time.Now().Unix()

	payloadStruct := model.WebhookPayload{
		EventID:   eventID,
		Event:     eventType,
		Timestamp: now,
		Data:      data,
	}

	payloadBytes, err := json.Marshal(payloadStruct)
	if err != nil {
		return fmt.Errorf("序列化 Webhook 负载失败: %w", err)
	}

	evt := &model.Event{
		EventID:     eventID,
		EventType:   eventType,
		MessageID:   messageID,
		Payload:     string(payloadBytes),
		Status:      "pending",
		RetryCount:  0,
		MaxRetries:  d.cfg.MaxRetries,
		NextRetryAt: time.Now(),
	}

	if err := d.repo.CreateEvent(evt); err != nil {
		d.logger.Error("无法保存 Webhook 事件到数据库", zap.Error(err))
		return err
	}

	return nil
}

// StartWorker 启动后台 Webhook 轮询重试任务
func (d *Dispatcher) StartWorker() {
	if !d.cfg.Enabled || d.cfg.URL == "" {
		d.logger.Info("Webhook 未启用或未配置 URL，跳过 Worker 启动")
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	go func() {
		d.logger.Info("Webhook 重试 Worker 已启动", zap.String("url", d.cfg.URL))
		for {
			select {
			case <-ticker.C:
				d.processPendingEvents()
			case <-d.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop 停止 Worker
func (d *Dispatcher) Stop() {
	close(d.stopChan)
}

func (d *Dispatcher) processPendingEvents() {
	events, err := d.repo.GetPendingEvents(20)
	if err != nil {
		d.logger.Error("获取待推送 Webhook 事件失败", zap.Error(err))
		return
	}

	for _, evt := range events {
		d.sendSingleEvent(evt)
	}
}

func (d *Dispatcher) sendSingleEvent(evt model.Event) {
	timestamp := time.Now().Unix()
	bodyBytes := []byte(evt.Payload)

	// 计算签名: HMAC-SHA256(secret, timestamp.payload)
	signContent := fmt.Sprintf("%d.%s", timestamp, string(bodyBytes))
	mac := hmac.New(sha256.New, []byte(d.cfg.Secret))
	mac.Write([]byte(signContent))
	signature := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))

	req, err := http.NewRequest("POST", d.cfg.URL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		d.handleFailed(evt, fmt.Sprintf("创建 HTTP 请求失败: %v", err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mail-Event", evt.EventType)
	req.Header.Set("X-Mail-Event-Id", evt.EventID)
	req.Header.Set("X-Mail-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Mail-Signature", signature)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		d.handleFailed(evt, fmt.Sprintf("网络请求异常: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		d.logger.Info("Webhook 事件推送成功", zap.String("eventId", evt.EventID), zap.String("event", evt.EventType))
		_ = d.repo.UpdateEventStatus(evt.ID, "sent", evt.RetryCount, time.Now().Format("2006-01-02 15:04:05"), "")
	} else {
		d.handleFailed(evt, fmt.Sprintf("Spring Boot 返回 HTTP 响应码: %d", resp.StatusCode))
	}
}

func (d *Dispatcher) handleFailed(evt model.Event, errMsg string) {
	newRetry := evt.RetryCount + 1
	status := "failed"
	if newRetry >= evt.MaxRetries {
		d.logger.Warn("Webhook 事件达到最大重试次数, 标记最终失败", zap.String("eventId", evt.EventID), zap.String("err", errMsg))
	} else {
		d.logger.Warn("Webhook 事件推送失败, 将重试", zap.String("eventId", evt.EventID), zap.Int("retry", newRetry), zap.String("err", errMsg))
	}

	// 计算下一次重试间隔 (1m, 5m, 15m, 30m, 1h, 2h)
	delay := d.calculateBackoff(newRetry)
	nextTime := time.Now().Add(delay).Format("2006-01-02 15:04:05")

	_ = d.repo.UpdateEventStatus(evt.ID, status, newRetry, nextTime, errMsg)
}

func (d *Dispatcher) calculateBackoff(retryCount int) time.Duration {
	switch retryCount {
	case 1:
		return 1 * time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 15 * time.Minute
	case 4:
		return 30 * time.Minute
	case 5:
		return 1 * time.Hour
	default:
		return 2 * time.Hour
	}
}
