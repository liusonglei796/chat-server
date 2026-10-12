-- 为 user_friendship 表增加 version 乐观锁版本号
-- 解决好友拉黑/解拉黑可逆循环状态机中的 ABA 问题与迟到请求引起的会话级联软删除副作用

USE chat_friendship;

ALTER TABLE user_friendship
ADD COLUMN version BIGINT NOT NULL DEFAULT 1 COMMENT '乐观锁版本号';
