package queue

import (
	"time"

	"chrelyonly-email-server-go/internal/delivery"
	"chrelyonly-email-server-go/internal/dkim"
	"chrelyonly-email-server-go/internal/model"
	"chrelyonly-email-server-go/internal/repository"
	"chrelyonly-email-server-go/internal/storage"
	"chrelyonly-email-server-go/internal/webhook"
	"chrelyonly-email-server-go/pkg/utils"

	"go.uber.org/zap"
)

type QueueWorker struct {
	repo       *repository.Repository
	storage    *storage.StorageManager
	sender     *delivery.Sender
	dispatcher *webhook.Dispatcher
	logger     *zap.Logger
	stopChan   chan struct{}
}

func NewQueueWorker(
	repo *repository.Repository,
	storage *storage.StorageManager,
	sender *delivery.Sender,
	dispatcher *webhook.Dispatcher,
	logger *zap.Logger,
) *QueueWorker {
	return &QueueWorker{
		repo:       repo,
		storage:    storage,
		sender:     sender,
		dispatcher: dispatcher,
		logger:     logger,
		stopChan:   make(chan struct{}),
	}
}

func (w *QueueWorker) Start() {
	ticker := time.NewTicker(3 * time.Second)
	go func() {
		w.logger.Info("邮件发送队列 Queue Worker 已启动")
		for {
			select {
			case <-ticker.C:
				w.processQueue()
			case <-w.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (w *QueueWorker) Stop() {
	close(w.stopChan)
}

func (w *QueueWorker) processQueue() {
	items, err := w.repo.GetPendingQueueItems(10)
	if err != nil {
		w.logger.Error("读取发送队列失败", zap.Error(err))
		return
	}

	for _, item := range items {
		w.processItem(item)
	}
}

func (w *QueueWorker) processItem(item model.QueueItem) {
	// 标记状态为 processing
	_ = w.repo.UpdateQueueStatus(item.ID, "processing", item.RetryCount, time.Now().Format("2006-01-02 15:04:05"), "")

	msg, err := w.repo.GetMessageByMessageID(item.MessageID)
	if err != nil || msg == nil {
		w.logger.Error("未找到队列对应的邮件记录", zap.String("messageId", item.MessageID))
		_ = w.repo.UpdateQueueStatus(item.ID, "failed", item.RetryCount, time.Now().Format("2006-01-02 15:04:05"), "Message record not found")
		return
	}

	rawEML, err := w.storage.ReadRawMessage(msg.RawFilePath)
	if err != nil {
		w.logger.Error("读取原始邮件文件失败", zap.String("path", msg.RawFilePath), zap.Error(err))
		_ = w.repo.UpdateQueueStatus(item.ID, "failed", item.RetryCount, time.Now().Format("2006-01-02 15:04:05"), err.Error())
		return
	}

	// 检查并添加 DKIM 签名
	senderDomain := utils.ExtractDomainFromEmail(item.FromAddress)
	if domainObj, _ := w.repo.GetDomainByName(senderDomain); domainObj != nil && domainObj.DkimPrivateKey != "" {
		signedBytes, err := dkim.SignMessage(domainObj.Domain, domainObj.DkimSelector, domainObj.DkimPrivateKey, rawEML)
		if err == nil {
			rawEML = signedBytes
		} else {
			w.logger.Warn("DKIM 签名计算失败，使用原文发送", zap.Error(err))
		}
	}

	// 尝试向远端 MX 投递
	result, err := w.sender.DeliverMail(item.FromAddress, item.ToAddress, rawEML)

	queueLog := &model.DeliveryLog{
		QueueID:   &item.ID,
		MessageID: item.MessageID,
		MXHost:    "",
		SMTPCode:  0,
		Response:  "",
	}
	if result != nil {
		queueLog.MXHost = result.MXHost
		queueLog.SMTPCode = result.SMTPCode
		queueLog.Response = result.Response
	}

	if err == nil {
		// 投递成功
		queueLog.Status = "success"
		_ = w.repo.CreateDeliveryLog(queueLog)
		_ = w.repo.UpdateQueueStatus(item.ID, "sent", item.RetryCount, time.Now().Format("2006-01-02 15:04:05"), "")

		w.logger.Info("邮件成功送到远端服务器", zap.String("messageId", item.MessageID), zap.String("to", item.ToAddress))

		// 派发 mail.sent 事件
		_ = w.dispatcher.DispatchEvent("mail.sent", item.MessageID, map[string]interface{}{
			"messageId": item.MessageID,
			"from":      item.FromAddress,
			"to":        []string{item.ToAddress},
			"mxHost":    queueLog.MXHost,
			"sentAt":    time.Now().Format(time.RFC3339),
		})

	} else {
		// 投递失败，处理重试逻辑
		queueLog.Status = "failed"
		_ = w.repo.CreateDeliveryLog(queueLog)

		newRetry := item.RetryCount + 1
		errMsg := err.Error()

		if newRetry >= item.MaxRetries {
			w.logger.Error("邮件投递达到最大重试次数，标记失败", zap.String("messageId", item.MessageID), zap.String("to", item.ToAddress))
			_ = w.repo.UpdateQueueStatus(item.ID, "failed", newRetry, time.Now().Format("2006-01-02 15:04:05"), errMsg)

			// 派发 mail.failed 事件
			_ = w.dispatcher.DispatchEvent("mail.failed", item.MessageID, map[string]interface{}{
				"messageId": item.MessageID,
				"from":      item.FromAddress,
				"to":        []string{item.ToAddress},
				"reason":    errMsg,
				"failedAt":  time.Now().Format(time.RFC3339),
			})
		} else {
			delay := w.calculateBackoff(newRetry)
			nextTime := time.Now().Add(delay).Format("2006-01-02 15:04:05")
			w.logger.Warn("邮件投递失败，将进行下一次重试", zap.String("messageId", item.MessageID), zap.Int("retry", newRetry), zap.String("next", nextTime))

			_ = w.repo.UpdateQueueStatus(item.ID, "pending", newRetry, nextTime, errMsg)
		}
	}
}

func (w *QueueWorker) calculateBackoff(retryCount int) time.Duration {
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
