package models

import (
	"time"
)

type Email struct {
	ID          uint64       `gorm:"primaryKey;autoIncrement" json:"id"`
	MessageID   string       `gorm:"type:varchar(255);index" json:"message_id"`
	MailboxID   uint64       `gorm:"index;not null" json:"mailbox_id"`
	UserID      uint64       `gorm:"index;not null" json:"user_id"`
	UID         uint32       `gorm:"index;not null" json:"uid"`
	FromAddress string       `gorm:"type:varchar(255);not null" json:"from_address"`
	ToAddress   string       `gorm:"type:text;not null" json:"to_address"`
	CcAddress   string       `gorm:"type:text" json:"cc_address"`
	BccAddress  string       `gorm:"type:text" json:"bcc_address"`
	Subject     string       `gorm:"type:varchar(512)" json:"subject"`
	TextBody    string       `gorm:"type:longtext" json:"text_body"`
	HtmlBody    string       `gorm:"type:longtext" json:"html_body"`
	RawPath     string       `gorm:"type:varchar(512)" json:"raw_path"`
	Size        int64        `gorm:"not null" json:"size"`
	IsRead      bool         `gorm:"default:false" json:"is_read"`
	IsFlagged   bool         `gorm:"default:false" json:"is_flagged"`
	IsDeleted   bool         `gorm:"default:false" json:"is_deleted"`
	IsDraft     bool         `gorm:"default:false" json:"is_draft"`
	SentAt      time.Time    `json:"sent_at"`
	ReceivedAt  time.Time    `json:"received_at"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Attachments []Attachment `gorm:"foreignKey:EmailID" json:"attachments,omitempty"`
}
