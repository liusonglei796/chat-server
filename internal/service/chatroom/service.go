package chatroom

import (
	"kama_chat_server/internal/dto/respond"
)

// chatRoomService 聊天室业务逻辑实现
// 此 service 不依赖 DAO，仅依赖内存数据结构
type chatRoomService struct{}

// NewChatRoomService 构造函数
func NewChatRoomService() *chatRoomService {
	return &chatRoomService{}
}

type chatRoomKey struct {
	userId    string
	contactId string
}

var chatRooms = make(map[chatRoomKey][]string)

// GetCurContactListInChatRoom 获取当前聊天室联系人列表
func (c *chatRoomService) GetCurContactListInChatRoom(userId string, contactId string) ([]respond.GetCurContactListInChatRoomRespond, error) {
	var rspList []respond.GetCurContactListInChatRoomRespond
	for _, cid := range chatRooms[chatRoomKey{userId, contactId}] {
		rspList = append(rspList, respond.GetCurContactListInChatRoomRespond{
			ContactId: cid,
		})
	}
	return rspList, nil
}
