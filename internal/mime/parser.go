package mime

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"chrelyonly-email-server-go/internal/model"
	"chrelyonly-email-server-go/pkg/utils"

	"github.com/emersion/go-message/mail"
)

// ParsedEmail 结构体，包含解包后的完整邮件元数据
type ParsedEmail struct {
	MessageID   string
	From        string
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	TextContent string
	HTMLContent string
	Attachments []ParsedAttachment
}

// ParsedAttachment 包含解析到的邮件附件信息
type ParsedAttachment struct {
	Filename    string
	ContentType string
	ContentID   string
	Data        []byte
}

// ParseMessage 解析原始 EML 数据
func ParseMessage(raw []byte) (*ParsedEmail, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("解析邮件头失败: %w", err)
	}

	header := mr.Header
	parsed := &ParsedEmail{}

	// 解析 Message-ID
	msgID := header.Get("Message-ID")
	if msgID == "" {
		msgID = utils.GenerateMessageID("chrelyonly.cn")
	}
	parsed.MessageID = msgID

	// 解析 Subject
	subject, _ := header.Subject()
	parsed.Subject = subject

	// 解析 From
	fromList, err := header.AddressList("From")
	if err == nil && len(fromList) > 0 {
		parsed.From = fromList[0].Address
	} else {
		parsed.From = header.Get("From")
	}

	// 解析 To
	toList, err := header.AddressList("To")
	if err == nil {
		for _, addr := range toList {
			parsed.To = append(parsed.To, addr.Address)
		}
	}

	// 解析 Cc
	ccList, err := header.AddressList("Cc")
	if err == nil {
		for _, addr := range ccList {
			parsed.Cc = append(parsed.Cc, addr.Address)
		}
	}

	// 解析 Body 和 Attachments
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := h.ContentType()
			b, _ := io.ReadAll(p.Body)
			if contentType == "text/plain" && parsed.TextContent == "" {
				parsed.TextContent = string(b)
			} else if contentType == "text/html" && parsed.HTMLContent == "" {
				parsed.HTMLContent = string(b)
			}
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			if filename == "" {
				filename = "attachment.bin"
			}
			contentType, _, _ := h.ContentType()
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			contentID := h.Get("Content-ID")
			data, _ := io.ReadAll(p.Body)

			parsed.Attachments = append(parsed.Attachments, ParsedAttachment{
				Filename:    filename,
				ContentType: contentType,
				ContentID:   contentID,
				Data:        data,
			})
		}
	}

	return parsed, nil
}

// BuildEML 生成符合 RFC 2045 / RFC 5322 标准的原始 EML 格式数据
func BuildEML(req model.SendMailRequest, msgID string) ([]byte, error) {
	var buf bytes.Buffer

	boundaryMixed := fmt.Sprintf("=_Mix_%d", time.Now().UnixNano())
	boundaryAlt := fmt.Sprintf("=_Alt_%d", time.Now().UnixNano())

	buf.WriteString(fmt.Sprintf("From: %s\r\n", req.From))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(req.To, ", ")))
	if len(req.Cc) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(req.Cc, ", ")))
	}
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("utf-8", req.Subject)))
	buf.WriteString(fmt.Sprintf("Message-ID: %s\r\n", msgID))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString("MIME-Version: 1.0\r\n")

	hasAttachments := len(req.Attachments) > 0

	if hasAttachments {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundaryMixed))
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
	}

	if req.HTML != "" && req.Text != "" {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundaryAlt))

		// 纯文本部分
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(req.Text)))
		buf.WriteString("\r\n")

		// HTML 部分
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(req.HTML)))
		buf.WriteString("\r\n")

		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryAlt))
	} else if req.HTML != "" {
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(req.HTML)))
		buf.WriteString("\r\n")
	} else {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(req.Text)))
		buf.WriteString("\r\n")
	}

	// 附件部分
	if hasAttachments {
		for _, att := range req.Attachments {
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
			buf.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.ContentType, mime.QEncoding.Encode("utf-8", att.Filename)))
			buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", mime.QEncoding.Encode("utf-8", att.Filename)))
			buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
			buf.WriteString(att.DataBase64)
			buf.WriteString("\r\n")
		}
		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryMixed))
	}

	return buf.Bytes(), nil
}
