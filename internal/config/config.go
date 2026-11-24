package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 全局配置结构体
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	SMTP     SMTPConfig     `yaml:"smtp"`
	Database DatabaseConfig `yaml:"database"`
	Storage  StorageConfig  `yaml:"storage"`
	Webhook  WebhookConfig  `yaml:"webhook"`
	Security SecurityConfig `yaml:"security"`
	Logger   LoggerConfig   `yaml:"logger"`
}

type ServerConfig struct {
	Name     string `yaml:"name"`
	HTTPAddr string `yaml:"http_addr"`
}

type SMTPPorts struct {
	SMTP       int `yaml:"smtp"`
	SMTPS      int `yaml:"smtps"`
	Submission int `yaml:"submission"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type SMTPConfig struct {
	Hostname string    `yaml:"hostname"`
	Ports    SMTPPorts `yaml:"ports"`
	TLS      TLSConfig `yaml:"tls"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type StorageConfig struct {
	MessagePath       string `yaml:"message_path"`
	AttachmentPath    string `yaml:"attachment_path"`
	MaxMessageSize    int64  `yaml:"max_message_size"`
	MaxAttachmentSize int64  `yaml:"max_attachment_size"`
}

type WebhookConfig struct {
	Enabled    bool   `yaml:"enabled"`
	URL        string `yaml:"url"`
	Secret     string `yaml:"secret"`
	Retry      bool   `yaml:"retry"`
	MaxRetries int    `yaml:"max_retries"`
}

type SecurityConfig struct {
	MaxConnections  int `yaml:"max_connections"`
	MaxAuthAttempts int `yaml:"max_auth_attempts"`
}

type LoggerConfig struct {
	Level       string   `yaml:"level"`        // debug / info / warn / error
	Encoding    string   `yaml:"encoding"`     // console / json
	OutputPaths []string `yaml:"output_paths"` // ["stdout", "./logs/server.log"]
}

// LoadConfig 从指定 YAML 文件路径加载配置
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("CONFIG_PATH")
		if path == "" {
			path = "configs/config.yaml"
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取配置文件 %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}

	// 补全默认值
	if cfg.Server.HTTPAddr == "" {
		cfg.Server.HTTPAddr = "127.0.0.1:18080"
	}
	if cfg.Storage.MessagePath == "" {
		cfg.Storage.MessagePath = "./data/messages"
	}
	if cfg.Storage.AttachmentPath == "" {
		cfg.Storage.AttachmentPath = "./data/attachments"
	}
	if cfg.Storage.MaxMessageSize == 0 {
		cfg.Storage.MaxMessageSize = 26214400 // 25MB
	}
	if cfg.Storage.MaxAttachmentSize == 0 {
		cfg.Storage.MaxAttachmentSize = 20971520 // 20MB
	}
	if cfg.Webhook.MaxRetries == 0 {
		cfg.Webhook.MaxRetries = 10
	}
	if cfg.Logger.Level == "" {
		cfg.Logger.Level = "info"
	}
	if cfg.Logger.Encoding == "" {
		cfg.Logger.Encoding = "console"
	}
	if len(cfg.Logger.OutputPaths) == 0 {
		cfg.Logger.OutputPaths = []string{"stdout"}
	}

	return &cfg, nil
}
