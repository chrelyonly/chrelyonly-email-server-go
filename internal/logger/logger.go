package logger

import (
	"os"
	"path/filepath"
	"strings"

	"chrelyonly-email-server-go/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

// InitLogger 初始化 Zap 全局日志记录器
func InitLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	var zapCfg zap.Config

	if strings.ToLower(cfg.Encoding) == "json" {
		zapCfg = zap.NewProductionConfig()
		zapCfg.Encoding = "json"
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.Encoding = "console"
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// 自定义时间格式为: 2006-01-02 15:04:05
	zapCfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")

	// 设置日志级别
	switch strings.ToLower(cfg.Level) {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "warn":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// 设置输出路径并确保日志文件夹存在
	if len(cfg.OutputPaths) > 0 {
		for _, path := range cfg.OutputPaths {
			if path != "stdout" && path != "stderr" {
				dir := filepath.Dir(path)
				_ = os.MkdirAll(dir, 0755)
			}
		}
		zapCfg.OutputPaths = cfg.OutputPaths
	} else {
		zapCfg.OutputPaths = []string{"stdout"}
	}

	logger, err := zapCfg.Build()
	if err != nil {
		return nil, err
	}

	Log = logger
	zap.ReplaceGlobals(logger)
	return logger, nil
}
