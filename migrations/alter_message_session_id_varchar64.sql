-- 修改 message 表的 session_id 字段长度为 VARCHAR(64)
-- 支持单聊统合会话标识 P_{min(u1, u2)}_{max(u1, u2)} 与群聊统一 session_id
-- 数据库: chat_message

USE chat_message;

ALTER TABLE message MODIFY COLUMN session_id VARCHAR(64) NOT NULL COMMENT '会话uuid';
