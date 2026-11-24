-- Chrelyonly Mail Server 数据库初始化脚本
-- 数据库: chrelyonly_mail_server

CREATE DATABASE IF NOT EXISTS `chrelyonly_mail_server` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE `chrelyonly_mail_server`;

-- 1. 托管域名表
CREATE TABLE IF NOT EXISTS `mail_domains` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `domain` VARCHAR(255) NOT NULL UNIQUE COMMENT '域名, 如 chrelyonly.cn',
    `is_active` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用',
    `dkim_selector` VARCHAR(64) DEFAULT 'default' COMMENT 'DKIM 选择器',
    `dkim_private_key` TEXT DEFAULT NULL COMMENT 'DKIM 私钥 (PEM 格式)',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_domain` (`domain`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='托管域名表';

-- 2. 邮箱账户表
CREATE TABLE IF NOT EXISTS `mail_accounts` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `email` VARCHAR(255) NOT NULL UNIQUE COMMENT '完整邮箱地址, 如 admin@chrelyonly.cn',
    `password_hash` VARCHAR(255) NOT NULL COMMENT '密码哈希 (Bcrypt)',
    `domain_id` BIGINT NOT NULL COMMENT '关联域名 ID',
    `quota_bytes` BIGINT NOT NULL DEFAULT 10737418240 COMMENT '容量限制 (字节, 默认10GB)',
    `used_bytes` BIGINT NOT NULL DEFAULT 0 COMMENT '已用容量',
    `is_active` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '账户状态 1-启用 0-禁用',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (`domain_id`) REFERENCES `mail_domains`(`id`) ON DELETE CASCADE,
    INDEX `idx_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='邮箱账户表';

-- 3. 邮件表
CREATE TABLE IF NOT EXISTS `mail_messages` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `message_id` VARCHAR(255) NOT NULL UNIQUE COMMENT '邮件标准 Message-ID / 本地唯一标识',
    `account_id` BIGINT DEFAULT NULL COMMENT '所属本地账户 ID',
    `from_address` VARCHAR(255) NOT NULL COMMENT '发件人地址',
    `subject` TEXT COMMENT '邮件主题',
    `text_content` LONGTEXT COMMENT '纯文本内容',
    `html_content` LONGTEXT COMMENT 'HTML 内容',
    `raw_file_path` VARCHAR(512) NOT NULL COMMENT '原始 EML 文件相对路径',
    `size_bytes` BIGINT NOT NULL DEFAULT 0 COMMENT '邮件大小 (字节)',
    `has_attachments` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否有附件',
    `is_read` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否已读',
    `direction` VARCHAR(16) NOT NULL DEFAULT 'inbound' COMMENT '方向: inbound(接收) / outbound(发送)',
    `status` VARCHAR(32) NOT NULL DEFAULT 'received' COMMENT '状态: received / queued / sent / failed / bounced',
    `received_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '接收或创建时间',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_account_id` (`account_id`),
    INDEX `idx_from` (`from_address`),
    INDEX `idx_received_at` (`received_at`),
    INDEX `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='邮件主表';

-- 4. 邮件收件人关联表
CREATE TABLE IF NOT EXISTS `mail_recipients` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `message_id` BIGINT NOT NULL COMMENT '邮件 ID (mail_messages.id)',
    `recipient_type` VARCHAR(8) NOT NULL COMMENT '收件类型: to / cc / bcc',
    `email_address` VARCHAR(255) NOT NULL COMMENT '收件人邮箱',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (`message_id`) REFERENCES `mail_messages`(`id`) ON DELETE CASCADE,
    INDEX `idx_msg_recipient` (`message_id`, `recipient_type`),
    INDEX `idx_email` (`email_address`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='邮件收件人明细表';

-- 5. 邮件附件表
CREATE TABLE IF NOT EXISTS `mail_attachments` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `message_id` BIGINT NOT NULL COMMENT '邮件 ID (mail_messages.id)',
    `filename` VARCHAR(255) NOT NULL COMMENT '附件原始文件名',
    `content_type` VARCHAR(128) NOT NULL DEFAULT 'application/octet-stream' COMMENT 'MIME 类型',
    `size_bytes` BIGINT NOT NULL DEFAULT 0 COMMENT '文件大小 (字节)',
    `file_path` VARCHAR(512) NOT NULL COMMENT '磁盘存储相对路径',
    `content_id` VARCHAR(255) DEFAULT NULL COMMENT 'Content-ID (内嵌图片使用)',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (`message_id`) REFERENCES `mail_messages`(`id`) ON DELETE CASCADE,
    INDEX `idx_message_id` (`message_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='邮件附件表';

-- 6. 邮件发送队列表
CREATE TABLE IF NOT EXISTS `mail_queue` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `message_id` VARCHAR(255) NOT NULL COMMENT '关联邮件 Message-ID',
    `from_address` VARCHAR(255) NOT NULL COMMENT '发件人',
    `to_address` VARCHAR(255) NOT NULL COMMENT '目标收件人',
    `status` VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT '状态: pending / processing / sent / failed',
    `retry_count` INT NOT NULL DEFAULT 0 COMMENT '已重试次数',
    `max_retries` INT NOT NULL DEFAULT 10 COMMENT '最大重试次数',
    `next_retry_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '下一次尝试时间',
    `last_error` TEXT DEFAULT NULL COMMENT '最后一次错误信息',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_queue_status_retry` (`status`, `next_retry_at`),
    INDEX `idx_message_id` (`message_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='邮件发送投递队列表';

-- 7. 投递日志表
CREATE TABLE IF NOT EXISTS `mail_delivery_logs` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `queue_id` BIGINT DEFAULT NULL COMMENT '队列任务 ID',
    `message_id` VARCHAR(255) NOT NULL COMMENT '邮件 Message-ID',
    `status` VARCHAR(32) NOT NULL COMMENT '投递状态: success / failed / retrying',
    `mx_host` VARCHAR(255) DEFAULT NULL COMMENT '目标 MX 服务器地址',
    `smtp_code` INT DEFAULT 0 COMMENT 'SMTP 状态码',
    `response` TEXT DEFAULT NULL COMMENT 'SMTP 服务器响应字符串',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_msg_id` (`message_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='邮件投递日志表';

-- 8. Webhook 事件通知表
CREATE TABLE IF NOT EXISTS `mail_events` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `event_id` VARCHAR(128) NOT NULL UNIQUE COMMENT '事件全局唯一 ID',
    `event_type` VARCHAR(64) NOT NULL COMMENT '事件类型, 如 mail.received, mail.sent',
    `message_id` VARCHAR(255) NOT NULL COMMENT '邮件 Message-ID',
    `payload` LONGTEXT NOT NULL COMMENT 'JSON 负载',
    `status` VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT '推送状态: pending / processing / sent / failed',
    `retry_count` INT NOT NULL DEFAULT 0 COMMENT '重试次数',
    `max_retries` INT NOT NULL DEFAULT 10 COMMENT '最大重试次数',
    `next_retry_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '下一次推送时间',
    `last_error` TEXT DEFAULT NULL COMMENT '最后推报错信息',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_event_status_retry` (`status`, `next_retry_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Webhook 事件队列表';

-- 9. 认证日志表
CREATE TABLE IF NOT EXISTS `mail_auth_logs` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `username` VARCHAR(255) NOT NULL COMMENT '尝试登录的用户名',
    `ip_address` VARCHAR(64) NOT NULL COMMENT '客户端 IP',
    `success` TINYINT(1) NOT NULL COMMENT '是否成功',
    `reason` VARCHAR(255) DEFAULT NULL COMMENT '失败原因',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_auth_time` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='SMTP/IMAP 认证日志表';
