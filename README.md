# Chrelyonly Mail Server 🍓

## AI 项目开发需求文档

> 项目名称：Chrelyonly Mail  
> 项目定位：自主开发的邮件收发基础设施  
> 核心架构：Go Mail Server + Java Spring Boot API  
> 开发模式：前后端分离、服务解耦、HTTP API 通信

---

# 一、项目介绍

开发一套独立的邮件服务器系统，使用 Go 语言实现底层邮件协议和邮件收发功能，使用 Java Spring Boot 实现上层业务管理、REST API、账户管理和事件回调。

本项目不依赖 Postfix、Dovecot、Exim 等现成邮件服务器作为核心运行组件。

要求自主实现邮件服务器核心功能，合理使用成熟的 Go 邮件协议库，避免重复造轮子。

项目分为两个独立服务：

### 1. Go Mail Server

负责底层邮件基础设施：

- SMTP 邮件接收。
- SMTP 邮件发送。
- 邮件 MIME 解析。
- 邮件附件解析和存储。
- 邮件投递。
- 邮件账户认证。
- TLS 加密。
- DKIM 签名。
- SPF、DMARC 相关验证。
- 邮件队列管理。
- 邮件投递状态跟踪。
- 邮件事件通知。
- 可选 IMAP 服务。

### 2. Spring Boot Mail System

负责上层业务：

- 邮件 REST API。
- 邮箱账户管理。
- 域名管理。
- 邮件发送管理。
- 邮件收件箱管理。
- 邮件记录管理。
- 邮件附件管理。
- API Key 管理。
- Webhook 管理。
- 邮件统计。
- 系统配置管理。
- Go 邮件服务器状态监控。

两个项目独立部署、独立运行，不允许将 Go 的邮件核心逻辑放入 Spring Boot。

---

# 二、整体系统架构

```text
                    外部应用
                Android / Vue / Java
                        |
                        |
                   REST API
                        |
                        v
              +-------------------+
              |   Spring Boot     |
              |   Mail System     |
              +-------------------+
                        |
                 Internal API
                        |
                        v
              +-------------------+
              |   Go Mail Server  |
              |                   |
              | SMTP Server       |
              | SMTP Client       |
              | MIME Parser       |
              | Mail Delivery     |
              | Mail Queue        |
              | DKIM              |
              | TLS               |
              | Event Dispatcher  |
              +-------------------+
                    |       |
                    |       |
                    v       v
                 MySQL    Mail Storage
                    |
                    |
               Webhook Events
                    |
                    v
              Spring Boot
```

核心原则：

1. Go 负责邮件协议和实际收发。
2. Spring Boot 负责业务管理和 API。
3. Go 不直接依赖 Spring Boot 的数据库结构。
4. Spring Boot 不直接操作 Go 的内部存储。
5. 两个服务通过 HTTP API 通信。
6. Go 可以独立运行，即使 Spring Boot 暂时不可用，也不能丢失已经接收的邮件。
7. 邮件事件采用持久化队列和重试机制。

---

# 三、Go Mail Server 技术要求

## 3.1 开发语言

- Go。
- 使用 Go Modules。
- 代码结构清晰。
- 支持 Linux AMD64。
- 支持 Linux ARM64。
- 支持 Docker 部署。
- 尽量减少不必要的第三方依赖。

## 3.2 推荐依赖库

| 依赖 | 用途 |
|---|---|
| github.com/emersion/go-smtp | SMTP 服务端 |
| github.com/emersion/go-imap/v2 | IMAP 服务端 |
| github.com/emersion/go-message | MIME 邮件解析 |
| github.com/emersion/go-msgauth | 邮件认证与 DKIM 相关 |
| github.com/go-chi/chi/v5 | 内部 HTTP 路由 |
| github.com/go-sql-driver/mysql | MySQL 驱动 |
| go.uber.org/zap | 日志 |
| github.com/robfig/cron/v3 | 定时任务 |

注意：开发前必须核实依赖库当前版本、API 和兼容性，不允许凭空编写不存在的方法。

## 3.3 SMTP 服务

必须实现：

| 端口 | 功能 |
|---|---|
| 25 | SMTP 服务器间邮件投递 |
| 465 | SMTPS 隐式 TLS |
| 587 | SMTP Submission + STARTTLS |

支持：

- SMTP AUTH。
- LOGIN / PLAIN 等认证机制。
- STARTTLS。
- SMTP 隐式 TLS。
- 多域名。
- 多邮箱账户。
- 多收件人。
- CC / BCC。
- HTML 邮件。
- 纯文本邮件。
- 附件。
- 邮件大小限制。
- SMTP 命令错误处理。
- 发件人和收件人验证。
- 防止开放中继。

不允许未经身份验证的普通用户通过服务器向任意外部地址发送邮件。

## 3.4 SMTP 邮件接收

当外部邮件服务器向本服务器投递邮件时：

1. 接收 SMTP 连接。
2. 验证目标域名是否由本服务器管理。
3. 检查收件人邮箱是否存在。
4. 检查邮件大小。
5. 接收原始 MIME 数据。
6. 保存原始邮件。
7. 解析邮件头。
8. 解析邮件正文。
9. 解析附件。
10. 保存邮件索引。
11. 生成邮件接收事件。
12. 通过 Webhook 通知 Spring Boot。

注意：

SMTP 返回成功之前，应确保邮件已经可靠持久化，不能只保存在内存中。

## 3.5 邮件发送

Go 服务器负责真正向目标邮件服务器发送邮件。

支持：

- MX DNS 查询。
- SMTP 连接。
- TLS。
- 多收件人。
- 邮件附件。
- DKIM 签名。
- 发送失败重试。
- 退信处理。
- SMTP 响应记录。
- 投递状态记录。

发送流程：

```text
Spring Boot
     |
     v
Go Mail API
     |
     v
创建邮件任务
     |
     v
持久化邮件队列
     |
     v
MX DNS 查询
     |
     v
SMTP 连接
     |
     v
邮件投递
     |
     v
更新投递状态
     |
     v
Webhook 通知
```

必须区分：

- 已提交：邮件已进入本地队列。
- 已发送：远端 SMTP 服务器接受了邮件。
- 已送达：有可靠的投递确认依据。
- 发送失败：投递失败。
- 已退信：收到退信通知。

不能将远端 SMTP 返回成功直接认定为最终进入对方收件箱。

## 3.6 邮件解析

解析并保存以下内容：

```json
{
  "messageId": "邮件唯一ID",
  "from": "sender@example.com",
  "to": [
    "admin@example.com"
  ],
  "cc": [],
  "bcc": [],
  "subject": "测试邮件",
  "text": "纯文本内容",
  "html": "<h1>Hello</h1>",
  "headers": {},
  "attachments": [],
  "receivedAt": "2026-11-24T14:00:00+08:00"
}
```

支持：

- MIME multipart。
- text/plain。
- text/html。
- multipart/alternative。
- multipart/mixed。
- Base64。
- Quoted-Printable。
- UTF-8。
- 常见邮件字符集。
- 内嵌图片。
- 普通附件。
- 邮件头解码。

必须防范 MIME 嵌套过深、超大附件、解压炸弹和恶意文件。

---

# 四、邮件存储设计

建议使用 MySQL 保存元数据，使用本地文件系统保存邮件原文和附件。

## 4.1 数据库

Go 端和 Spring Boot 端可以使用同一 MySQL 实例，但必须使用独立数据库和独立账号。

Go 数据库：

`chrelyonly_mail_server`

主要表：

- mail_domains
- mail_accounts
- mail_queue
- mail_messages
- mail_recipients
- mail_attachments
- mail_delivery_logs
- mail_events
- mail_auth_logs

Spring Boot 数据库：

`chrelyonly_mail_system`

主要表：

- sys_api_keys
- sys_webhooks
- sys_webhook_logs
- sys_mail_records
- sys_config

两端不得直接操作对方的业务表。

## 4.2 文件存储

建议目录：

```text
data/
├── messages/
│   ├── 2026/
│   │   ├── 11/
│   │   │   └── 原始邮件文件
│   │
├── attachments/
│   ├── 2026/
│   │   └── 邮件附件
│
├── queue/
├── dkim/
└── logs/
```

要求：

- 邮件原文与附件分开管理。
- 文件使用随机 ID 命名。
- 不允许直接使用用户上传文件名作为物理路径。
- 限制磁盘空间。
- 支持邮件数据备份。
- 支持邮件恢复。
- 防止路径穿越。
- 附件下载需要权限校验。

---

# 五、Go 内部 API

Go 邮件服务器需要提供一套内部 HTTP API，供 Spring Boot 调用。

建议默认监听：

`127.0.0.1:18080`

如果跨服务器部署，则使用 HTTPS 和强认证。

## 5.1 邮件发送

```http
POST /internal/api/v1/mail/send
```

请求：

```json
{
  "from": "admin@chrelyonly.cn",
  "to": ["user@example.com"],
  "cc": [],
  "bcc": [],
  "subject": "测试邮件",
  "text": "Hello",
  "html": "<h1>Hello World</h1>",
  "attachments": []
}
```

返回：

```json
{
  "success": true,
  "messageId": "mail_202611240001",
  "status": "queued"
}
```

## 5.2 查询邮件

```http
GET /internal/api/v1/mail/{messageId}
```

## 5.3 查询邮件列表

```http
GET /internal/api/v1/mail?account=admin@chrelyonly.cn&page=1&size=20
```

## 5.4 创建邮箱

```http
POST /internal/api/v1/accounts
```

## 5.5 删除邮箱

```http
DELETE /internal/api/v1/accounts/{id}
```

## 5.6 域名管理

```http
GET    /internal/api/v1/domains
POST   /internal/api/v1/domains
DELETE /internal/api/v1/domains/{id}
```

## 5.7 健康检查

```http
GET /internal/api/v1/health
```

返回：

```json
{
  "success": true,
  "status": "healthy",
  "smtp": true,
  "database": true,
  "storage": true
}
```

---

# 六、Spring Boot 管理 API

Spring Boot 作为整个系统唯一对外的业务 API 入口。

## 6.1 邮件接口

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/mail/send | 发送邮件 |
| GET | /api/mail/list | 邮件列表 |
| GET | /api/mail/detail/{id} | 邮件详情 |
| DELETE | /api/mail/delete/{id} | 删除邮件 |
| GET | /api/mail/attachment/{id} | 下载附件 |
| GET | /api/mail/statistics | 邮件统计 |

## 6.2 邮箱账户管理

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/mail/accounts | 邮箱列表 |
| POST | /api/mail/accounts | 创建邮箱 |
| PUT | /api/mail/accounts/{id} | 修改邮箱 |
| DELETE | /api/mail/accounts/{id} | 删除邮箱 |
| PUT | /api/mail/accounts/{id}/password | 修改密码 |

## 6.3 域名管理

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/mail/domains | 域名列表 |
| POST | /api/mail/domains | 添加域名 |
| GET | /api/mail/domains/{id} | 域名详情 |
| DELETE | /api/mail/domains/{id} | 删除域名 |
| GET | /api/mail/domains/{id}/dns | DNS 配置检查 |

## 6.4 API Key 管理

每个外部应用可以拥有独立 API Key。

要求：

- API Key 创建。
- API Key 禁用。
- API Key 删除。
- API Key 权限控制。
- API Key 使用日志。
- API Key 限流。
- API Key 过期时间。
- 只保存 API Key 的安全哈希，不保存明文。

---

# 七、Webhook 邮件事件系统

Go 邮件服务器需要向 Spring Boot 提供事件回调。

默认回调地址由 Spring Boot 配置。

## 7.1 事件类型

```text
mail.received
mail.queued
mail.sent
mail.failed
mail.bounced
mail.deleted
mail.read
```

注意：mail.read 需要有明确的读取行为，例如 IMAP 标记已读或 API 标记已读。

## 7.2 Webhook 示例

```http
POST /internal/mail/webhook
Content-Type: application/json
X-Mail-Event: mail.received
X-Mail-Event-Id: evt_202611240001
X-Mail-Timestamp: 1795500000
X-Mail-Signature: sha256=签名内容
```

请求：

```json
{
  "eventId": "evt_202611240001",
  "event": "mail.received",
  "timestamp": 1795500000,
  "data": {
    "messageId": "mail_202611240001",
    "from": "sender@example.com",
    "to": ["admin@chrelyonly.cn"],
    "subject": "测试邮件",
    "hasAttachments": true
  }
}
```

## 7.3 Webhook 安全机制

必须实现：

- HMAC-SHA256 签名。
- 时间戳验证。
- 事件 ID。
- 幂等处理。
- 失败重试。
- 指数退避。
- 最大重试次数。
- 失败日志。
- 手动重新投递。
- 事件持久化。

Webhook 不能因为 Spring Boot 暂时不可用而丢失。

建议重试间隔：

```text
1分钟
5分钟
15分钟
30分钟
1小时
2小时
```

超过最大次数后进入失败队列，等待管理员处理。

---

# 八、邮件安全要求

邮件服务器必须重视安全。

必须实现：

1. SMTP AUTH。
2. TLS 1.2 / TLS 1.3。
3. 防止开放中继。
4. 密码使用 Argon2id 或 bcrypt 哈希。
5. 邮件大小限制。
6. 附件大小限制。
7. SMTP 连接频率限制。
8. 登录失败限制。
9. IP 黑名单。
10. 邮件发送频率限制。
11. 防止目录穿越。
12. 防止邮件头注入。
13. 防止 CRLF 注入。
14. 防止未授权邮件读取。
15. Webhook 签名验证。
16. 内部 API 身份验证。
17. 日志中隐藏密码和敏感信息。

邮件认证能力：

- SPF 验证。
- DKIM 签名。
- DKIM 验证。
- DMARC 策略。
- PTR / 反向 DNS 检查。

注意：邮件认证和反垃圾邮件能力应逐步实现，不能将 SPF、DKIM、DMARC 简化成仅检查 DNS 记录。

---

# 九、Go 项目目录结构

建议：

```text
chrelyonly-mail-server/
│
├── cmd/
│   └── server/
│       └── main.go
│
├── internal/
│   ├── config/
│   ├── api/
│   ├── smtp/
│   ├── imap/
│   ├── delivery/
│   ├── queue/
│   ├── message/
│   ├── mime/
│   ├── account/
│   ├── domain/
│   ├── storage/
│   ├── webhook/
│   ├── auth/
│   ├── security/
│   └── logger/
│
├── pkg/
│   ├── response/
│   └── utils/
│
├── migrations/
│
├── configs/
│   └── config.example.yaml
│
├── scripts/
│
├── deployments/
│   └── Dockerfile
│
├── data/
│
├── logs/
│
├── go.mod
├── go.sum
└── README.md
```

要求：

- 不要为了架构而过度封装。
- 不要建立大量没有实际意义的接口。
- 业务代码集中、清晰。
- 不要在 main.go 中堆积所有业务。
- 所有配置从 YAML 或环境变量读取。
- 日志统一使用 Zap。
- 重要操作必须有错误处理。
- 所有数据库操作必须使用参数化查询。

---

# 十、Spring Boot 项目目录建议

```text
chrelyonly-mail-system/
│
├── src/main/java/
│   └── cn/chrelyonly/mail/
│       ├── controller/
│       ├── service/
│       ├── mapper/
│       ├── entity/
│       ├── dto/
│       ├── vo/
│       ├── client/
│       ├── webhook/
│       ├── config/
│       ├── security/
│       └── utils/
│
├── src/main/resources/
│   ├── mapper/
│   ├── application.yml
│   └── db/
│
├── pom.xml
└── README.md
```

Spring Boot 只负责业务，不重复实现 Go 已经提供的邮件协议功能。

---

# 十一、配置文件

Go 配置示例：

```yaml
server:
  name: chrelyonly-mail-server
  http_addr: 127.0.0.1:18080

smtp:
  hostname: mail.chrelyonly.cn
  ports:
    smtp: 25
    smtps: 465
    submission: 587
  tls:
    enabled: true
    cert_file: /etc/mail/cert.pem
    key_file: /etc/mail/key.pem

database:
  driver: mysql
  dsn: user:password@tcp(127.0.0.1:3306)/chrelyonly_mail_server?parseTime=true

storage:
  message_path: ./data/messages
  attachment_path: ./data/attachments
  max_message_size: 26214400
  max_attachment_size: 20971520

webhook:
  enabled: true
  url: http://127.0.0.1:8078/internal/mail/webhook
  secret: CHANGE_ME
  retry: true
  max_retries: 10

security:
  max_connections: 500
  max_auth_attempts: 5
```

以上为配置设计示例，实际运行时必须验证参数、限制文件权限，并将密钥放入安全配置中。

---

# 十二、部署要求

支持：

- Linux systemd。
- Docker。
- Docker Compose。
- 独立二进制部署。

建议部署结构：

```text
服务器
│
├── chrelyonly-mail-server
│   ├── SMTP :25
│   ├── SMTPS :465
│   ├── Submission :587
│   ├── Internal API :18080
│   └── Data Storage
│
├── chrelyonly-mail-system
│   └── REST API :8078
│
└── MySQL
```

生产环境中内部 API 默认仅允许本机访问；如果两个服务不在同一服务器，必须使用加密通信、身份认证和网络访问控制。

---

# 十三、开发阶段规划

必须按照阶段开发，禁止一次性生成大量不完整代码。

## 第一阶段：基础服务

- 初始化 Go 项目。
- 完成配置管理。
- 完成日志系统。
- 完成 MySQL 连接。
- 完成数据库迁移。
- 完成内部 HTTP API。
- 完成健康检查。

验收：Go 服务可以独立启动，能够连接数据库。

## 第二阶段：SMTP 接收

- SMTP 服务端。
- 域名验证。
- 邮箱账户验证。
- SMTP 接收。
- MIME 解析。
- 邮件持久化。
- 附件存储。

验收：可以通过外部 SMTP 客户端向服务器投递邮件，并保存完整邮件。

## 第三阶段：SMTP 发送

- SMTP 客户端。
- MX 查询。
- TLS。
- DKIM 签名。
- 邮件队列。
- 投递重试。
- 投递状态。

验收：可以通过 API 发送真实邮件到外部邮箱。

## 第四阶段：Spring Boot 集成

- 创建 Go 内部 API 客户端。
- 邮件发送接口。
- 邮箱管理接口。
- 域名管理接口。
- 邮件列表接口。
- 邮件详情接口。
- 附件下载接口。

验收：Spring Boot 可以完整控制 Go 邮件服务。

## 第五阶段：Webhook

- 邮件接收事件。
- 邮件发送事件。
- 邮件失败事件。
- HMAC 签名。
- 重试机制。
- 幂等机制。
- 失败事件管理。

验收：邮件收到后 Spring Boot 能够收到通知，服务暂时不可用时也不会丢失事件。

## 第六阶段：安全和生产部署

- SPF。
- DKIM。
- DMARC。
- TLS。
- 反垃圾邮件。
- 连接限制。
- Docker 部署。
- systemd 部署。
- 监控。
- 数据备份。

验收：具备正式部署所需的基本安全和运维能力。

## 第七阶段：IMAP

- IMAP 服务端。
- 收件箱。
- 已读状态。
- 邮件移动。
- 邮件删除。
- 文件夹管理。

验收：支持 Thunderbird、Outlook 等常见邮件客户端连接。

---

# 十四、AI 开发规则

AI 编程助手必须严格遵循以下规则：

1. 必须使用 Go 开发邮件核心服务。
2. 必须使用 Java Spring Boot 开发管理 API。
3. 两个项目必须独立运行。
4. 不允许将 Go 邮件核心改成 Java。
5. 不允许将 Spring Boot 管理 API 改成 Go。
6. 不允许使用 Postfix、Dovecot 代替本项目核心。
7. 可以使用成熟协议库，但必须保证代码可编译。
8. 不允许虚构第三方库 API。
9. 不允许省略核心业务代码。
10. 不允许只给伪代码。
11. 所有代码必须完整。
12. 所有配置必须提供示例。
13. 所有数据库表必须提供建表迁移。
14. 所有 API 必须提供请求和响应示例。
15. 必须处理异常情况。
16. 必须提供必要的中文注释。
17. 不允许随意更改已有接口和字段。
18. 不允许为了所谓架构设计而引入过度复杂的框架。
19. 每个阶段完成后必须提供编译和测试方法。
20. 不允许在未实现的功能上返回虚假的成功状态。

如果代码量较大，必须分阶段输出文件，不能为了缩短回答而省略关键实现。

---

# 十五、最终目标

最终实现一套属于自己的邮件基础设施。

可以通过 Spring Boot 提供的 REST API：

- 创建邮箱。
- 删除邮箱。
- 管理邮件域名。
- 发送邮件。
- 接收邮件。
- 查询收件箱。
- 查询邮件详情。
- 下载附件。
- 查询发送状态。
- 获取邮件事件。
- 配置 Webhook。
- 监控邮件服务器。

外部业务系统只需要调用 Spring Boot API，不需要理解 SMTP、IMAP 等底层协议。

Go 邮件服务器独立运行，Spring Boot 作为业务管理中心。

**项目最终名称：Chrelyonly Mail 🍓**