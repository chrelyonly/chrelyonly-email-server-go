package logger

import (
	"os"
	"path/filepath"
	"strings"

	"chrelyonly-email-server-go/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

// InitLogger initializes global zap logger
func InitLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	var level zapcore.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = zapcore.DebugLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	var encoder zapcore.Encoder
	if strings.ToLower(cfg.Encoding) == "json" {
		encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	var cores []zapcore.Core
	for _, path := range cfg.OutputPaths {
		if path == "stdout" {
			cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level))
		} else if path == "stderr" {
			cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stderr), level))
		} else {
			// Ensure log directory exists
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err == nil {
				file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err == nil {
					fileEncoderConfig := zap.NewProductionEncoderConfig()
					fileEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
					fileEncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
					fileEncoder := zapcore.NewConsoleEncoder(fileEncoderConfig)
					if strings.ToLower(cfg.Encoding) == "json" {
						fileEncoder = zapcore.NewJSONEncoder(fileEncoderConfig)
					}
					cores = append(cores, zapcore.NewCore(fileEncoder, zapcore.AddSync(file), level))
				}
			}
		}
	}

	if len(cores) == 0 {
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level))
	}

	core := zapcore.NewTee(cores...)
	Log = zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(Log)

	return Log, nil
}
