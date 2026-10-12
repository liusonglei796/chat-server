-- 为 group_info 表增加 (owner_id, name) 联合唯一索引
-- 用于 MySQL 层面的建群幂等兜底，防止 Redis 宕机或并发连击时创建出同名重复群聊

-- 增加联合唯一索引
ALTER TABLE group_info 
ADD UNIQUE INDEX idx_owner_name (owner_id, name);
