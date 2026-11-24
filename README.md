# Chrelyonly Email Server (Go)

基于 `configs/config.yaml` 实现的高性能、功能完备的邮件收发管理服务器，支持完整的 **SMTP 邮件收发**、**IMAP 客户端登录同步**、**MySQL 用户与邮件持久化**、**RESTful API 管理**、**本地文件存储** 及 **安全 Webhook 通知**。

---

## 🚀 核心功能特性

1. **多协议支持**：
   - **SMTP Server (端口 25 / 465 / 587)**：支持标准收信、Submission 认证发信、TLS 加密传输及外部邮件智能转发（MX 路由）。
   - **IMAP Server (端口 143)**：完美支持 Foxmail、Outlook、Thunderbird、Apple Mail / 手机自带邮件客户端登录收信、同步文件夹（INBOX / Sent / Drafts / Trash / Junk）、标记已读/星标/删除。
2. **数据库持久化 (MySQL / GORM)**：
   - 自动迁移用户账户、邮箱文件夹、邮件元数据、附件信息、Webhook 日志。
3. **HTTP RESTful API (端口 18080)**：
   - 系统状态监控 (`/health`, `/api/v1/system/status`)
   - 用户账号管理 (创建、列表、改密、删除)
   - 邮件收发管理 (API 发信、查询邮件列表、下载 `.eml` 原文与附件)
   - Webhook 测试与事件日志
4. **安全与日志**：
   - 基于 `zap` 的高性能结构化日志（同时输出至终端与 `./logs/server.log`）。
   - Webhook HMAC-SHA256 签名校验与自动重试机制。

---

## 📦 项目结构

```
chrelyonly-email-server-go/
├── configs/
│   └── config.yaml      # 核心配置文件
├── pkg/
│   ├── api/             # Gin HTTP REST API 路由与处理器
│   ├── config/          # YAML 配置解析
│   ├── db/              # MySQL 数据库连接与 AutoMigrate
│   ├── imap/            # IMAP v4rev1 服务端实现 (支持第三方客户端)
│   ├── logger/          # Zap 日志记录器
│   ├── models/          # GORM 数据模型 (User, Mailbox, Email, Attachment, WebhookLog)
│   ├── smtp/            # SMTP 服务端、Session、MIME 解析与 MX Relayer
│   ├── storage/         # 本地邮件 .eml 与附件二进制存储管理
│   └── webhook/         # 异步 Webhook 事件分发与重试
├── main.go              # 程序入口
├── build.bat            # Windows 编译脚本
└── README.md            # 项目说明文档
```

---

## ⚙️ 配置说明 (`configs/config.yaml`)

参考您的配置文件：
```yaml
server:
  name: chrelyonly-mail-server
  http_addr: 127.0.0.1:18080

smtp:
  hostname: 192.168.10.70
  ports:
    smtp: 25
    smtps: 465
    submission: 587
  tls:
    enabled: false
    cert_file: "1"
    key_file: "2"

imap:
  enabled: true
  port: 143

database:
  driver: mysql
  dsn: chrelyonly-email:hSf3Yfx5PY2ry7cM@tcp(192.168.10.47:3306)/chrelyonly-email?parseTime=true&charset=utf8mb4&loc=Local

storage:
  message_path: ./data/messages
  attachment_path: ./data/attachments
  max_message_size: 26214400
  max_attachment_size: 20971520
```

---

## 🛠️ 编译与运行

### 1. 编译
运行项目根目录下的编译脚本 (`build.bat`)：
```cmd
build.bat
```
或直接使用 Go 命令：
```cmd
go build -o mailserver.exe main.go
```

### 2. 启动服务
```cmd
./mailserver.exe
```
> **提示**：首次启动若数据库中无用户，系统会自动创建一个默认测试用户：
> - **邮箱**: `admin@192.168.10.70`
> - **密码**: `123456`

---

## 📱 第三方客户端配置指南 (Foxmail / Outlook / Thunderbird)

您可以在 Foxmail 或 Outlook 中配置并登录收发邮件：

- **账号 / 邮箱地址**：`admin@192.168.10.70`
- **密码**：`123456`
- **接收服务器 (IMAP)**：
  - 服务器地址：`192.168.10.70` (或 `127.0.0.1`)
  - 端口：`143`
  - SSL：无 / 关闭
- **发送服务器 (SMTP)**：
  - 服务器地址：`192.168.10.70` (或 `127.0.0.1`)
  - 端口：`587` (或 `25`)
  - SSL：无 / 关闭
  - 认证：需要服务器验证 (使用相同账号密码)

---

## 🔌 RESTful API 接口速览

### 1. 健康检查
- `GET /health`

### 2. 系统状态
- `GET /api/v1/system/status`

### 3. 用户管理
- `POST /api/v1/users` (创建用户)
- `GET /api/v1/users` (用户列表)
- `PUT /api/v1/users/:id/password` (修改密码)

### 4. 邮件收发
- `POST /api/v1/emails/send` (API 发送邮件)
- `GET /api/v1/emails?user_id=1&mailbox=INBOX` (查询邮件列表)
- `GET /api/v1/emails/:id` (获取邮件详情)
- `GET /api/v1/emails/:id/raw` (下载 `.eml` 原文)
