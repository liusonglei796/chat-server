-- 为 message 表增加 client_msg_id 字段和唯一索引
-- 用于 MySQL 层面的幂等兜底，防止 Redis 宕机时重复消息压垮数据库

-- 1. 增加 client_msg_id 字段（允许为空，兼容旧客户端）
ALTER TABLE message 
ADD COLUMN client_msg_id VARCHAR(64) DEFAULT NULL COMMENT '客户端消息唯一标识（幂等去重）';

-- 2. 增加唯一索引（使用 UNIQUE 而非普通 INDEX）
-- 注意：如果表中已有重复的 client_msg_id 数据，此语句会失败
-- 执行前请确认数据干净，或先用 DELETE 清理重复数据
ALTER TABLE message 
ADD UNIQUE INDEX uk_client_msg_id (client_msg_id);

-- 可选：如果历史数据中存在重复，可以先清理：
-- DELETE m1 FROM message m1
-- INNER JOIN (
--     SELECT client_msg_id, MIN(id) as min_id
--     FROM message
--     WHERE client_msg_id IS NOT NULL
--     GROUP BY client_msg_id
--     HAVING COUNT(*) > 1
-- ) m2 ON m1.client_msg_id = m2.client_msg_id AND m1.id > m2.min_id;
