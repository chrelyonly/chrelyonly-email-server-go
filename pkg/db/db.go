package db

import (
	"fmt"
	"time"

	"chrelyonly-email-server-go/pkg/config"
	"chrelyonly-email-server-go/pkg/logger"
	"chrelyonly-email-server-go/pkg/models"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(cfg config.DatabaseConfig) (*gorm.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("数据库 DSN 不能为空")
	}

	gormConfig := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	}

	database, err := gorm.Open(mysql.Open(cfg.DSN), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 数据库失败: %w", err)
	}

	sqlDB, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 sql.DB 实例失败: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 自动迁移
	if err := database.AutoMigrate(
		&models.User{},
		&models.Mailbox{},
		&models.Email{},
		&models.Attachment{},
		&models.WebhookLog{},
	); err != nil {
		return nil, fmt.Errorf("数据库自动迁移失败: %w", err)
	}

	DB = database
	logger.Log.Info("数据库初始化成功", zap.String("驱动", cfg.Driver))
	return DB, nil
}

// CreateDefaultMailboxes 创建默认 IMAP 文件夹
func CreateDefaultMailboxes(tx *gorm.DB, userID uint64) error {
	defaultFolders := []string{"INBOX", "Sent", "Drafts", "Trash", "Junk"}
	for _, folder := range defaultFolders {
		var count int64
		tx.Model(&models.Mailbox{}).Where("user_id = ? AND name = ?", userID, folder).Count(&count)
		if count == 0 {
			mb := models.Mailbox{
				UserID:      userID,
				Name:        folder,
				UIDNext:     1,
				UIDValidity: uint32(time.Now().Unix()),
			}
			if err := tx.Create(&mb).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
