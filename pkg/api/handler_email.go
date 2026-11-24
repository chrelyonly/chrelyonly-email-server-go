package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chrelyonly-email-server-go/pkg/imap"
	"chrelyonly-email-server-go/pkg/models"
	"chrelyonly-email-server-go/pkg/smtp"
	"chrelyonly-email-server-go/pkg/storage"
	"chrelyonly-email-server-go/pkg/webhook"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EmailHandler struct {
	db      *gorm.DB
	storage *storage.StorageManager
	webhook *webhook.WebhookManager
	relayer *smtp.MailRelayer
}

func NewEmailHandler(db *gorm.DB, storage *storage.StorageManager, webhook *webhook.WebhookManager) *EmailHandler {
	return &EmailHandler{
		db:      db,
		storage: storage,
		webhook: webhook,
		relayer: smtp.NewMailRelayer(),
	}
}

type SendEmailRequest struct {
	FromAddress string `json:"from_address" binding:"required"`
	ToAddress   string `json:"to_address" binding:"required"`
	CcAddress   string `json:"cc_address"`
	BccAddress  string `json:"bcc_address"`
	Subject     string `json:"subject"`
	TextBody    string `json:"text_body"`
	HtmlBody    string `json:"html_body"`
}

func (h *EmailHandler) SendEmail(c *gin.Context) {
	var req SendEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cleanFrom := strings.ToLower(strings.TrimSpace(req.FromAddress))
	cleanTo := strings.ToLower(strings.TrimSpace(req.ToAddress))

	var senderUser models.User
	if err := h.db.Where("email = ?", cleanFrom).First(&senderUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sender user email not found in local server"})
		return
	}

	// Build MIME payload
	messageID := fmt.Sprintf("<api.%d@local>", time.Now().UnixNano())
	now := time.Now()

	var rawBuf strings.Builder
	rawBuf.WriteString(fmt.Sprintf("From: %s\r\n", cleanFrom))
	rawBuf.WriteString(fmt.Sprintf("To: %s\r\n", cleanTo))
	if req.CcAddress != "" {
		rawBuf.WriteString(fmt.Sprintf("Cc: %s\r\n", req.CcAddress))
	}
	rawBuf.WriteString(fmt.Sprintf("Subject: %s\r\n", req.Subject))
	rawBuf.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
	rawBuf.WriteString(fmt.Sprintf("Date: %s\r\n", now.Format(time.RFC1123Z)))
	rawBuf.WriteString("MIME-Version: 1.0\r\n")

	if req.HtmlBody != "" {
		rawBuf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		rawBuf.WriteString(req.HtmlBody)
	} else {
		rawBuf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		rawBuf.WriteString(req.TextBody)
	}

	rawBytes := []byte(rawBuf.String())

	// 1. Save to Sender's "Sent" Mailbox
	var sentMb models.Mailbox
	if err := h.db.Where("user_id = ? AND name = ?", senderUser.ID, "Sent").First(&sentMb).Error; err != nil {
		sentMb = models.Mailbox{UserID: senderUser.ID, Name: "Sent", UIDNext: 1, UIDValidity: uint32(now.Unix())}
		h.db.Create(&sentMb)
	}

	rawPath, err := h.storage.SaveMessage(senderUser.ID, messageID, rawBytes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save email file"})
		return
	}

	uid := sentMb.UIDNext
	sentMb.UIDNext++
	h.db.Save(&sentMb)

	emailRecord := models.Email{
		MessageID:   messageID,
		MailboxID:   sentMb.ID,
		UserID:      senderUser.ID,
		UID:         uid,
		FromAddress: cleanFrom,
		ToAddress:   cleanTo,
		CcAddress:   req.CcAddress,
		BccAddress:  req.BccAddress,
		Subject:     req.Subject,
		TextBody:    req.TextBody,
		HtmlBody:    req.HtmlBody,
		RawPath:     rawPath,
		Size:        int64(len(rawBytes)),
		IsRead:      true,
		SentAt:      now,
		ReceivedAt:  now,
	}

	if err := h.db.Create(&emailRecord).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create email DB record"})
		return
	}

	// 2. Deliver to local recipient or relay to remote MX
	var recipientUser models.User
	if err := h.db.Where("email = ?", cleanTo).First(&recipientUser).Error; err == nil && recipientUser.ID > 0 {
		var inMb models.Mailbox
		if err := h.db.Where("user_id = ? AND name = ?", recipientUser.ID, "INBOX").First(&inMb).Error; err != nil {
			inMb = models.Mailbox{UserID: recipientUser.ID, Name: "INBOX", UIDNext: 1, UIDValidity: uint32(now.Unix())}
			h.db.Create(&inMb)
		}

		inUid := inMb.UIDNext
		inMb.UIDNext++
		h.db.Save(&inMb)

		recRawPath, _ := h.storage.SaveMessage(recipientUser.ID, messageID, rawBytes)

		recEmail := models.Email{
			MessageID:   messageID,
			MailboxID:   inMb.ID,
			UserID:      recipientUser.ID,
			UID:         inUid,
			FromAddress: cleanFrom,
			ToAddress:   cleanTo,
			Subject:     req.Subject,
			TextBody:    req.TextBody,
			HtmlBody:    req.HtmlBody,
			RawPath:     recRawPath,
			Size:        int64(len(rawBytes)),
			IsRead:      false,
			SentAt:      now,
			ReceivedAt:  now,
		}
		h.db.Create(&recEmail)
		h.webhook.TriggerEmailReceived(recEmail)
		imap.NotifyNewEmail(recipientUser.Email, "INBOX", recEmail)
	} else {
		// Relay outbound
		go h.relayer.RelayMail(cleanFrom, cleanTo, rawBytes)
	}

	h.webhook.TriggerEmailSent(emailRecord)
	c.JSON(http.StatusOK, gin.H{"message": "Email sent successfully", "email": emailRecord})
}

func (h *EmailHandler) ListEmails(c *gin.Context) {
	userIDStr := c.Query("user_id")
	mailboxName := c.DefaultQuery("mailbox", "INBOX")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	query := h.db.Model(&models.Email{})
	if userIDStr != "" {
		userID, _ := strconv.ParseUint(userIDStr, 10, 64)
		var mb models.Mailbox
		if err := h.db.Where("user_id = ? AND name = ?", userID, mailboxName).First(&mb).Error; err != nil {
			query = query.Where("mailbox_id = ?", mb.ID)
		} else {
			query = query.Where("user_id = ?", userID)
		}
	}

	var total int64
	query.Count(&total)

	var emails []models.Email
	offset := (page - 1) * limit
	query.Preload("Attachments").Order("id DESC").Offset(offset).Limit(limit).Find(&emails)

	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"page":   page,
		"limit":  limit,
		"emails": emails,
	})
}

func (h *EmailHandler) GetEmail(c *gin.Context) {
	id := c.Param("id")
	var email models.Email
	if err := h.db.Preload("Attachments").First(&email, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Email not found"})
		return
	}
	c.JSON(http.StatusOK, email)
}

func (h *EmailHandler) DownloadRawEmail(c *gin.Context) {
	id := c.Param("id")
	var email models.Email
	if err := h.db.First(&email, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Email record not found"})
		return
	}

	rawBytes, err := h.storage.ReadMessage(email.RawPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Raw email file not found on disk"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=email_%d.eml", email.ID))
	c.Data(http.StatusOK, "message/rfc822", rawBytes)
}

func (h *EmailHandler) DownloadAttachment(c *gin.Context) {
	attID := c.Param("id")
	var att models.Attachment
	if err := h.db.First(&att, attID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}

	c.FileAttachment(att.FilePath, att.Filename)
}

func (h *EmailHandler) DeleteEmail(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&models.Email{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete email"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Email deleted successfully"})
}

func (h *EmailHandler) TestWebhook(c *gin.Context) {
	testEmail := models.Email{
		ID:          99999,
		MessageID:   "<test.webhook@local>",
		FromAddress: "test@local.domain",
		ToAddress:   "recipient@local.domain",
		Subject:     "Webhook Test Email",
		TextBody:    "This is a test webhook payload triggered manually.",
		SentAt:      time.Now(),
		ReceivedAt:  time.Now(),
	}
	h.webhook.TriggerEmailReceived(testEmail)
	c.JSON(http.StatusOK, gin.H{"message": "Test webhook triggered successfully"})
}

func (h *EmailHandler) ListWebhookLogs(c *gin.Context) {
	var logs []models.WebhookLog
	h.db.Order("id DESC").Limit(50).Find(&logs)
	c.JSON(http.StatusOK, logs)
}
