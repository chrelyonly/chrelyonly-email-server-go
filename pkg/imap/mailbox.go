package imap

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/models"
	"chrelyonly-email-server-go/pkg/storage"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/jhillyerd/enmime"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Mailbox struct {
	db      *gorm.DB
	storage *storage.StorageManager
	user    *models.User
	mailbox *models.Mailbox
}

func (m *Mailbox) Name() string {
	return m.mailbox.Name
}

func (m *Mailbox) Info() (*imap.MailboxInfo, error) {
	return &imap.MailboxInfo{
		Attributes: []string{imap.UnmarkedAttr},
		Delimiter:  "/",
		Name:       m.mailbox.Name,
	}, nil
}

func (m *Mailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	// Auto-fix any emails with UID == 0 so accumulated emails are never hidden
	var zeroUidEmails []models.Email
	m.db.Where("mailbox_id = ? AND (uid IS NULL OR uid = 0)", m.mailbox.ID).Find(&zeroUidEmails)
	for _, ze := range zeroUidEmails {
		ze.UID = m.mailbox.UIDNext
		m.mailbox.UIDNext++
		m.db.Save(&ze)
		m.db.Save(m.mailbox)
	}

	status := imap.NewMailboxStatus(m.mailbox.Name, items)
	status.Flags = []string{imap.SeenFlag, imap.AnsweredFlag, imap.FlaggedFlag, imap.DeletedFlag, imap.DraftFlag}
	status.PermanentFlags = []string{imap.SeenFlag, imap.AnsweredFlag, imap.FlaggedFlag, imap.DeletedFlag, imap.DraftFlag, "\\*"}
	status.UidNext = m.mailbox.UIDNext
	status.UidValidity = m.mailbox.UIDValidity

	var total int64
	m.db.Model(&models.Email{}).Where("mailbox_id = ? AND is_deleted = ?", m.mailbox.ID, false).Count(&total)
	status.Messages = uint32(total)

	var unseen int64
	m.db.Model(&models.Email{}).Where("mailbox_id = ? AND is_read = ? AND is_deleted = ?", m.mailbox.ID, false, false).Count(&unseen)
	status.Unseen = uint32(unseen)

	return status, nil
}

func (m *Mailbox) SetSubscribed(subscribed bool) error {
	return nil
}

func (m *Mailbox) Check() error {
	return nil
}

func (m *Mailbox) ListMessages(uid bool, seqSet *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	defer close(ch)

	// Auto-fix any emails with UID == 0 so accumulated emails are never hidden
	var zeroUidEmails []models.Email
	m.db.Where("mailbox_id = ? AND (uid IS NULL OR uid = 0)", m.mailbox.ID).Find(&zeroUidEmails)
	for _, ze := range zeroUidEmails {
		ze.UID = m.mailbox.UIDNext
		m.mailbox.UIDNext++
		m.db.Save(&ze)
		m.db.Save(m.mailbox)
	}

	var emails []models.Email
	query := m.db.Where("mailbox_id = ?", m.mailbox.ID).Order("uid ASC")

	if err := query.Find(&emails).Error; err != nil {
		return err
	}

	for i, email := range emails {
		seqNum := uint32(i + 1)
		targetID := seqNum
		if uid {
			targetID = email.UID
		}

		if !seqSet.Contains(targetID) {
			continue
		}

		msg := imap.NewMessage(seqNum, items)
		msg.Uid = email.UID
		msg.Size = uint32(email.Size)
		msg.InternalDate = email.ReceivedAt

		// Flags
		var flags []string
		if email.IsRead {
			flags = append(flags, imap.SeenFlag)
		}
		if email.IsFlagged {
			flags = append(flags, imap.FlaggedFlag)
		}
		if email.IsDeleted {
			flags = append(flags, imap.DeletedFlag)
		}
		if email.IsDraft {
			flags = append(flags, imap.DraftFlag)
		}
		msg.Flags = flags

		// Envelope
		msg.Envelope = &imap.Envelope{
			Date:      email.SentAt,
			Subject:   email.Subject,
			From:      parseAddressList(email.FromAddress),
			To:        parseAddressList(email.ToAddress),
			Cc:        parseAddressList(email.CcAddress),
			Bcc:       parseAddressList(email.BccAddress),
			MessageId: email.MessageID,
		}

		// BodyStructure (prevent nil pointer panic in go-imap)
		if email.HtmlBody != "" {
			msg.BodyStructure = &imap.BodyStructure{
				MIMEType:    "text",
				MIMESubType: "html",
				Params:      map[string]string{"charset": "utf-8"},
				Encoding:    "QUOTED-PRINTABLE",
				Size:        uint32(len(email.HtmlBody)),
			}
		} else {
			msg.BodyStructure = &imap.BodyStructure{
				MIMEType:    "text",
				MIMESubType: "plain",
				Params:      map[string]string{"charset": "utf-8"},
				Encoding:    "QUOTED-PRINTABLE",
				Size:        uint32(len(email.TextBody)),
			}
		}

		// Read raw bytes
		rawBytes, err := m.storage.ReadMessage(email.RawPath)
		if err != nil || len(rawBytes) == 0 {
			rawBytes = []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
				email.FromAddress, email.ToAddress, email.Subject, email.TextBody))
		}

		// Populate requested Body sections cleanly
		for _, item := range items {
			sectionName, err := imap.ParseBodySectionName(item)
			if err == nil && sectionName != nil {
				l, err := extractBodySection(rawBytes, sectionName)
				if err == nil {
					msg.Body[sectionName] = l
				}
			}
		}

		ch <- msg
	}

	return nil
}

func (m *Mailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	// Auto-fix any emails with UID == 0 so accumulated emails are never hidden
	var zeroUidEmails []models.Email
	m.db.Where("mailbox_id = ? AND (uid IS NULL OR uid = 0)", m.mailbox.ID).Find(&zeroUidEmails)
	for _, ze := range zeroUidEmails {
		ze.UID = m.mailbox.UIDNext
		m.mailbox.UIDNext++
		m.db.Save(&ze)
		m.db.Save(m.mailbox)
	}

	var emails []models.Email
	if err := m.db.Where("mailbox_id = ?", m.mailbox.ID).Order("uid ASC").Find(&emails).Error; err != nil {
		return nil, err
	}

	var results []uint32
	for i, email := range emails {
		seqNum := uint32(i + 1)
		targetID := seqNum
		if uid {
			targetID = email.UID
		}

		if len(criteria.WithFlags) > 0 {
			matched := true
			for _, flag := range criteria.WithFlags {
				if flag == imap.SeenFlag && !email.IsRead {
					matched = false
				} else if flag == imap.FlaggedFlag && !email.IsFlagged {
					matched = false
				} else if flag == imap.DeletedFlag && !email.IsDeleted {
					matched = false
				}
			}
			if !matched {
				continue
			}
		}

		results = append(results, targetID)
	}

	return results, nil
}

func (m *Mailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	rawBytes, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	env, _ := enmime.ReadEnvelope(bytes.NewReader(rawBytes))
	subject := ""
	fromStr := ""
	toStr := ""
	textBody := ""
	htmlBody := ""
	msgID := ""

	if env != nil {
		subject = env.GetHeader("Subject")
		fromStr = env.GetHeader("From")
		toStr = env.GetHeader("To")
		textBody = env.Text
		htmlBody = env.HTML
		msgID = env.GetHeader("Message-ID")
	}

	if msgID == "" {
		msgID = fmt.Sprintf("<append.%d@local>", time.Now().UnixNano())
	}
	if fromStr == "" {
		fromStr = m.user.Email
	}

	rawPath, err := m.storage.SaveMessage(m.user.ID, msgID, rawBytes)
	if err != nil {
		return err
	}

	uid := m.mailbox.UIDNext
	m.mailbox.UIDNext++
	m.db.Save(m.mailbox)

	isRead := containsFlag(flags, imap.SeenFlag)
	isFlagged := containsFlag(flags, imap.FlaggedFlag)
	isDeleted := containsFlag(flags, imap.DeletedFlag)
	isDraft := containsFlag(flags, imap.DraftFlag)

	if date.IsZero() {
		date = time.Now()
	}

	email := models.Email{
		MessageID:   msgID,
		MailboxID:   m.mailbox.ID,
		UserID:      m.user.ID,
		UID:         uid,
		FromAddress: fromStr,
		ToAddress:   toStr,
		Subject:     subject,
		TextBody:    textBody,
		HtmlBody:    htmlBody,
		RawPath:     rawPath,
		Size:        int64(len(rawBytes)),
		IsRead:      isRead,
		IsFlagged:   isFlagged,
		IsDeleted:   isDeleted,
		IsDraft:     isDraft,
		SentAt:      date,
		ReceivedAt:  date,
	}

	if err := m.db.Create(&email).Error; err != nil {
		return err
	}

	logger.Log.Info("通过 IMAP 追加邮件成功", zap.String("邮箱", m.mailbox.Name), zap.Uint32("UID", uid))
	return nil
}

func (m *Mailbox) UpdateMessagesFlags(uid bool, seqSet *imap.SeqSet, operation imap.FlagsOp, flags []string) error {
	var emails []models.Email
	m.db.Where("mailbox_id = ?", m.mailbox.ID).Order("uid ASC").Find(&emails)

	for i, email := range emails {
		seqNum := uint32(i + 1)
		targetID := seqNum
		if uid {
			targetID = email.UID
		}

		if !seqSet.Contains(targetID) {
			continue
		}

		updates := map[string]interface{}{}
		for _, f := range flags {
			var val bool
			if operation == imap.AddFlags || operation == imap.SetFlags {
				val = true
			} else if operation == imap.RemoveFlags {
				val = false
			}

			switch f {
			case imap.SeenFlag:
				updates["is_read"] = val
			case imap.FlaggedFlag:
				updates["is_flagged"] = val
			case imap.DeletedFlag:
				updates["is_deleted"] = val
			case imap.DraftFlag:
				updates["is_draft"] = val
			}
		}

		if len(updates) > 0 {
			m.db.Model(&models.Email{}).Where("id = ?", email.ID).Updates(updates)
		}
	}
	return nil
}

func (m *Mailbox) CopyMessages(uid bool, seqSet *imap.SeqSet, destName string) error {
	var destMb models.Mailbox
	if err := m.db.Where("user_id = ? AND name = ?", m.user.ID, destName).First(&destMb).Error; err != nil {
		return backend.ErrNoSuchMailbox
	}

	var emails []models.Email
	m.db.Where("mailbox_id = ?", m.mailbox.ID).Order("uid ASC").Find(&emails)

	for i, email := range emails {
		seqNum := uint32(i + 1)
		targetID := seqNum
		if uid {
			targetID = email.UID
		}

		if !seqSet.Contains(targetID) {
			continue
		}

		newUID := destMb.UIDNext
		destMb.UIDNext++
		m.db.Save(&destMb)

		newEmail := email
		newEmail.ID = 0
		newEmail.MailboxID = destMb.ID
		newEmail.UID = newUID
		m.db.Create(&newEmail)
	}

	return nil
}

func (m *Mailbox) Expunge() error {
	m.db.Where("mailbox_id = ? AND is_deleted = ?", m.mailbox.ID, true).Delete(&models.Email{})
	return nil
}

func parseAddressList(addrStr string) []*imap.Address {
	if addrStr == "" {
		return nil
	}
	parts := strings.Split(addrStr, ",")
	var addrs []*imap.Address
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		mailboxName := p
		hostName := ""
		if idx := strings.Index(p, "@"); idx >= 0 {
			mailboxName = p[:idx]
			hostName = p[idx+1:]
		}
		addrs = append(addrs, &imap.Address{
			PersonalName: p,
			MailboxName:  mailboxName,
			HostName:     hostName,
		})
	}
	return addrs
}

func extractBodySection(rawBytes []byte, section *imap.BodySectionName) (imap.Literal, error) {
	if section == nil {
		return bytes.NewReader(rawBytes), nil
	}
	if section.Specifier == imap.HeaderSpecifier {
		idx := bytes.Index(rawBytes, []byte("\r\n\r\n"))
		if idx >= 0 {
			return bytes.NewReader(rawBytes[:idx+4]), nil
		}
		return bytes.NewReader(rawBytes), nil
	}

	env, err := enmime.ReadEnvelope(bytes.NewReader(rawBytes))
	if err == nil && env != nil {
		content := env.Text
		if content == "" {
			content = env.HTML
		}
		if content != "" {
			return bytes.NewReader([]byte(content)), nil
		}
	}

	idx := bytes.Index(rawBytes, []byte("\r\n\r\n"))
	if idx >= 0 && section.Specifier == imap.TextSpecifier {
		return bytes.NewReader(rawBytes[idx+4:]), nil
	}

	return bytes.NewReader(rawBytes), nil
}

func containsFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}
