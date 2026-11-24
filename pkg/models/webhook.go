package models

import (
	"time"
)

type WebhookLog struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	EventType    string    `gorm:"type:varchar(64);not null" json:"event_type"` // email.received, email.sent
	EmailID      uint64    `gorm:"index" json:"email_id"`
	TargetURL    string    `gorm:"type:varchar(512);not null" json:"target_url"`
	StatusCode   int       `json:"status_code"`
	ResponseBody string    `gorm:"type:text" json:"response_body"`
	Error        string    `gorm:"type:text" json:"error"`
	RetryCount   int       `json:"retry_count"`
	CreatedAt    time.Time `json:"created_at"`
}
