package database

import (
	"database/sql"
	"fmt"
	"time"

	"chrelyonly-email-server-go/internal/config"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// InitDB 初始化数据库连接池（包含失败自动重试机制）
func InitDB(cfg config.DatabaseConfig, log *zap.Logger) (*sql.DB, error) {
	if cfg.Driver != "mysql" {
		return nil, fmt.Errorf("不支持的数据库驱动: %s", cfg.Driver)
	}

	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("打开数据库连接失败: %w", err)
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	maxRetries := 12
	retryInterval := 5 * time.Second

	for i := 1; i <= maxRetries; i++ {
		err := db.Ping()
		if err == nil {
			if log != nil {
				log.Info("数据库连接与 Ping 校验成功！")
			}
			return db, nil
		}

		if log != nil {
			log.Warn(fmt.Sprintf("数据库连接失败，%v 后尝试第 %d/%d 次重试...", retryInterval, i, maxRetries), zap.Error(err))
		}
		time.Sleep(retryInterval)
	}

	return nil, fmt.Errorf("连续重试 %d 次后仍然无法建立数据库连接: %w", maxRetries, err)
}
