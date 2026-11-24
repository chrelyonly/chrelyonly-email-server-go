package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	SMTP     SMTPConfig     `yaml:"smtp"`
	IMAP     IMAPConfig     `yaml:"imap"`
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

type SMTPPortsConfig struct {
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
	Hostname string          `yaml:"hostname"`
	Ports    SMTPPortsConfig `yaml:"ports"`
	TLS      TLSConfig       `yaml:"tls"`
}

type IMAPConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type StorageConfig struct {
	MessagePath    string `yaml:"message_path"`
	AttachmentPath string `yaml:"attachment_path"`
	MaxMessageSize int64  `yaml:"max_message_size"`
	MaxAttachSize  int64  `yaml:"max_attachment_size"`
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
	Level       string   `yaml:"level"`
	Encoding    string   `yaml:"encoding"`
	OutputPaths []string `yaml:"output_paths"`
}

// LoadConfig reads config from YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	// Set sensible defaults if empty
	if cfg.Server.HTTPAddr == "" {
		cfg.Server.HTTPAddr = "127.0.0.1:18080"
	}
	if cfg.SMTP.Hostname == "" {
		cfg.SMTP.Hostname = "localhost"
	}
	if cfg.SMTP.Ports.SMTP == 0 {
		cfg.SMTP.Ports.SMTP = 25
	}
	if cfg.SMTP.Ports.Submission == 0 {
		cfg.SMTP.Ports.Submission = 587
	}
	if cfg.SMTP.Ports.SMTPS == 0 {
		cfg.SMTP.Ports.SMTPS = 465
	}
	if cfg.IMAP.Port == 0 {
		cfg.IMAP.Port = 143
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

	return &cfg, nil
}
