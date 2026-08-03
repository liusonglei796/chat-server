-- 一次性迁移：旧数据在 chat 库；先由各服务 AutoMigrate 创建新库表结构，再搬数据
USE chat_user;
INSERT IGNORE INTO user_info SELECT * FROM chat.user_info;
INSERT IGNORE INTO outbox SELECT * FROM chat.outbox;

USE chat_relation;
INSERT IGNORE INTO group_info SELECT * FROM chat.group_info;
INSERT IGNORE INTO group_member SELECT * FROM chat.group_member;
INSERT IGNORE INTO user_friendship SELECT * FROM chat.user_friendship;
INSERT IGNORE INTO contact_apply SELECT * FROM chat.contact_apply;
INSERT IGNORE INTO outbox SELECT * FROM chat.outbox;

USE chat_message;
INSERT IGNORE INTO session SELECT * FROM chat.session;
INSERT IGNORE INTO message SELECT * FROM chat.message;
INSERT IGNORE INTO outbox SELECT * FROM chat.outbox;
