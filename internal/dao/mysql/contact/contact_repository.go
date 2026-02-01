// Package contact 提供联系关系数据访问层的具体实现
//
// Contact 表用于存储用户的联系关系，包括两种类型：
//   - 好友关系 (contact_type=0): 存储用户之间的好友关系，双向存储
//     例如：A 和 B 是好友，则有两条记录 (A→B) 和 (B→A)
//   - 群聊关系 (contact_type=1): 存储用户加入的群聊
//     例如：A 加入了群1，则有一条记录 (A→群1)
//
// 本文件实现 ContactRepository 接口，处理联系关系相关的数据库操作
package contact

import (
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// contactRepository ContactRepository 接口的实现
type contactRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewContactRepository 创建 ContactRepository 实例
func NewContactRepository(db *gorm.DB) *contactRepository {
	return &contactRepository{db: db}
}

// FindByUserIdAndContactId 根据用户ID和联系人ID查找单条关系记录
//
// 参数:
//   - userId: 用户ID（记录的主体）
//   - contactId: 联系人ID（好友的用户ID 或 群聊ID）
//   - contactType: 联系类型，0=好友关系，1=群聊关系
//
// 返回:
//   - 找到则返回 Contact 记录，未找到或出错返回 error
func (r *contactRepository) FindByUserIdAndContactId(userId, contactId string, contactType int8) (*model.Contact, error) {
	var contact model.Contact
	if err := r.db.Where("user_id = ? AND contact_id = ? AND contact_type = ?", userId, contactId, contactType).First(&contact).Error; err != nil {
		return nil, errorx.WrapDBErrorf(err, "查询联系人 user_id=%s contact_id=%s type=%d", userId, contactId, contactType)
	}
	return &contact, nil
}

// IsFriend 判断两个用户是否互为好友
//
// 好友关系是双向的，需要同时存在两条记录且状态都为正常(status=0)：
//   - user1 → user2 (A 的好友列表里有 B)
//   - user2 → user1 (B 的好友列表里有 A)
//
// 返回:
//   - true: 双方互为好友
//   - false: 不是好友（单向添加或都没添加）
func (r *contactRepository) IsFriend(userId1, userId2 string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.Contact{}).
		Where("(user_id = ? AND contact_id = ? AND contact_type = 0) OR (user_id = ? AND contact_id = ? AND contact_type = 0)",
			userId1, userId2, userId2, userId1).
		Where("status = ?", 0).
		Count(&count).Error; err != nil {
		return false, errorx.WrapDBError(err, "check friend relationship failed")
	}
	// 必须存在两条记录（双向关系）才算是好友
	return count == 2, nil
}

// FindByUserIdAndType 分页查询用户的联系列表
//
// 参数:
//   - userId: 用户ID
//   - contactType: 联系类型，0=查询好友列表，1=查询加入的群聊列表
//   - page: 页码（从1开始）
//   - pageSize: 每页数量
//
// 返回:
//   - []model.Contact: 联系列表
//   - int64: 总记录数（用于分页）
func (r *contactRepository) FindByUserIdAndType(userId string, contactType int8, page, pageSize int) ([]model.Contact, int64, error) {
	var contacts []model.Contact
	var total int64

	query := r.db.Model(&model.Contact{}).
		Where("user_id = ? AND contact_type = ?", userId, contactType)

	// 查询总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "查询联系人总数 user_id=%s type=%d", userId, contactType)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&contacts).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "查询联系人列表 user_id=%s type=%d", userId, contactType)
	}

	return contacts, total, nil
}

// FindUsersByContactId 反向查询：查找有哪些用户添加了指定的联系人
//
// 使用场景:
//   - 查找某个用户被哪些人添加为好友
//   - 查找某个群聊有哪些成员（配合 member 表更准确）
//
// 参数:
//   - contactId: 被查询的联系人ID（用户ID 或 群聊ID）
func (r *contactRepository) FindUsersByContactId(contactId string) ([]model.Contact, error) {
	var contacts []model.Contact
	if err := r.db.Where("contact_id = ?", contactId).Find(&contacts).Error; err != nil {
		return nil, errorx.WrapDBErrorf(err, "查询联系人 contact_id=%s", contactId)
	}
	return contacts, nil
}

// CreateContact 创建联系关系记录
//
// 使用场景:
//   - 添加好友时，创建双向记录（需调用两次或在 service 层处理）
//   - 加入群聊时，创建用户→群聊的记录
func (r *contactRepository) CreateContact(contact *model.Contact) error {
	if err := r.db.Create(contact).Error; err != nil {
		return errorx.WrapDBError(err, "创建联系人关系")
	}
	return nil
}

// UpdateStatus 更新联系关系的状态
//
// 参数:
//   - status: 状态值，参见 contact_status_enum
//   - 0: 正常
//   - 1: 拉黑
//   - 2: 被拉黑
//
// 注意: 此方法只更新单向记录，拉黑等双向操作需在 service 层处理
func (r *contactRepository) UpdateStatus(userId, contactId string, contactType int8, status int8) error {
	if err := r.db.Model(&model.Contact{}).
		Where("user_id = ? AND contact_id = ? AND contact_type = ?", userId, contactId, contactType).
		Update("status", status).Error; err != nil {
		return errorx.WrapDBErrorf(err, "更新联系人状态 user_id=%s contact_id=%s type=%d", userId, contactId, contactType)
	}
	return nil
}

// SoftDelete 软删除联系关系（双向删除）
//
// 使用 OR 条件同时删除双向关系记录：
//   - userId → contactId（我添加对方）
//   - contactId → userId（对方添加我）
//
// 使用场景:
//   - 删除好友：A 删除 B，同时移除 A→B 和 B→A 两条记录
//   - 退出群聊：用户退出群时，删除用户→群聊的记录（群聊场景通常只有单向记录）
func (r *contactRepository) SoftDelete(userId, contactId string, contactType int8) error {
	if err := r.db.Where(
		"(user_id = ? AND contact_id = ? AND contact_type = ?) OR (user_id = ? AND contact_id = ? AND contact_type = ?)",
		userId, contactId, contactType,
		contactId, userId, contactType,
	).Delete(&model.Contact{}).Error; err != nil {
		return errorx.WrapDBErrorf(err, "删除联系人关系 user_id=%s contact_id=%s type=%d", userId, contactId, contactType)
	}
	return nil
}

// SoftDeleteByUsers 批量软删除指定用户的所有联系关系
//
// 删除所有与指定用户相关的记录，包括：
//   - user_id 在列表中的记录（用户主动添加的好友/加入的群聊）
//   - contact_id 在列表中的记录（别人添加该用户为好友的记录）
//
// 使用场景:
//   - 用户注销账号时，清理所有与该用户相关的联系关系
//   - 批量清理多个用户的联系关系
//
// 示例：删除用户 A 的所有联系关系
//
//	SoftDeleteByUsers(["A"])
//	// 删除: A→B, A→C, A→群1 (A 的联系列表)
//	// 删除: B→A, C→A (别人的联系列表中关于 A 的记录)
func (r *contactRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.Where("user_id IN ? OR contact_id IN ?", userUuids, userUuids).Delete(&model.Contact{}).Error; err != nil {
		return errorx.WrapDBError(err, "批量删除联系人关系")
	}
	return nil
}
