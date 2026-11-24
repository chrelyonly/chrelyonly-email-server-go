package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chrelyonly-email-server-go/internal/config"
	"chrelyonly-email-server-go/pkg/utils"
)

type StorageManager struct {
	messageDir    string
	attachmentDir string
}

func NewStorageManager(cfg config.StorageConfig) (*StorageManager, error) {
	msgDir := filepath.Clean(cfg.MessagePath)
	attDir := filepath.Clean(cfg.AttachmentPath)

	if err := os.MkdirAll(msgDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建邮件存储目录 %s: %w", msgDir, err)
	}
	if err := os.MkdirAll(attDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建附件存储目录 %s: %w", attDir, err)
	}

	return &StorageManager{
		messageDir:    msgDir,
		attachmentDir: attDir,
	}, nil
}

// SaveRawMessage 保存原始 EML 格式数据
func (s *StorageManager) SaveRawMessage(data []byte) (string, error) {
	now := time.Now()
	subDir := filepath.Join(s.messageDir, now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	filename := fmt.Sprintf("%s.eml", utils.GenerateUUID())
	absPath := filepath.Join(subDir, filename)

	if err := os.WriteFile(absPath, data, 0600); err != nil {
		return "", fmt.Errorf("写入原始邮件失败: %w", err)
	}

	relPath, err := filepath.Rel(s.messageDir, absPath)
	if err != nil {
		return absPath, nil
	}
	return relPath, nil
}

// SaveAttachment 保存邮件附件文件
func (s *StorageManager) SaveAttachment(filename string, r io.Reader) (string, int64, error) {
	now := time.Now()
	subDir := filepath.Join(s.attachmentDir, now.Format("2006"))
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return "", 0, fmt.Errorf("创建附件目录失败: %w", err)
	}

	// 安全处理文件名，防止目录穿越
	safeFilename := filepath.Base(filename)
	safeFilename = strings.ReplaceAll(safeFilename, "..", "")
	if safeFilename == "" || safeFilename == "." {
		safeFilename = "attachment.bin"
	}

	saveName := fmt.Sprintf("%s_%s", utils.GenerateUUID(), safeFilename)
	absPath := filepath.Join(subDir, saveName)

	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", 0, fmt.Errorf("无法创建附件文件: %w", err)
	}
	defer file.Close()

	n, err := io.Copy(file, r)
	if err != nil {
		return "", 0, fmt.Errorf("写入附件失败: %w", err)
	}

	relPath, err := filepath.Rel(s.attachmentDir, absPath)
	if err != nil {
		return absPath, n, nil
	}
	return relPath, n, nil
}

// ReadRawMessage 读取原始邮件数据
func (s *StorageManager) ReadRawMessage(relPath string) ([]byte, error) {
	fullPath := filepath.Join(s.messageDir, filepath.Clean(relPath))
	return os.ReadFile(fullPath)
}

// ReadAttachmentReader 获取附件文件的 ReadCloser
func (s *StorageManager) ReadAttachmentReader(relPath string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.attachmentDir, filepath.Clean(relPath))
	return os.Open(fullPath)
}

// DeleteMessageFile 删除原始邮件文件
func (s *StorageManager) DeleteMessageFile(relPath string) error {
	fullPath := filepath.Join(s.messageDir, filepath.Clean(relPath))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// DeleteAttachmentFile 删除附件文件
func (s *StorageManager) DeleteAttachmentFile(relPath string) error {
	fullPath := filepath.Join(s.attachmentDir, filepath.Clean(relPath))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsHealthy 检查存储读写健康状态
func (s *StorageManager) IsHealthy() bool {
	testFile := filepath.Join(s.messageDir, ".health_check")
	if err := os.WriteFile(testFile, []byte("ok"), 0644); err != nil {
		return false
	}
	_ = os.Remove(testFile)
	return true
}
