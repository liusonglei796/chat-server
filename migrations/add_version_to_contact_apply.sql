-- 为 contact_apply 表增加 version 乐观锁版本号
-- 解决好友/群申请被拒绝后重新申请时复用记录造成的时序倒退与迟到审批穿透

USE chat_apply;

ALTER TABLE contact_apply
ADD COLUMN version INT NOT NULL DEFAULT 1 COMMENT '乐观锁版本号';
