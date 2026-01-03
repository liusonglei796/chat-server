package chatroom

import (
	"kama_chat_server/internal/dao/mysql/repository"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/pkg/errorx"
)

// chatRoomService 聊天室业务逻辑实现
type chatRoomService struct {
	repos *repository.Repositories
}

// NewChatRoomService 构造函数
func NewChatRoomService(repos *repository.Repositories) *chatRoomService {
	return &chatRoomService{repos: repos}
}

// GetCurContactListInChatRoom 获取当前聊天室联系人具体的所有成员 ID 列表。
func (c *chatRoomService) GetCurContactListInChatRoom(userId string, contactId string) ([]respond.GetCurContactListInChatRoomRespond, error) {
	if len(contactId) == 0 {
		return nil, errorx.New(errorx.CodeInvalidParam, "contactId cannot be empty")
	}

	var memberIds []string
	var err error

	// 1. 判断聊天类型
	if contactId[0] == 'U' {
		// 私聊：成员就是通信双方
		// 注意：如果不验证好友关系，这里可能直接返回。严格来说应该验证，但私聊房间成员固定。
		memberIds = []string{userId, contactId}
	} else if contactId[0] == 'G' {
		// 群聊：优先查 Redis
		cacheKey := "group_member_ids_" + contactId
		memberIds, err = myredis.SMembers(cacheKey)

		// 如果缓存未命中或为空 (无成员群聊几乎不存在，即使有也应该查DB确认)
		if err != nil || len(memberIds) == 0 {
			// 回源查 DB
			// 注意：GetMemberIdsByGroupUuids 接受切片参数
			ids, dbErr := c.repos.GroupMember.GetMemberIdsByGroupUuids([]string{contactId})
			if dbErr != nil {
				return nil, dbErr
			}
			memberIds = ids

			// 写入缓存 (如果不为空)
			if len(memberIds) > 0 {
				// 转换 interface{} 切片以适配 SAdd
				membersArgs := make([]interface{}, len(memberIds))
				for i, v := range memberIds {
					membersArgs[i] = v
				}
				_ = myredis.SAdd(cacheKey, membersArgs...)
			}
		}
	} else {
		return nil, errorx.New(errorx.CodeInvalidParam, "invalid contactId prefix")
	}

	// 2. 构造响应
	rspList := make([]respond.GetCurContactListInChatRoomRespond, 0, len(memberIds))
	for _, cid := range memberIds {
		rspList = append(rspList, respond.GetCurContactListInChatRoomRespond{
			ContactId: cid,
		})
	}

	return rspList, nil
}
