-- 从 message 表中删除未使用的字段 status 和 send_at
-- 数据库: chat_message

USE chat_message;

ALTER TABLE message
  DROP COLUMN status,
  DROP COLUMN send_at;
