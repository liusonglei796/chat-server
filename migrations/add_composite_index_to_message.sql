-- 为 message 表增加联合索引，彻底解决聊天记录深翻页与 filesort 排序瓶颈
-- 数据库: chat_message

USE chat_message;

-- 1. 会话游标分页联合索引 (session_id, uuid DESC)
-- 适用: 按会话维度（单聊/群聊统合）拉取历史消息，消除 filesort，毫秒级快速定点下钻
ALTER TABLE message ADD INDEX idx_session_uuid (session_id, uuid DESC);

-- 2. 群聊接收者游标分页联合索引 (receive_id, uuid DESC)
-- 适用: 按群组维度拉取群聊天历史，消除 filesort
ALTER TABLE message ADD INDEX idx_receive_uuid (receive_id, uuid DESC);
