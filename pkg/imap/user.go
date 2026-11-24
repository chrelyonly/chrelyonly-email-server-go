package imap

import (
	"chrelyonly-email-server-go/pkg/models"
	"chrelyonly-email-server-go/pkg/storage"
	"github.com/emersion/go-imap/backend"
	"gorm.io/gorm"
)

type User struct {
	db      *gorm.DB
	storage *storage.StorageManager
	user    *models.User
}

func (u *User) Username() string {
	return u.user.Email
}

func (u *User) ListMailboxes(subscribed bool) ([]backend.Mailbox, error) {
	var mbs []models.Mailbox
	if err := u.db.Where("user_id = ?", u.user.ID).Find(&mbs).Error; err != nil {
		return nil, err
	}

	var res []backend.Mailbox
	for i := range mbs {
		res = append(res, &Mailbox{
			db:      u.db,
			storage: u.storage,
			user:    u.user,
			mailbox: &mbs[i],
		})
	}
	return res, nil
}

func (u *User) GetMailbox(name string) (backend.Mailbox, error) {
	var mb models.Mailbox
	if err := u.db.Where("user_id = ? AND name = ?", u.user.ID, name).First(&mb).Error; err != nil {
		return nil, backend.ErrNoSuchMailbox
	}

	return &Mailbox{
		db:      u.db,
		storage: u.storage,
		user:    u.user,
		mailbox: &mb,
	}, nil
}

func (u *User) CreateMailbox(name string) error {
	var count int64
	u.db.Model(&models.Mailbox{}).Where("user_id = ? AND name = ?", u.user.ID, name).Count(&count)
	if count > 0 {
		return backend.ErrMailboxAlreadyExists
	}

	mb := models.Mailbox{
		UserID:      u.user.ID,
		Name:        name,
		UIDNext:     1,
		UIDValidity: 1,
	}
	return u.db.Create(&mb).Error
}

func (u *User) DeleteMailbox(name string) error {
	if name == "INBOX" {
		return backend.ErrNoSuchMailbox
	}
	return u.db.Where("user_id = ? AND name = ?", u.user.ID, name).Delete(&models.Mailbox{}).Error
}

func (u *User) RenameMailbox(existingName, newName string) error {
	if existingName == "INBOX" {
		return backend.ErrNoSuchMailbox
	}
	return u.db.Model(&models.Mailbox{}).Where("user_id = ? AND name = ?", u.user.ID, existingName).Update("name", newName).Error
}

func (u *User) Logout() error {
	return nil
}
