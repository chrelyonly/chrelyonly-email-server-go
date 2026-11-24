package imap

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"chrelyonly-email-server-go/internal/config"
	mimeParser "chrelyonly-email-server-go/internal/mime"
	"chrelyonly-email-server-go/internal/model"
	"chrelyonly-email-server-go/internal/repository"
	"chrelyonly-email-server-go/internal/security"
	"chrelyonly-email-server-go/internal/storage"
	"chrelyonly-email-server-go/internal/webhook"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	imapServer "github.com/emersion/go-imap/server"
	"go.uber.org/zap"
)

type IMAPServerManager struct {
	cfg        config.IMAPConfig
	repo       *repository.Repository
	storage    *storage.StorageManager
	dispatcher *webhook.Dispatcher
	logger     *zap.Logger
	server     *imapServer.Server
}

func NewIMAPServerManager(
	cfg config.IMAPConfig,
	repo *repository.Repository,
	storage *storage.StorageManager,
	dispatcher *webhook.Dispatcher,
	logger *zap.Logger,
) *IMAPServerManager {
	return &IMAPServerManager{
		cfg:        cfg,
		repo:       repo,
		storage:    storage,
		dispatcher: dispatcher,
		logger:     logger,
	}
}

type zapLogWriter struct {
	logger *zap.Logger
}

func (w *zapLogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if strings.Contains(msg, "forcibly closed") || strings.Contains(msg, "EOF") || strings.Contains(msg, "broken pipe") {
		w.logger.Debug("IMAP 客户端连接断开", zap.String("msg", msg))
	} else {
		w.logger.Warn("IMAP 服务提示", zap.String("msg", msg))
	}
	return len(p), nil
}

func (m *IMAPServerManager) Start() error {
	if !m.cfg.Enabled || m.cfg.Port <= 0 {
		m.logger.Info("IMAP 服务未启用")
		return nil
	}

	b := &Backend{
		manager: m,
	}

	s := imapServer.New(b)
	s.Addr = fmt.Sprintf(":%d", m.cfg.Port)
	s.AllowInsecureAuth = true
	s.ErrorLog = log.New(&zapLogWriter{logger: m.logger}, "", 0)

	m.server = s

	go func() {
		m.logger.Info("IMAP 服务开始监听", zap.String("addr", s.Addr))
		if err := s.ListenAndServe(); err != nil {
			m.logger.Error("IMAP 服务挂掉", zap.Error(err))
		}
	}()

	return nil
}

func (m *IMAPServerManager) Stop() {
	if m.server != nil {
		_ = m.server.Close()
	}
}

// --- Backend ---

type Backend struct {
	manager *IMAPServerManager
}

func (b *Backend) Login(connInfo *imap.ConnInfo, username, password string) (backend.User, error) {
	acc, err := b.manager.repo.GetAccountByEmail(username)
	if err != nil || acc == nil {
		b.manager.logger.Warn("IMAP 登录失败：数据库中未找到该账号", zap.String("username", username))
		return nil, backend.ErrInvalidCredentials
	}

	if !acc.IsActive {
		b.manager.logger.Warn("IMAP 登录失败：账号已被禁用", zap.String("username", username))
		return nil, backend.ErrInvalidCredentials
	}

	if !security.VerifyPassword(password, acc.PasswordHash) {
		b.manager.logger.Warn("IMAP 登录失败：密码校验不匹配", zap.String("username", username))
		return nil, backend.ErrInvalidCredentials
	}

	b.manager.logger.Info("IMAP 账户登录校验成功", zap.String("username", username))

	return &User{
		account: acc,
		manager: b.manager,
	}, nil
}

// --- User ---

type User struct {
	account *model.Account
	manager *IMAPServerManager
}

func (u *User) Username() string {
	return u.account.Email
}

func (u *User) ListMailboxes(subscribed bool) ([]backend.Mailbox, error) {
	u.manager.logger.Info("IMAP [LIST] 收到获取文件夹列表请求", zap.String("account", u.account.Email))
	inbox, _ := u.GetMailbox("INBOX")
	sent, _ := u.GetMailbox("Sent")
	trash, _ := u.GetMailbox("Trash")
	drafts, _ := u.GetMailbox("Drafts")
	return []backend.Mailbox{inbox, sent, trash, drafts}, nil
}

func (u *User) GetMailbox(name string) (backend.Mailbox, error) {
	upperName := strings.ToUpper(name)
	boxName := "INBOX"
	if strings.Contains(upperName, "SENT") {
		boxName = "Sent"
	} else if strings.Contains(upperName, "TRASH") || strings.Contains(upperName, "DELETE") {
		boxName = "Trash"
	} else if strings.Contains(upperName, "DRAFT") {
		boxName = "Drafts"
	}

	u.manager.logger.Info("IMAP [SELECT/EXAMINE] 收到选择文件夹请求",
		zap.String("account", u.account.Email),
		zap.String("requestedName", name),
		zap.String("mappedFolder", boxName),
	)

	return &Mailbox{
		name:    boxName,
		user:    u,
		manager: u.manager,
	}, nil
}

func (u *User) CreateMailbox(name string) error {
	return nil
}

func (u *User) DeleteMailbox(name string) error {
	return nil
}

func (u *User) RenameMailbox(existingName, newName string) error {
	return nil
}

func (u *User) Logout() error {
	u.manager.logger.Info("IMAP [LOGOUT] 客户端退出登录", zap.String("account", u.account.Email))
	return nil
}

// --- Mailbox ---

type Mailbox struct {
	name    string
	user    *User
	manager *IMAPServerManager
}

func (m *Mailbox) Name() string {
	return m.name
}

func (m *Mailbox) Info() (*imap.MailboxInfo, error) {
	attrs := []string{"\\HasNoChildren"}
	if m.name == "INBOX" {
		attrs = append(attrs, "\\Inbox")
	} else if m.name == "Sent" {
		attrs = append(attrs, "\\Sent")
	} else if m.name == "Trash" {
		attrs = append(attrs, "\\Trash")
	} else if m.name == "Drafts" {
		attrs = append(attrs, "\\Drafts")
	}
	return &imap.MailboxInfo{
		Attributes: attrs,
		Delimiter:  "/",
		Name:       m.name,
	}, nil
}

func (m *Mailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	messages, total, err := m.manager.repo.ListFolderMessages(m.user.account.Email, m.name, 1, 10000)
	if err != nil {
		m.manager.logger.Error("IMAP [STATUS] 读取数据库失败", zap.String("account", m.user.account.Email), zap.Error(err))
		return nil, err
	}

	var unseen uint32
	for _, msg := range messages {
		if !msg.IsRead {
			unseen++
		}
	}

	status := imap.NewMailboxStatus(m.name, items)
	status.Messages = uint32(total)
	status.Unseen = unseen
	status.Flags = []string{imap.SeenFlag, imap.DeletedFlag}
	status.PermanentFlags = []string{imap.SeenFlag, imap.DeletedFlag}
	status.UidValidity = 1
	status.UidNext = uint32(total) + 1

	m.manager.logger.Info("IMAP [STATUS] 返回文件夹状态",
		zap.String("account", m.user.account.Email),
		zap.String("folder", m.name),
		zap.Uint32("messagesCount", status.Messages),
		zap.Uint32("unseenCount", status.Unseen),
	)

	return status, nil
}

func (m *Mailbox) SetSubscribed(subscribed bool) error {
	return nil
}

func (m *Mailbox) Check() error {
	return nil
}

func (m *Mailbox) ListMessages(uid bool, seqSet *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	m.manager.logger.Info("IMAP [FETCH] 收到拉取邮件请求",
		zap.String("account", m.user.account.Email),
		zap.String("folder", m.name),
		zap.Bool("useUID", uid),
	)

	messages, _, err := m.manager.repo.ListFolderMessages(m.user.account.Email, m.name, 1, 1000)
	if err != nil {
		m.manager.logger.Error("IMAP [FETCH] 数据库查询失败", zap.Error(err))
		return err
	}

	m.manager.logger.Info("IMAP [FETCH] 数据库符合条件的邮件条数",
		zap.String("account", m.user.account.Email),
		zap.String("folder", m.name),
		zap.Int("dbTotalCount", len(messages)),
	)

	maxVal := uint32(len(messages))
	if uid && len(messages) > 0 {
		for _, msg := range messages {
			if uint32(msg.ID) > maxVal {
				maxVal = uint32(msg.ID)
			}
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	if seqSet.Dynamic() {
		for i := range seqSet.Set {
			if seqSet.Set[i].Start == 0 {
				seqSet.Set[i].Start = maxVal
			}
			if seqSet.Set[i].Stop == 0 {
				seqSet.Set[i].Stop = maxVal
			}
		}
	}

	pushedCount := 0
	for idx, msg := range messages {
		seqNum := uint32(idx + 1)
		msgUid := uint32(msg.ID)

		targetId := seqNum
		if uid {
			targetId = msgUid
		}

		if !seqSet.Contains(targetId) {
			continue
		}

		imapMsg := imap.NewMessage(seqNum, items)
		imapMsg.Uid = msgUid

		var flags []string
		if msg.IsRead {
			flags = append(flags, imap.SeenFlag)
		}
		imapMsg.Flags = flags
		imapMsg.InternalDate = msg.ReceivedAt

		rawEML, _ := m.manager.storage.ReadRawMessage(msg.RawFilePath)
		if len(rawEML) == 0 {
			rawEML, _ = mimeParser.BuildEML(model.SendMailRequest{
				From:    msg.FromAddress,
				To:      msg.To,
				Cc:      msg.Cc,
				Bcc:     msg.Bcc,
				Subject: msg.Subject,
				Text:    msg.TextContent,
				HTML:    msg.HTMLContent,
			}, msg.MessageID)
		}

		imapMsg.Size = uint32(len(rawEML))

		parsed, _ := mimeParser.ParseMessage(rawEML)
		fromStr := msg.FromAddress
		subjectStr := msg.Subject
		if parsed != nil {
			if parsed.From != "" {
				fromStr = parsed.From
			}
			if parsed.Subject != "" {
				subjectStr = parsed.Subject
			}
		}

		fromAddrs := parseAddresses(fromStr)
		if len(fromAddrs) == 0 {
			fromAddrs = []*imap.Address{{MailboxName: "unknown", HostName: "localhost"}}
		}

		toAddrs := parseAddressesSlice(msg.To)
		if parsed != nil && len(parsed.To) > 0 {
			toAddrs = parseAddressesSlice(parsed.To)
		}

		imapMsg.Envelope = &imap.Envelope{
			Date:      msg.ReceivedAt,
			Subject:   subjectStr,
			From:      fromAddrs,
			Sender:    fromAddrs,
			ReplyTo:   fromAddrs,
			To:        toAddrs,
			Cc:        parseAddressesSlice(msg.Cc),
			Bcc:       parseAddressesSlice(msg.Bcc),
			MessageId: msg.MessageID,
		}

		// 填充 BodyStructure (改用标准的 MIMEType 与 MIMESubType 字段)
		if parsed != nil && (parsed.HTMLContent != "" || parsed.TextContent != "") {
			if parsed.HTMLContent != "" && parsed.TextContent != "" {
				imapMsg.BodyStructure = &imap.BodyStructure{
					MIMEType:    "multipart",
					MIMESubType: "alternative",
					Parts: []*imap.BodyStructure{
						{
							MIMEType:    "text",
							MIMESubType: "plain",
							Params:      map[string]string{"charset": "utf-8"},
							Size:        uint32(len(parsed.TextContent)),
						},
						{
							MIMEType:    "text",
							MIMESubType: "html",
							Params:      map[string]string{"charset": "utf-8"},
							Size:        uint32(len(parsed.HTMLContent)),
						},
					},
				}
			} else if parsed.HTMLContent != "" {
				imapMsg.BodyStructure = &imap.BodyStructure{
					MIMEType:    "text",
					MIMESubType: "html",
					Params:      map[string]string{"charset": "utf-8"},
					Size:        uint32(len(parsed.HTMLContent)),
				}
			} else {
				imapMsg.BodyStructure = &imap.BodyStructure{
					MIMEType:    "text",
					MIMESubType: "plain",
					Params:      map[string]string{"charset": "utf-8"},
					Size:        uint32(len(parsed.TextContent)),
				}
			}
		} else {
			imapMsg.BodyStructure = &imap.BodyStructure{
				MIMEType:    "text",
				MIMESubType: "plain",
				Params:      map[string]string{"charset": "utf-8"},
				Size:        uint32(len(msg.TextContent)),
			}
		}

		for _, item := range items {
			section, err := imap.ParseBodySectionName(item)
			if err == nil && len(rawEML) > 0 {
				imapMsg.Body[section] = bytes.NewReader(rawEML)
			}
		}

		ch <- imapMsg
		pushedCount++
	}

	m.manager.logger.Info("IMAP [FETCH] 成功向客户端推送邮件列表",
		zap.String("account", m.user.account.Email),
		zap.Int("pushedCount", pushedCount),
	)

	return nil
}

func (m *Mailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	m.manager.logger.Info("IMAP [SEARCH] 收到搜索邮件请求", zap.String("account", m.user.account.Email), zap.String("folder", m.name))
	messages, _, err := m.manager.repo.ListFolderMessages(m.user.account.Email, m.name, 1, 1000)
	if err != nil {
		return nil, err
	}

	var res []uint32
	for idx, msg := range messages {
		seqNum := uint32(idx + 1)
		msgUid := uint32(msg.ID)

		val := seqNum
		if uid {
			val = msgUid
		}
		res = append(res, val)
	}

	m.manager.logger.Info("IMAP [SEARCH] 返回匹配序列", zap.String("account", m.user.account.Email), zap.Int("matchCount", len(res)))
	return res, nil
}

func (m *Mailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	return nil
}

func (m *Mailbox) UpdateMessagesFlags(uid bool, seqSet *imap.SeqSet, operation imap.FlagsOp, flags []string) error {
	hasSeen := false
	for _, f := range flags {
		if f == imap.SeenFlag {
			hasSeen = true
			break
		}
	}

	if hasSeen && (operation == imap.AddFlags || operation == imap.SetFlags) {
		messages, _, err := m.manager.repo.ListFolderMessages(m.user.account.Email, m.name, 1, 1000)
		if err == nil {
			for idx, msg := range messages {
				targetId := uint32(idx + 1)
				if uid {
					targetId = uint32(msg.ID)
				}
				if seqSet.Contains(targetId) {
					_ = m.manager.dispatcher.DispatchEvent("mail.read", msg.MessageID, map[string]interface{}{
						"messageId": msg.MessageID,
						"account":   m.user.account.Email,
						"readAt":    time.Now().Format(time.RFC3339),
					})
				}
			}
		}
	}

	return nil
}

func (m *Mailbox) CopyMessages(uid bool, seqSet *imap.SeqSet, destName string) error {
	return nil
}

func (m *Mailbox) Expunge() error {
	return nil
}

// 辅助解析地址工具类
func parseAddress(addrStr string) *imap.Address {
	parts := strings.Split(strings.TrimSpace(addrStr), "@")
	if len(parts) == 2 {
		return &imap.Address{
			MailboxName: parts[0],
			HostName:    parts[1],
		}
	}
	if addrStr != "" {
		return &imap.Address{
			MailboxName: addrStr,
		}
	}
	return nil
}

func parseAddresses(addrStr string) []*imap.Address {
	if addrStr == "" {
		return nil
	}
	addr := parseAddress(addrStr)
	if addr == nil {
		return nil
	}
	return []*imap.Address{addr}
}

func parseAddressesSlice(addrs []string) []*imap.Address {
	var list []*imap.Address
	for _, a := range addrs {
		if addr := parseAddress(a); addr != nil {
			list = append(list, addr)
		}
	}
	return list
}
