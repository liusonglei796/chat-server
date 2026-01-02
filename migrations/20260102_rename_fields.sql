-- 迁移脚本: 重命名字段和索引
-- 执行日期: 2026-01-02
-- 执行前请备份数据库！

-- ============================================
-- 1. user_contact 表: contacted_id → contact_id
-- (如果字段存在)
-- ============================================
-- ALTER TABLE user_contact CHANGE COLUMN contacted_id contact_id CHAR(20) NOT NULL COMMENT '联系人ID';

-- ============================================
-- 2. contact_apply 表: 
--    user_id → applicant_id
--    contacted_id → target_id
-- ============================================
ALTER TABLE contact_apply CHANGE COLUMN user_id applicant_id CHAR(20) NOT NULL COMMENT '申请人ID';
ALTER TABLE contact_apply CHANGE COLUMN contact_id target_id CHAR(20) NOT NULL COMMENT '目标ID(用户/群组)';

-- ============================================
-- 3. 更新索引名称
-- ============================================
ALTER TABLE contact_apply DROP INDEX idx_contact_apply_user_id;
ALTER TABLE contact_apply ADD INDEX idx_contact_apply_applicant_id (applicant_id);

ALTER TABLE contact_apply DROP INDEX idx_contact_apply_contact_id;
ALTER TABLE contact_apply ADD INDEX idx_contact_apply_target_id (target_id);
