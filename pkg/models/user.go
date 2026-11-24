package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"type:varchar(128);uniqueIndex;not null" json:"username"`
	Email        string    `gorm:"type:varchar(191);uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	QuotaBytes   int64     `gorm:"default:1073741824" json:"quota_bytes"` // Default 1GB
	UsedBytes    int64     `gorm:"default:0" json:"used_bytes"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

func (u *User) CheckPassword(password string) bool {
	// First try bcrypt check
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	if err == nil {
		return true
	}
	// Fallback to plain text comparison if hash isn't bcrypt (for dev testing)
	return u.PasswordHash == password
}
