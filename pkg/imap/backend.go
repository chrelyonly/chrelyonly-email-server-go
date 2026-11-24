package imap

import (
	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/models"
	"chrelyonly-email-server-go/pkg/storage"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var GlobalBackend *Backend

type Backend struct {
	db      *gorm.DB
	storage *storage.StorageManager
	updates chan backend.Update
}

func NewBackend(db *gorm.DB, storage *storage.StorageManager) *Backend {
	b := &Backend{
		db:      db,
		storage: storage,
		updates: make(chan backend.Update, 100),
	}
	GlobalBackend = b
	InitUpdateQueueManager(b)
	return b
}

func (b *Backend) Updates() <-chan backend.Update {
	return b.updates
}

func (b *Backend) NotifyNewEmail(username, mailboxName string, email models.Email) {
	if QueueMgr != nil {
		QueueMgr.Push(username, mailboxName, email)
	} else {
		b.NotifyNewEmailDirect(username, mailboxName, email)
	}
}

func (b *Backend) NotifyNewEmailDirect(username, mailboxName string, email models.Email) {
	var user models.User
	if err := b.db.Where("email = ? OR username = ?", username, username).First(&user).Error; err != nil {
		return
	}

	var mb models.Mailbox
	if err := b.db.Where("user_id = ? AND name = ?", user.ID, mailboxName).First(&mb).Error; err != nil {
		return
	}

	var total int64
	b.db.Model(&models.Email{}).Where("mailbox_id = ? AND is_deleted = ?", mb.ID, false).Count(&total)

	var unseen int64
	b.db.Model(&models.Email{}).Where("mailbox_id = ? AND is_read = ? AND is_deleted = ?", mb.ID, false, false).Count(&unseen)

	// 1. Send MailboxUpdate (EXISTS)
	status := imap.NewMailboxStatus(mailboxName, []imap.StatusItem{
		imap.StatusMessages,
		imap.StatusUnseen,
		imap.StatusUidNext,
		imap.StatusUidValidity,
	})
	status.Messages = uint32(total)
	status.Unseen = uint32(unseen)
	status.UidNext = mb.UIDNext
	status.UidValidity = mb.UIDValidity
	status.Recent = 1

	mbUpdate := &backend.MailboxUpdate{
		Update:        backend.NewUpdate(username, mailboxName),
		MailboxStatus: status,
	}
	select {
	case b.updates <- mbUpdate:
	default:
	}

	// 2. Send MessageUpdate (FETCH push)
	msg := imap.NewMessage(uint32(total), []imap.FetchItem{
		imap.FetchUid,
		imap.FetchFlags,
		imap.FetchInternalDate,
		imap.FetchRFC822Size,
		imap.FetchEnvelope,
	})
	msg.Uid = email.UID
	msg.Size = uint32(email.Size)
	msg.InternalDate = email.ReceivedAt
	if email.IsRead {
		msg.Flags = []string{imap.SeenFlag}
	} else {
		msg.Flags = []string{}
	}
	msg.Envelope = &imap.Envelope{
		Date:      email.SentAt,
		Subject:   email.Subject,
		From:      parseAddressList(email.FromAddress),
		To:        parseAddressList(email.ToAddress),
		MessageId: email.MessageID,
	}

	msgUpdate := &backend.MessageUpdate{
		Update:  backend.NewUpdate(username, mailboxName),
		Message: msg,
	}
	select {
	case b.updates <- msgUpdate:
		logger.Log.Info("已通过队列一套一套平滑推送新邮件至客户端", zap.String("用户", username), zap.String("主题", email.Subject))
	default:
	}
}

func (b *Backend) NotifyUpdate(username, mailboxName string) {
	var user models.User
	if err := b.db.Where("email = ? OR username = ?", username, username).First(&user).Error; err != nil {
		return
	}
	var mb models.Mailbox
	if err := b.db.Where("user_id = ? AND name = ?", user.ID, mailboxName).First(&mb).Error; err == nil {
		var latestEmail models.Email
		if err := b.db.Where("mailbox_id = ?", mb.ID).Order("id DESC").First(&latestEmail).Error; err == nil {
			b.NotifyNewEmail(username, mailboxName, latestEmail)
		}
	}
}

func NotifyNewEmail(username, mailbox string, email models.Email) {
	if GlobalBackend != nil {
		GlobalBackend.NotifyNewEmail(username, mailbox, email)
	}
}

func NotifyGlobalUpdate(username, mailbox string) {
	if GlobalBackend != nil {
		GlobalBackend.NotifyUpdate(username, mailbox)
	}
}

func (b *Backend) Login(conn *imap.ConnInfo, username, password string) (backend.User, error) {
	var user models.User
	err := b.db.Where("email = ? OR username = ?", username, username).First(&user).Error
	if err != nil || !user.CheckPassword(password) {
		logger.Log.Warn("IMAP 登录失败", zap.String("用户名", username))
		return nil, backend.ErrInvalidCredentials
	}

	if !user.IsActive {
		logger.Log.Warn("IMAP 登录被拒绝：账号已禁用", zap.String("用户名", username))
		return nil, backend.ErrInvalidCredentials
	}

	logger.Log.Info("IMAP 用户登录成功", zap.String("邮箱", user.Email))
	return &User{
		db:      b.db,
		storage: b.storage,
		user:    &user,
	}, nil
}
