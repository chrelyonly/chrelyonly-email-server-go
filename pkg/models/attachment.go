package models

import (
	"time"
)

type Attachment struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	EmailID     uint64    `gorm:"index;not null" json:"email_id"`
	Filename    string    `gorm:"type:varchar(255);not null" json:"filename"`
	ContentType string    `gorm:"type:varchar(128)" json:"content_type"`
	Size        int64     `gorm:"not null" json:"size"`
	FilePath    string    `gorm:"type:varchar(512);not null" json:"file_path"`
	ContentID   string    `gorm:"type:varchar(128)" json:"content_id"`
	CreatedAt   time.Time `json:"created_at"`
}
