package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// GenerateID 生成指定前缀的唯一随机ID
func GenerateID(prefix string) string {
	bytes := make([]byte, 12)
	_, _ = rand.Read(bytes)
	timestamp := time.Now().Format("20060102150405")
	return fmt.Sprintf("%s_%s_%s", prefix, timestamp, hex.EncodeToString(bytes)[:8])
}

// GenerateMessageID 生成标准邮件 Message-ID 标头
func GenerateMessageID(domain string) string {
	if domain == "" {
		domain = "localhost"
	}
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	hexStr := hex.EncodeToString(bytes)
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), hexStr[:16], domain)
}

// GenerateUUID 生成标准的 UUID v4 简易替代字符串
func GenerateUUID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

// ExtractDomainFromEmail 从邮箱字符串中提取域名
func ExtractDomainFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return strings.ToLower(strings.TrimSpace(parts[1]))
	}
	return ""
}
