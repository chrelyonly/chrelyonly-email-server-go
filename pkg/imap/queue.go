package imap

import (
	"sync"
	"time"

	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/models"
	"go.uber.org/zap"
)

type UpdateQueueManager struct {
	backend *Backend
	queues  map[string]chan queuedEmail
	mu      sync.Mutex
}

type queuedEmail struct {
	mailboxName string
	email       models.Email
}

var QueueMgr *UpdateQueueManager

func InitUpdateQueueManager(b *Backend) {
	QueueMgr = &UpdateQueueManager{
		backend: b,
		queues:  make(map[string]chan queuedEmail),
	}
}

func (m *UpdateQueueManager) Push(username, mailboxName string, email models.Email) {
	m.mu.Lock()
	ch, exists := m.queues[username]
	if !exists {
		ch = make(chan queuedEmail, 1000)
		m.queues[username] = ch
		go m.worker(username, ch)
	}
	m.mu.Unlock()

	select {
	case ch <- queuedEmail{mailboxName: mailboxName, email: email}:
	default:
		logger.Log.Warn("用户消息推送队列已满", zap.String("用户", username))
	}
}

func (m *UpdateQueueManager) worker(username string, ch chan queuedEmail) {
	for item := range ch {
		m.backend.NotifyNewEmailDirect(username, item.mailboxName, item.email)
		// 队列一套一套平滑推送，避免并发冲击客户端
		time.Sleep(150 * time.Millisecond)
	}
}
