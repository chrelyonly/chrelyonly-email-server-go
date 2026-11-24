package model

import (
	"time"
)

// Domain 托管域名
type Domain struct {
	ID             int64     `json:"id"`
	Domain         string    `json:"domain"`
	IsActive       bool      `json:"isActive"`
	DkimSelector   string    `json:"dkimSelector"`
	DkimPrivateKey string    `json:"-"` // 不直接在 JSON 中输出私钥
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Account 邮箱账户
type Account struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	DomainID     int64     `json:"domainId"`
	QuotaBytes   int64     `json:"quotaBytes"`
	UsedBytes    int64     `json:"usedBytes"`
	IsActive     bool      `json:"isActive"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Message 邮件主数据结构
type Message struct {
	ID             int64        `json:"id"`
	MessageID      string       `json:"messageId"`
	AccountID      *int64       `json:"accountId,omitempty"`
	FromAddress    string       `json:"from"`
	Subject        string       `json:"subject"`
	TextContent    string       `json:"text,omitempty"`
	HTMLContent    string       `json:"html,omitempty"`
	RawFilePath    string       `json:"-"`
	SizeBytes      int64        `json:"sizeBytes"`
	HasAttachments bool         `json:"hasAttachments"`
	IsRead         bool         `json:"isRead"`
	Direction      string       `json:"direction"` // inbound / outbound
	Status         string       `json:"status"`    // received / queued / sent / failed / bounced
	ReceivedAt     time.Time    `json:"receivedAt"`
	CreatedAt      time.Time    `json:"createdAt"`
	UpdatedAt      time.Time    `json:"updatedAt"`
	To             []string     `json:"to"`
	Cc             []string     `json:"cc"`
	Bcc            []string     `json:"bcc"`
	Attachments    []Attachment `json:"attachments,omitempty"`
}

// Recipient 收件人
type Recipient struct {
	ID            int64     `json:"id"`
	MessageID     int64     `json:"messageId"`
	RecipientType string    `json:"type"` // to / cc / bcc
	EmailAddress  string    `json:"email"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Attachment 邮件附件
type Attachment struct {
	ID          int64     `json:"id"`
	MessageID   int64     `json:"messageId"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	FilePath    string    `json:"-"`
	ContentID   string    `json:"contentId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// QueueItem 发送队列记录
type QueueItem struct {
	ID          int64     `json:"id"`
	MessageID   string    `json:"messageId"`
	FromAddress string    `json:"from"`
	ToAddress   string    `json:"to"`
	Status      string    `json:"status"` // pending / processing / sent / failed
	RetryCount  int       `json:"retryCount"`
	MaxRetries  int       `json:"maxRetries"`
	NextRetryAt time.Time `json:"nextRetryAt"`
	LastError   string    `json:"lastError,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// DeliveryLog 投递日志
type DeliveryLog struct {
	ID        int64     `json:"id"`
	QueueID   *int64    `json:"queueId,omitempty"`
	MessageID string    `json:"messageId"`
	Status    string    `json:"status"`
	MXHost    string    `json:"mxHost"`
	SMTPCode  int       `json:"smtpCode"`
	Response  string    `json:"response"`
	CreatedAt time.Time `json:"createdAt"`
}

// Event Webhook 事件队列
type Event struct {
	ID          int64     `json:"id"`
	EventID     string    `json:"eventId"`
	EventType   string    `json:"event"`
	MessageID   string    `json:"messageId"`
	Payload     string    `json:"payload"`
	Status      string    `json:"status"` // pending / processing / sent / failed
	RetryCount  int       `json:"retryCount"`
	MaxRetries  int       `json:"maxRetries"`
	NextRetryAt time.Time `json:"nextRetryAt"`
	LastError   string    `json:"lastError,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// AuthLog 认证日志
type AuthLog struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	IPAddress string    `json:"ipAddress"`
	Success   bool      `json:"success"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

// --- API DTOs ---

// SendAttachmentInput 发送邮件时的内联附件结构
type SendAttachmentInput struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	DataBase64  string `json:"dataBase64"` // Base64 编码的文件数据
}

// SendMailRequest HTTP API 投递邮件请求
type SendMailRequest struct {
	From        string                `json:"from"`
	To          []string              `json:"to"`
	Cc          []string              `json:"cc,omitempty"`
	Bcc         []string              `json:"bcc,omitempty"`
	Subject     string                `json:"subject"`
	Text        string                `json:"text,omitempty"`
	HTML        string                `json:"html,omitempty"`
	Attachments []SendAttachmentInput `json:"attachments,omitempty"`
}

// SendMailResponse 投递邮件响应
type SendMailResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
}

// CreateAccountRequest 创建邮箱账户请求
type CreateAccountRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	DomainID   int64  `json:"domainId"`
	QuotaBytes int64  `json:"quotaBytes"`
}

// CreateDomainRequest 创建域名请求
type CreateDomainRequest struct {
	Domain         string `json:"domain"`
	DkimSelector   string `json:"dkimSelector,omitempty"`
	DkimPrivateKey string `json:"dkimPrivateKey,omitempty"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Success  bool   `json:"success"`
	Status   string `json:"status"`
	SMTP     bool   `json:"smtp"`
	Database bool   `json:"database"`
	Storage  bool   `json:"storage"`
}

// WebhookPayload 推送给 Spring Boot 的 Webhook 数据格式
type WebhookPayload struct {
	EventID   string      `json:"eventId"`
	Event     string      `json:"event"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data"`
}
