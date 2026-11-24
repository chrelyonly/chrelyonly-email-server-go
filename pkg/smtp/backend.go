package smtp

import (
	"chrelyonly-email-server-go/pkg/storage"
	"chrelyonly-email-server-go/pkg/webhook"
	"github.com/emersion/go-smtp"
	"gorm.io/gorm"
)

type Backend struct {
	db      *gorm.DB
	storage *storage.StorageManager
	webhook *webhook.WebhookManager
	relayer *MailRelayer
}

func NewBackend(db *gorm.DB, storage *storage.StorageManager, webhook *webhook.WebhookManager) *Backend {
	return &Backend{
		db:      db,
		storage: storage,
		webhook: webhook,
		relayer: NewMailRelayer(),
	}
}

func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{
		db:      b.db,
		storage: b.storage,
		webhook: b.webhook,
		relayer: b.relayer,
	}, nil
}
