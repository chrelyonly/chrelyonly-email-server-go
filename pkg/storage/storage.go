package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"chrelyonly-email-server-go/pkg/config"
)

type StorageManager struct {
	cfg config.StorageConfig
}

func NewStorageManager(cfg config.StorageConfig) (*StorageManager, error) {
	if err := os.MkdirAll(cfg.MessagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create message storage directory: %w", err)
	}
	if err := os.MkdirAll(cfg.AttachmentPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create attachment storage directory: %w", err)
	}
	return &StorageManager{cfg: cfg}, nil
}

// SaveMessage writes raw .eml bytes to file
func (s *StorageManager) SaveMessage(userID uint64, msgID string, rawBytes []byte) (string, error) {
	if int64(len(rawBytes)) > s.cfg.MaxMessageSize {
		return "", fmt.Errorf("message size (%d) exceeds max allowed limit (%d)", len(rawBytes), s.cfg.MaxMessageSize)
	}

	now := time.Now()
	subDir := filepath.Join(s.cfg.MessagePath, fmt.Sprintf("%d", userID), now.Format("2006/01"))
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return "", err
	}

	fileName := fmt.Sprintf("%d_%s.eml", now.UnixNano(), cleanFilename(msgID))
	fullPath := filepath.Join(subDir, fileName)

	if err := os.WriteFile(fullPath, rawBytes, 0644); err != nil {
		return "", err
	}
	return fullPath, nil
}

// ReadMessage reads raw .eml bytes from file
func (s *StorageManager) ReadMessage(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

// SaveAttachment saves an attachment file to disk
func (s *StorageManager) SaveAttachment(emailID uint64, filename string, r io.Reader) (string, int64, error) {
	now := time.Now()
	subDir := filepath.Join(s.cfg.AttachmentPath, now.Format("2006/01"))
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return "", 0, err
	}

	safeName := fmt.Sprintf("%d_%d_%s", emailID, now.UnixNano(), cleanFilename(filename))
	fullPath := filepath.Join(subDir, safeName)

	file, err := os.Create(fullPath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	written, err := io.Copy(file, r)
	if err != nil {
		return "", 0, err
	}

	if s.cfg.MaxAttachSize > 0 && written > s.cfg.MaxAttachSize {
		os.Remove(fullPath)
		return "", 0, fmt.Errorf("attachment size (%d) exceeds limit (%d)", written, s.cfg.MaxAttachSize)
	}

	return fullPath, written, nil
}

func cleanFilename(name string) string {
	if name == "" {
		return "unnamed"
	}
	// Sanitize illegal path chars
	reg := []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|'}
	out := []rune(name)
	for i, r := range out {
		for _, b := range reg {
			if r == b {
				out[i] = '_'
			}
		}
	}
	return string(out)
}
