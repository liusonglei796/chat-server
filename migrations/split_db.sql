-- 拆分：user 库
CREATE DATABASE IF NOT EXISTS chat_user CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
-- 拆分：relation 库
CREATE DATABASE IF NOT EXISTS chat_relation CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
-- 拆分：message 库
CREATE DATABASE IF NOT EXISTS chat_message CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 服务账号
CREATE USER IF NOT EXISTS 'svc_user'@'%' IDENTIFIED BY 'svc_user_pwd';
CREATE USER IF NOT EXISTS 'svc_relation'@'%' IDENTIFIED BY 'svc_relation_pwd';
CREATE USER IF NOT EXISTS 'svc_message'@'%' IDENTIFIED BY 'svc_message_pwd';

-- 最小权限原则：各服务只能访问自己的库
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP ON chat_user.* TO 'svc_user'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP ON chat_relation.* TO 'svc_relation'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP ON chat_message.* TO 'svc_message'@'%';

FLUSH PRIVILEGES;
