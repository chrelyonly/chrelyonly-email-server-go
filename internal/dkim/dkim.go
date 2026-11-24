package dkim

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/emersion/go-msgauth/dkim"
)

// SignMessage 给原始 EML 邮件添加 DKIM 签名
func SignMessage(domain string, selector string, privateKeyPEM string, rawMessage []byte) ([]byte, error) {
	if domain == "" || selector == "" || privateKeyPEM == "" {
		// 无需 DKIM 签名，直接返回原文
		return rawMessage, nil
	}

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return rawMessage, fmt.Errorf("无法解析 DKIM PEM 私钥块")
	}

	var parsedKey interface{}
	var err error
	if key, e := x509.ParsePKCS1PrivateKey(block.Bytes); e == nil {
		parsedKey = key
	} else if key, e := x509.ParsePKCS8PrivateKey(block.Bytes); e == nil {
		parsedKey = key
	} else {
		return rawMessage, fmt.Errorf("无法解析 PKCS1/PKCS8 格式私钥: %w", err)
	}

	signer, ok := parsedKey.(crypto.Signer)
	if !ok {
		return rawMessage, fmt.Errorf("该私钥未实现 crypto.Signer 接口")
	}

	options := &dkim.SignOptions{
		Domain:     domain,
		Selector:   selector,
		Signer:     signer,
		HeaderKeys: []string{"from", "to", "subject", "date", "message-id"},
	}

	var signedBuf bytes.Buffer
	if err := dkim.Sign(&signedBuf, bytes.NewReader(rawMessage), options); err != nil {
		return rawMessage, fmt.Errorf("DKIM 签名执行失败: %w", err)
	}

	return signedBuf.Bytes(), nil
}
