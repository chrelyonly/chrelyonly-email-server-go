package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"chrelyonly-email-server-go/internal/model"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// --- Domain Repositories ---

func (r *Repository) CreateDomain(domain *model.Domain) error {
	query := `INSERT INTO mail_domains (domain, is_active, dkim_selector, dkim_private_key) VALUES (?, ?, ?, ?)`
	res, err := r.db.Exec(query, domain.Domain, domain.IsActive, domain.DkimSelector, domain.DkimPrivateKey)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		domain.ID = id
	}
	return nil
}

func (r *Repository) GetDomainByName(name string) (*model.Domain, error) {
	query := `SELECT id, domain, is_active, dkim_selector, dkim_private_key, created_at, updated_at FROM mail_domains WHERE domain = ?`
	row := r.db.QueryRow(query, strings.ToLower(name))
	var d model.Domain
	var dkimKey sql.NullString
	err := row.Scan(&d.ID, &d.Domain, &d.IsActive, &d.DkimSelector, &dkimKey, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if dkimKey.Valid {
		d.DkimPrivateKey = dkimKey.String
	}
	return &d, nil
}

func (r *Repository) GetDomainByID(id int64) (*model.Domain, error) {
	query := `SELECT id, domain, is_active, dkim_selector, dkim_private_key, created_at, updated_at FROM mail_domains WHERE id = ?`
	row := r.db.QueryRow(query, id)
	var d model.Domain
	var dkimKey sql.NullString
	err := row.Scan(&d.ID, &d.Domain, &d.IsActive, &d.DkimSelector, &dkimKey, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if dkimKey.Valid {
		d.DkimPrivateKey = dkimKey.String
	}
	return &d, nil
}

func (r *Repository) ListDomains() ([]model.Domain, error) {
	query := `SELECT id, domain, is_active, dkim_selector, created_at, updated_at FROM mail_domains ORDER BY id DESC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Domain
	for rows.Next() {
		var d model.Domain
		if err := rows.Scan(&d.ID, &d.Domain, &d.IsActive, &d.DkimSelector, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}

func (r *Repository) DeleteDomain(id int64) error {
	query := `DELETE FROM mail_domains WHERE id = ?`
	_, err := r.db.Exec(query, id)
	return err
}

// --- Account Repositories ---

func (r *Repository) CreateAccount(acc *model.Account) error {
	query := `INSERT INTO mail_accounts (email, password_hash, domain_id, quota_bytes, is_active) VALUES (?, ?, ?, ?, ?)`
	res, err := r.db.Exec(query, strings.ToLower(acc.Email), acc.PasswordHash, acc.DomainID, acc.QuotaBytes, acc.IsActive)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		acc.ID = id
	}
	return nil
}

func (r *Repository) GetAccountByEmail(email string) (*model.Account, error) {
	query := `SELECT id, email, password_hash, domain_id, quota_bytes, used_bytes, is_active, created_at, updated_at FROM mail_accounts WHERE email = ?`
	row := r.db.QueryRow(query, strings.ToLower(email))
	var a model.Account
	err := row.Scan(&a.ID, &a.Email, &a.PasswordHash, &a.DomainID, &a.QuotaBytes, &a.UsedBytes, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) GetAccountByID(id int64) (*model.Account, error) {
	query := `SELECT id, email, password_hash, domain_id, quota_bytes, used_bytes, is_active, created_at, updated_at FROM mail_accounts WHERE id = ?`
	row := r.db.QueryRow(query, id)
	var a model.Account
	err := row.Scan(&a.ID, &a.Email, &a.PasswordHash, &a.DomainID, &a.QuotaBytes, &a.UsedBytes, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) ListAccounts() ([]model.Account, error) {
	query := `SELECT id, email, domain_id, quota_bytes, used_bytes, is_active, created_at, updated_at FROM mail_accounts ORDER BY id DESC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Account
	for rows.Next() {
		var a model.Account
		if err := rows.Scan(&a.ID, &a.Email, &a.DomainID, &a.QuotaBytes, &a.UsedBytes, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

func (r *Repository) DeleteAccount(id int64) error {
	query := `DELETE FROM mail_accounts WHERE id = ?`
	_, err := r.db.Exec(query, id)
	return err
}

// --- Message Repositories ---

func (r *Repository) SaveMessage(msg *model.Message, recipients []model.Recipient, attachments []model.Attachment) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	msgQuery := `INSERT INTO mail_messages (message_id, account_id, from_address, subject, text_content, html_content, raw_file_path, size_bytes, has_attachments, is_read, direction, status, received_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	res, err := tx.Exec(msgQuery,
		msg.MessageID, msg.AccountID, msg.FromAddress, msg.Subject, msg.TextContent, msg.HTMLContent,
		msg.RawFilePath, msg.SizeBytes, msg.HasAttachments, msg.IsRead, msg.Direction, msg.Status, msg.ReceivedAt)
	if err != nil {
		return fmt.Errorf("保存邮件主表失败: %w", err)
	}

	msgDbID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	msg.ID = msgDbID

	// 保存收件人
	recQuery := `INSERT INTO mail_recipients (message_id, recipient_type, email_address) VALUES (?, ?, ?)`
	for _, rec := range recipients {
		_, err := tx.Exec(recQuery, msgDbID, rec.RecipientType, rec.EmailAddress)
		if err != nil {
			return fmt.Errorf("保存邮件收件人关联失败: %w", err)
		}
	}

	// 保存附件
	attQuery := `INSERT INTO mail_attachments (message_id, filename, content_type, size_bytes, file_path, content_id) VALUES (?, ?, ?, ?, ?, ?)`
	for i := range attachments {
		att := &attachments[i]
		attRes, err := tx.Exec(attQuery, msgDbID, att.Filename, att.ContentType, att.SizeBytes, att.FilePath, att.ContentID)
		if err != nil {
			return fmt.Errorf("保存附件列表元数据失败: %w", err)
		}
		attID, _ := attRes.LastInsertId()
		att.ID = attID
	}

	return tx.Commit()
}

func (r *Repository) GetMessageByMessageID(messageID string) (*model.Message, error) {
	query := `SELECT id, message_id, account_id, from_address, subject, text_content, html_content, raw_file_path, size_bytes, has_attachments, is_read, direction, status, received_at, created_at, updated_at FROM mail_messages WHERE message_id = ?`
	row := r.db.QueryRow(query, messageID)

	var msg model.Message
	var accountID sql.NullInt64
	err := row.Scan(&msg.ID, &msg.MessageID, &accountID, &msg.FromAddress, &msg.Subject, &msg.TextContent, &msg.HTMLContent, &msg.RawFilePath, &msg.SizeBytes, &msg.HasAttachments, &msg.IsRead, &msg.Direction, &msg.Status, &msg.ReceivedAt, &msg.CreatedAt, &msg.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if accountID.Valid {
		msg.AccountID = &accountID.Int64
	}

	r.loadMessageDetails(&msg)
	return &msg, nil
}

func (r *Repository) ListMessages(accountEmail string, page int, size int) ([]model.Message, int64, error) {
	return r.ListFolderMessages(accountEmail, "ALL", page, size)
}

func (r *Repository) ListFolderMessages(accountEmail string, folder string, page int, size int) ([]model.Message, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	offset := (page - 1) * size

	var countQuery string
	var listQuery string
	var args []interface{}

	if accountEmail != "" {
		acc, err := r.GetAccountByEmail(accountEmail)
		if err != nil {
			return nil, 0, err
		}
		if acc == nil {
			return []model.Message{}, 0, nil
		}
		cleanEmail := strings.ToLower(strings.TrimSpace(accountEmail))

		if strings.EqualFold(folder, "INBOX") {
			countQuery = `SELECT COUNT(DISTINCT m.id) FROM mail_messages m
				LEFT JOIN mail_recipients r ON m.id = r.message_id
				WHERE r.email_address = ? OR (m.account_id = ? AND m.direction = 'inbound')`
			listQuery = `SELECT DISTINCT m.id, m.message_id, m.account_id, m.from_address, m.subject, m.text_content, m.html_content, m.raw_file_path, m.size_bytes, m.has_attachments, m.is_read, m.direction, m.status, m.received_at, m.created_at, m.updated_at
				FROM mail_messages m
				LEFT JOIN mail_recipients r ON m.id = r.message_id
				WHERE r.email_address = ? OR (m.account_id = ? AND m.direction = 'inbound')
				ORDER BY m.id DESC LIMIT ? OFFSET ?`
			args = append(args, cleanEmail, acc.ID)
		} else if strings.EqualFold(folder, "SENT") {
			countQuery = `SELECT COUNT(DISTINCT m.id) FROM mail_messages m
				LEFT JOIN mail_recipients r ON m.id = r.message_id
				WHERE m.from_address = ? OR (m.account_id = ? AND m.direction = 'outbound')`
			listQuery = `SELECT DISTINCT m.id, m.message_id, m.account_id, m.from_address, m.subject, m.text_content, m.html_content, m.raw_file_path, m.size_bytes, m.has_attachments, m.is_read, m.direction, m.status, m.received_at, m.created_at, m.updated_at
				FROM mail_messages m
				LEFT JOIN mail_recipients r ON m.id = r.message_id
				WHERE m.from_address = ? OR (m.account_id = ? AND m.direction = 'outbound')
				ORDER BY m.id DESC LIMIT ? OFFSET ?`
			args = append(args, cleanEmail, acc.ID)
		} else {
			countQuery = `SELECT COUNT(DISTINCT m.id) FROM mail_messages m
				LEFT JOIN mail_recipients r ON m.id = r.message_id
				WHERE r.email_address = ? OR m.from_address = ? OR m.account_id = ?`
			listQuery = `SELECT DISTINCT m.id, m.message_id, m.account_id, m.from_address, m.subject, m.text_content, m.html_content, m.raw_file_path, m.size_bytes, m.has_attachments, m.is_read, m.direction, m.status, m.received_at, m.created_at, m.updated_at
				FROM mail_messages m
				LEFT JOIN mail_recipients r ON m.id = r.message_id
				WHERE r.email_address = ? OR m.from_address = ? OR m.account_id = ?
				ORDER BY m.id DESC LIMIT ? OFFSET ?`
			args = append(args, cleanEmail, cleanEmail, acc.ID)
		}
	} else {
		countQuery = `SELECT COUNT(*) FROM mail_messages`
		listQuery = `SELECT id, message_id, account_id, from_address, subject, text_content, html_content, raw_file_path, size_bytes, has_attachments, is_read, direction, status, received_at, created_at, updated_at FROM mail_messages ORDER BY id DESC LIMIT ? OFFSET ?`
	}

	var total int64
	if len(args) > 0 {
		_ = r.db.QueryRow(countQuery, args...).Scan(&total)
	} else {
		_ = r.db.QueryRow(countQuery).Scan(&total)
	}

	listArgs := append(args, size, offset)
	rows, err := r.db.Query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var messages []model.Message
	for rows.Next() {
		var msg model.Message
		var accountID sql.NullInt64
		if err := rows.Scan(&msg.ID, &msg.MessageID, &accountID, &msg.FromAddress, &msg.Subject, &msg.TextContent, &msg.HTMLContent, &msg.RawFilePath, &msg.SizeBytes, &msg.HasAttachments, &msg.IsRead, &msg.Direction, &msg.Status, &msg.ReceivedAt, &msg.CreatedAt, &msg.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if accountID.Valid {
			msg.AccountID = &accountID.Int64
		}
		r.loadMessageDetails(&msg)
		messages = append(messages, msg)
	}

	return messages, total, nil
}

func (r *Repository) DeleteMessage(messageID string) (*model.Message, error) {
	msg, err := r.GetMessageByMessageID(messageID)
	if err != nil || msg == nil {
		return nil, err
	}

	query := `DELETE FROM mail_messages WHERE id = ?`
	_, err = r.db.Exec(query, msg.ID)
	return msg, err
}

func (r *Repository) loadMessageDetails(msg *model.Message) {
	// 加载收件人
	recQuery := `SELECT recipient_type, email_address FROM mail_recipients WHERE message_id = ?`
	rows, err := r.db.Query(recQuery, msg.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var rType, email string
			if err := rows.Scan(&rType, &email); err == nil {
				switch rType {
				case "to":
					msg.To = append(msg.To, email)
				case "cc":
					msg.Cc = append(msg.Cc, email)
				case "bcc":
					msg.Bcc = append(msg.Bcc, email)
				}
			}
		}
	}

	// 加载附件列表
	attQuery := `SELECT id, message_id, filename, content_type, size_bytes, file_path, content_id, created_at FROM mail_attachments WHERE message_id = ?`
	attRows, err := r.db.Query(attQuery, msg.ID)
	if err == nil {
		defer attRows.Close()
		for attRows.Next() {
			var att model.Attachment
			var contentID sql.NullString
			if err := attRows.Scan(&att.ID, &att.MessageID, &att.Filename, &att.ContentType, &att.SizeBytes, &att.FilePath, &contentID, &att.CreatedAt); err == nil {
				if contentID.Valid {
					att.ContentID = contentID.String
				}
				msg.Attachments = append(msg.Attachments, att)
			}
		}
	}
}

func (r *Repository) GetAttachmentByID(id int64) (*model.Attachment, error) {
	query := `SELECT id, message_id, filename, content_type, size_bytes, file_path, content_id, created_at FROM mail_attachments WHERE id = ?`
	row := r.db.QueryRow(query, id)
	var att model.Attachment
	var contentID sql.NullString
	err := row.Scan(&att.ID, &att.MessageID, &att.Filename, &att.ContentType, &att.SizeBytes, &att.FilePath, &contentID, &att.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if contentID.Valid {
		att.ContentID = contentID.String
	}
	return &att, nil
}

// --- Queue & Delivery Repositories ---

func (r *Repository) CreateQueueItem(item *model.QueueItem) error {
	query := `INSERT INTO mail_queue (message_id, from_address, to_address, status, retry_count, max_retries, next_retry_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.Exec(query, item.MessageID, item.FromAddress, item.ToAddress, item.Status, item.RetryCount, item.MaxRetries, item.NextRetryAt)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		item.ID = id
	}
	return nil
}

func (r *Repository) GetPendingQueueItems(limit int) ([]model.QueueItem, error) {
	query := `SELECT id, message_id, from_address, to_address, status, retry_count, max_retries, next_retry_at, last_error, created_at, updated_at
		FROM mail_queue WHERE status IN ('pending', 'failed') AND retry_count < max_retries AND next_retry_at <= NOW() ORDER BY id ASC LIMIT ?`
	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.QueueItem
	for rows.Next() {
		var item model.QueueItem
		var lastErr sql.NullString
		if err := rows.Scan(&item.ID, &item.MessageID, &item.FromAddress, &item.ToAddress, &item.Status, &item.RetryCount, &item.MaxRetries, &item.NextRetryAt, &lastErr, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if lastErr.Valid {
			item.LastError = lastErr.String
		}
		list = append(list, item)
	}
	return list, nil
}

func (r *Repository) UpdateQueueStatus(id int64, status string, retryCount int, nextRetryAt string, lastError string) error {
	query := `UPDATE mail_queue SET status = ?, retry_count = ?, next_retry_at = ?, last_error = ? WHERE id = ?`
	_, err := r.db.Exec(query, status, retryCount, nextRetryAt, lastError, id)
	return err
}

func (r *Repository) CreateDeliveryLog(log *model.DeliveryLog) error {
	query := `INSERT INTO mail_delivery_logs (queue_id, message_id, status, mx_host, smtp_code, response) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query, log.QueueID, log.MessageID, log.Status, log.MXHost, log.SMTPCode, log.Response)
	return err
}

// --- Event Webhook Repositories ---

func (r *Repository) CreateEvent(evt *model.Event) error {
	query := `INSERT INTO mail_events (event_id, event_type, message_id, payload, status, retry_count, max_retries, next_retry_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.Exec(query, evt.EventID, evt.EventType, evt.MessageID, evt.Payload, evt.Status, evt.RetryCount, evt.MaxRetries, evt.NextRetryAt)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		evt.ID = id
	}
	return nil
}

func (r *Repository) GetPendingEvents(limit int) ([]model.Event, error) {
	query := `SELECT id, event_id, event_type, message_id, payload, status, retry_count, max_retries, next_retry_at, last_error, created_at, updated_at
		FROM mail_events WHERE status IN ('pending', 'failed') AND retry_count < max_retries AND next_retry_at <= NOW() ORDER BY id ASC LIMIT ?`
	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Event
	for rows.Next() {
		var evt model.Event
		var lastErr sql.NullString
		if err := rows.Scan(&evt.ID, &evt.EventID, &evt.EventType, &evt.MessageID, &evt.Payload, &evt.Status, &evt.RetryCount, &evt.MaxRetries, &evt.NextRetryAt, &lastErr, &evt.CreatedAt, &evt.UpdatedAt); err != nil {
			return nil, err
		}
		if lastErr.Valid {
			evt.LastError = lastErr.String
		}
		list = append(list, evt)
	}
	return list, nil
}

func (r *Repository) UpdateEventStatus(id int64, status string, retryCount int, nextRetryAt string, lastError string) error {
	query := `UPDATE mail_events SET status = ?, retry_count = ?, next_retry_at = ?, last_error = ? WHERE id = ?`
	_, err := r.db.Exec(query, status, retryCount, nextRetryAt, lastError, id)
	return err
}

// --- Auth Log Repository ---

func (r *Repository) CreateAuthLog(log *model.AuthLog) error {
	query := `INSERT INTO mail_auth_logs (username, ip_address, success, reason) VALUES (?, ?, ?, ?)`
	_, err := r.db.Exec(query, log.Username, log.IPAddress, log.Success, log.Reason)
	return err
}
