package models

import (
	"time"
)

type Mailbox struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint64    `gorm:"index;not null" json:"user_id"`
	Name        string    `gorm:"type:varchar(64);not null" json:"name"` // INBOX, Sent, Drafts, Trash, Junk
	UIDNext     uint32    `gorm:"default:1" json:"uid_next"`
	UIDValidity uint32    `gorm:"default:1" json:"uid_validity"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
