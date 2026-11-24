package api

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	mimeParser "chrelyonly-email-server-go/internal/mime"
	"chrelyonly-email-server-go/internal/model"
	"chrelyonly-email-server-go/internal/repository"
	"chrelyonly-email-server-go/internal/security"
	"chrelyonly-email-server-go/internal/storage"
	"chrelyonly-email-server-go/internal/webhook"
	"chrelyonly-email-server-go/pkg/response"
	"chrelyonly-email-server-go/pkg/utils"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type APIHandler struct {
	repo       *repository.Repository
	storage    *storage.StorageManager
	dispatcher *webhook.Dispatcher
	db         *sql.DB
	logger     *zap.Logger
}

func NewAPIHandler(
	repo *repository.Repository,
	storage *storage.StorageManager,
	dispatcher *webhook.Dispatcher,
	db *sql.DB,
	logger *zap.Logger,
) *APIHandler {
	return &APIHandler{
		repo:       repo,
		storage:    storage,
		dispatcher: dispatcher,
		db:         db,
		logger:     logger,
	}
}

// SendMail POST /internal/api/v1/mail/send
func (h *APIHandler) SendMail(w http.ResponseWriter, r *http.Request) {
	var req model.SendMailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.JSONError(w, http.StatusBadRequest, "无效的 JSON 请求数据: "+err.Error())
		return
	}

	if req.From == "" || len(req.To) == 0 || req.Subject == "" {
		response.JSONError(w, http.StatusBadRequest, "发件人 (from)、收件人 (to) 和主题 (subject) 不能为空")
		return
	}

	senderDomain := utils.ExtractDomainFromEmail(req.From)
	msgID := utils.GenerateMessageID(senderDomain)

	// 1. 构建 EML 文件
	rawEML, err := mimeParser.BuildEML(req, msgID)
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "构建 MIME 邮件格式失败: "+err.Error())
		return
	}

	// 2. 保存原始 EML 到磁盘
	relPath, err := h.storage.SaveRawMessage(rawEML)
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "保存 EML 文件到磁盘失败: "+err.Error())
		return
	}

	// 3. 保存附件到磁盘并记录元数据
	var dbAttachments []model.Attachment
	for _, att := range req.Attachments {
		data, err := base64.StdEncoding.DecodeString(att.DataBase64)
		if err != nil {
			continue
		}
		attRelPath, attSize, err := h.storage.SaveAttachment(att.Filename, bytes.NewReader(data))
		if err == nil {
			dbAttachments = append(dbAttachments, model.Attachment{
				Filename:    att.Filename,
				ContentType: att.ContentType,
				SizeBytes:   attSize,
				FilePath:    attRelPath,
			})
		}
	}

	// 查找本地账户
	acc, _ := h.repo.GetAccountByEmail(req.From)
	var accountID *int64
	if acc != nil {
		accountID = &acc.ID
	}

	// 4. 保存 Message 主数据到数据库
	msg := &model.Message{
		MessageID:      msgID,
		AccountID:      accountID,
		FromAddress:    req.From,
		Subject:        req.Subject,
		TextContent:    req.Text,
		HTMLContent:    req.HTML,
		RawFilePath:    relPath,
		SizeBytes:      int64(len(rawEML)),
		HasAttachments: len(dbAttachments) > 0,
		IsRead:         true,
		Direction:      "outbound",
		Status:         "queued",
		ReceivedAt:     time.Now(),
	}

	var dbRecipients []model.Recipient
	for _, to := range req.To {
		dbRecipients = append(dbRecipients, model.Recipient{RecipientType: "to", EmailAddress: to})
	}
	for _, cc := range req.Cc {
		dbRecipients = append(dbRecipients, model.Recipient{RecipientType: "cc", EmailAddress: cc})
	}
	for _, bcc := range req.Bcc {
		dbRecipients = append(dbRecipients, model.Recipient{RecipientType: "bcc", EmailAddress: bcc})
	}

	if err := h.repo.SaveMessage(msg, dbRecipients, dbAttachments); err != nil {
		response.JSONError(w, http.StatusInternalServerError, "保存邮件元数据到数据库失败: "+err.Error())
		return
	}

	// 5. 将邮件加入发送投递队列
	allRecipients := append(req.To, req.Cc...)
	allRecipients = append(allRecipients, req.Bcc...)

	for _, rcpt := range allRecipients {
		queueItem := &model.QueueItem{
			MessageID:   msgID,
			FromAddress: req.From,
			ToAddress:   rcpt,
			Status:      "pending",
			RetryCount:  0,
			MaxRetries:  10,
			NextRetryAt: time.Now(),
		}
		_ = h.repo.CreateQueueItem(queueItem)
	}

	// 6. 派发 mail.queued 事件
	_ = h.dispatcher.DispatchEvent("mail.queued", msgID, map[string]interface{}{
		"messageId":  msgID,
		"from":       req.From,
		"to":         req.To,
		"subject":    req.Subject,
		"recipients": len(allRecipients),
		"queuedAt":   time.Now().Format(time.RFC3339),
	})

	response.JSONResponse(w, http.StatusOK, model.SendMailResponse{
		Success:   true,
		MessageID: msgID,
		Status:    "queued",
	}, "邮件已成功进入发送队列")
}

// GetMailDetail GET /internal/api/v1/mail/{messageId}
func (h *APIHandler) GetMailDetail(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageId")
	if messageID == "" {
		response.JSONError(w, http.StatusBadRequest, "参数 messageId 不能为空")
		return
	}

	msg, err := h.repo.GetMessageByMessageID(messageID)
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "查询邮件失败: "+err.Error())
		return
	}
	if msg == nil {
		response.JSONError(w, http.StatusNotFound, "找不到对应邮件")
		return
	}

	response.JSONResponse(w, http.StatusOK, msg, "获取邮件详情成功")
}

// ListMails GET /internal/api/v1/mail?account=admin@chrelyonly.cn&page=1&size=20
func (h *APIHandler) ListMails(w http.ResponseWriter, r *http.Request) {
	accountEmail := r.URL.Query().Get("account")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))

	messages, total, err := h.repo.ListMessages(accountEmail, page, size)
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "获取邮件列表失败: "+err.Error())
		return
	}

	response.JSONResponse(w, http.StatusOK, map[string]interface{}{
		"items": messages,
		"total": total,
		"page":  page,
		"size":  size,
	}, "获取邮件列表成功")
}

// DeleteMail DELETE /internal/api/v1/mail/{messageId}
func (h *APIHandler) DeleteMail(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageId")
	if messageID == "" {
		response.JSONError(w, http.StatusBadRequest, "参数 messageId 不能为空")
		return
	}

	msg, err := h.repo.DeleteMessage(messageID)
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "删除邮件记录失败: "+err.Error())
		return
	}
	if msg == nil {
		response.JSONError(w, http.StatusNotFound, "邮件不存在")
		return
	}

	// 删除磁盘文件
	if msg.RawFilePath != "" {
		_ = h.storage.DeleteMessageFile(msg.RawFilePath)
	}
	for _, att := range msg.Attachments {
		if att.FilePath != "" {
			_ = h.storage.DeleteAttachmentFile(att.FilePath)
		}
	}

	// 派发 Webhook mail.deleted
	_ = h.dispatcher.DispatchEvent("mail.deleted", messageID, map[string]interface{}{
		"messageId": messageID,
		"deletedAt": time.Now().Format(time.RFC3339),
	})

	response.JSONResponse(w, http.StatusOK, nil, "邮件已成功删除")
}

// DownloadAttachment GET /internal/api/v1/mail/attachments/{id}
func (h *APIHandler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	attID, _ := strconv.ParseInt(idStr, 10, 64)

	att, err := h.repo.GetAttachmentByID(attID)
	if err != nil || att == nil {
		response.JSONError(w, http.StatusNotFound, "附件不存在")
		return
	}

	reader, err := h.storage.ReadAttachmentReader(att.FilePath)
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "读取附件文件失败: "+err.Error())
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.Filename))
	w.Header().Set("Content-Length", strconv.FormatInt(att.SizeBytes, 10))

	_, _ = io.Copy(w, reader)
}

// CreateAccount POST /internal/api/v1/accounts
func (h *APIHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.JSONError(w, http.StatusBadRequest, "解析请求体失败: "+err.Error())
		return
	}

	if req.Email == "" || req.Password == "" || req.DomainID <= 0 {
		response.JSONError(w, http.StatusBadRequest, "email、password 和 domainId 为必填项")
		return
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "计算密码哈希失败: "+err.Error())
		return
	}

	if req.QuotaBytes == 0 {
		req.QuotaBytes = 10737418240 // 默认 10GB
	}

	acc := &model.Account{
		Email:        strings.ToLower(req.Email),
		PasswordHash: hash,
		DomainID:     req.DomainID,
		QuotaBytes:   req.QuotaBytes,
		IsActive:     true,
	}

	if err := h.repo.CreateAccount(acc); err != nil {
		response.JSONError(w, http.StatusInternalServerError, "创建邮箱账户失败: "+err.Error())
		return
	}

	response.JSONResponse(w, http.StatusOK, acc, "邮箱账户创建成功")
}

// ListAccounts GET /internal/api/v1/accounts
func (h *APIHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.repo.ListAccounts()
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "查询账户列表失败: "+err.Error())
		return
	}
	response.JSONResponse(w, http.StatusOK, accounts, "获取账户列表成功")
}

// DeleteAccount DELETE /internal/api/v1/accounts/{id}
func (h *APIHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	if err := h.repo.DeleteAccount(id); err != nil {
		response.JSONError(w, http.StatusInternalServerError, "删除账户失败: "+err.Error())
		return
	}
	response.JSONResponse(w, http.StatusOK, nil, "账户已成功删除")
}

// CreateDomain POST /internal/api/v1/domains
func (h *APIHandler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var req model.CreateDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.JSONError(w, http.StatusBadRequest, "解析请求失败: "+err.Error())
		return
	}

	if req.Domain == "" {
		response.JSONError(w, http.StatusBadRequest, "domain 不能为空")
		return
	}

	selector := req.DkimSelector
	if selector == "" {
		selector = "default"
	}

	d := &model.Domain{
		Domain:         strings.ToLower(req.Domain),
		IsActive:       true,
		DkimSelector:   selector,
		DkimPrivateKey: req.DkimPrivateKey,
	}

	if err := h.repo.CreateDomain(d); err != nil {
		response.JSONError(w, http.StatusInternalServerError, "添加域名失败: "+err.Error())
		return
	}

	response.JSONResponse(w, http.StatusOK, d, "添加域名成功")
}

// ListDomains GET /internal/api/v1/domains
func (h *APIHandler) ListDomains(w http.ResponseWriter, r *http.Request) {
	domains, err := h.repo.ListDomains()
	if err != nil {
		response.JSONError(w, http.StatusInternalServerError, "获取域名列表失败: "+err.Error())
		return
	}
	response.JSONResponse(w, http.StatusOK, domains, "获取域名列表成功")
}

// DeleteDomain DELETE /internal/api/v1/domains/{id}
func (h *APIHandler) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	if err := h.repo.DeleteDomain(id); err != nil {
		response.JSONError(w, http.StatusInternalServerError, "删除域名失败: "+err.Error())
		return
	}
	response.JSONResponse(w, http.StatusOK, nil, "域名已成功删除")
}

// HealthCheck GET /internal/api/v1/health
func (h *APIHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	dbHealthy := true
	if err := h.db.Ping(); err != nil {
		dbHealthy = false
	}

	storageHealthy := h.storage.IsHealthy()

	resp := model.HealthResponse{
		Success:  dbHealthy && storageHealthy,
		Status:   "healthy",
		SMTP:     true,
		Database: dbHealthy,
		Storage:  storageHealthy,
	}

	if !resp.Success {
		resp.Status = "unhealthy"
	}

	response.JSONResponse(w, http.StatusOK, resp, "健康检查成功")
}
