package smtp

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"chrelyonly-email-server-go/pkg/imap"
	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/models"
	"chrelyonly-email-server-go/pkg/storage"
	"chrelyonly-email-server-go/pkg/webhook"
	"github.com/emersion/go-smtp"
	"github.com/jhillyerd/enmime"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Session struct {
	db          *gorm.DB
	storage     *storage.StorageManager
	webhook     *webhook.WebhookManager
	relayer     *MailRelayer
	user        *models.User
	from        string
	rcpts       []string
	authFailure int
}

func (s *Session) AuthPlain(username, password string) error {
	var user models.User
	err := s.db.Where("email = ? OR username = ?", username, username).First(&user).Error
	if err != nil || !user.CheckPassword(password) {
		s.authFailure++
		logger.Log.Warn("SMTP 认证失败", zap.String("用户", username))
		return smtp.ErrAuthFailed
	}
	if !user.IsActive {
		return &smtp.SMTPError{
			Code:    554,
			Message: "用户账号已被禁用",
		}
	}

	s.user = &user
	logger.Log.Info("SMTP 用户认证成功", zap.String("邮箱", user.Email))
	return nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	rawBytes, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	env, err := enmime.ReadEnvelope(bytes.NewReader(rawBytes))
	subject := ""
	textBody := ""
	htmlBody := ""
	messageID := ""

	if err == nil && env != nil {
		subject = env.GetHeader("Subject")
		textBody = env.Text
		htmlBody = env.HTML
		messageID = env.GetHeader("Message-ID")
	}
	if messageID == "" {
		messageID = fmt.Sprintf("<%d.%d@local>", time.Now().UnixNano(), len(rawBytes))
	}

	for _, recipient := range s.rcpts {
		cleanRcpt := cleanEmailAddress(recipient)
		cleanFrom := cleanEmailAddress(s.from)

		var targetUser models.User
		err := s.db.Where("email = ?", cleanRcpt).First(&targetUser).Error
		isLocal := (err == nil && targetUser.ID > 0)

		if isLocal {
			if err := s.deliverToUser(&targetUser, "INBOX", cleanFrom, cleanRcpt, subject, textBody, htmlBody, messageID, rawBytes, env); err != nil {
				logger.Log.Error("投递本地邮件失败", zap.Error(err), zap.String("收件人", cleanRcpt))
			}
		} else {
			logger.Log.Info("投递至远程收件人", zap.String("收件人", cleanRcpt))
			go func(toAddr string) {
				if err := s.relayer.RelayMail(cleanFrom, toAddr, rawBytes); err != nil {
					logger.Log.Error("中继转发邮件至远程服务器失败", zap.Error(err), zap.String("收件人", toAddr))
				}
			}(cleanRcpt)
		}

		if s.user != nil && cleanEmailAddress(s.user.Email) == cleanFrom {
			if err := s.deliverToUser(s.user, "Sent", cleanFrom, cleanRcpt, subject, textBody, htmlBody, messageID, rawBytes, env); err != nil {
				logger.Log.Error("保存邮件至已发送文件夹失败", zap.Error(err))
			}
		}
	}

	return nil
}

func (s *Session) deliverToUser(user *models.User, folderName, from, to, subject, textBody, htmlBody, messageID string, rawBytes []byte, env *enmime.Envelope) error {
	var mb models.Mailbox
	if err := s.db.Where("user_id = ? AND name = ?", user.ID, folderName).First(&mb).Error; err != nil {
		mb = models.Mailbox{
			UserID:      user.ID,
			Name:        folderName,
			UIDNext:     1,
			UIDValidity: uint32(time.Now().Unix()),
		}
		if err := s.db.Create(&mb).Error; err != nil {
			return err
		}
	}

	rawPath, err := s.storage.SaveMessage(user.ID, messageID, rawBytes)
	if err != nil {
		return err
	}

	uid := mb.UIDNext
	mb.UIDNext++
	s.db.Save(&mb)

	now := time.Now()
	email := models.Email{
		MessageID:   messageID,
		MailboxID:   mb.ID,
		UserID:      user.ID,
		UID:         uid,
		FromAddress: from,
		ToAddress:   to,
		Subject:     subject,
		TextBody:    textBody,
		HtmlBody:    htmlBody,
		RawPath:     rawPath,
		Size:        int64(len(rawBytes)),
		IsRead:      (folderName == "Sent"),
		SentAt:      now,
		ReceivedAt:  now,
	}

	if err := s.db.Create(&email).Error; err != nil {
		return err
	}

	if env != nil {
		allParts := append(env.Attachments, env.Inlines...)
		for _, part := range allParts {
			attPath, attSize, err := s.storage.SaveAttachment(email.ID, part.FileName, bytes.NewReader(part.Content))
			if err == nil {
				attRecord := models.Attachment{
					EmailID:     email.ID,
					Filename:    part.FileName,
					ContentType: part.ContentType,
					Size:        attSize,
					FilePath:    attPath,
					ContentID:   part.ContentID,
				}
				s.db.Create(&attRecord)
			}
		}
	}

	if folderName == "INBOX" {
		s.webhook.TriggerEmailReceived(email)
		imap.NotifyNewEmail(user.Email, "INBOX", email)
	} else if folderName == "Sent" {
		s.webhook.TriggerEmailSent(email)
	}

	return nil
}

func (s *Session) Reset() {
	s.from = ""
	s.rcpts = nil
}

func (s *Session) Logout() error {
	return nil
}

func cleanEmailAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if idx := strings.Index(addr, "<"); idx >= 0 {
		if end := strings.Index(addr[idx:], ">"); end >= 0 {
			addr = addr[idx+1 : idx+end]
		}
	}
	return strings.ToLower(strings.TrimSpace(addr))
}
