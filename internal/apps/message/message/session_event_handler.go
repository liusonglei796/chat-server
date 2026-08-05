package message

import (
	"context"
	"encoding/json"
	"strings"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
)

// SessionEventHandler 将跨服务领域事件转换为本地 session 表操作
type SessionEventHandler struct {
	sessionStore store.SessionStore
}

// NewSessionEventHandler 创建会话事件处理器
func NewSessionEventHandler(sessionStore store.SessionStore) *SessionEventHandler {
	return &SessionEventHandler{sessionStore: sessionStore}
}

// Handle 按事件类型分发处理
func (h *SessionEventHandler) Handle(ctx context.Context, eventType string, payload []byte) error {
	switch eventType {
	case event.EventGroupCreated:
		var e event.GroupCreatedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		return h.createGroupSession(ctx, e.GroupId, e.OwnerId, e.GroupName, e.GroupAvatar)
	case event.EventGroupJoined:
		var e event.GroupJoinedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		return h.createGroupSession(ctx, e.GroupId, e.UserId, e.GroupName, e.GroupAvatar)
	case event.EventGroupDismissed:
		var e event.GroupDismissedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		return h.sessionStore.SoftDeleteByUsers(ctx, []string{e.GroupId})
	case event.EventGroupUpdated:
		var e event.GroupUpdatedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		updates := make(map[string]interface{})
		if e.Name != nil {
			updates["receive_name"] = *e.Name
		}
		if e.Avatar != nil {
			updates["avatar"] = *e.Avatar
		}
		if len(updates) == 0 {
			return nil
		}
		return h.sessionStore.UpdateByReceiveId(ctx, e.GroupId, updates)
	case event.EventUserUpdated:
		var e event.UserUpdatedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		updates := make(map[string]interface{})
		if e.Nickname != nil {
			updates["receive_name"] = *e.Nickname
		}
		if e.Avatar != nil {
			updates["avatar"] = *e.Avatar
		}
		if len(updates) == 0 {
			return nil
		}
		return h.sessionStore.UpdateByReceiveId(ctx, e.UserId, updates)
	case event.EventFriendBlacked:
		var e event.FriendBlackedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		return h.softDeleteFriendSessions(ctx, e.UserId, e.FriendId)
	}
	return nil
}

// softDeleteFriendSessions 拉黑好友时软删双方私聊会话（幂等：不存在则跳过）
func (h *SessionEventHandler) softDeleteFriendSessions(ctx context.Context, userId, friendId string) error {
	uuids := make([]string, 0, 2)
	if sess, err := h.sessionStore.FindBySendIdAndReceiveId(ctx, userId, friendId); err == nil && sess != nil {
		uuids = append(uuids, sess.Uuid)
	}
	if sess, err := h.sessionStore.FindBySendIdAndReceiveId(ctx, friendId, userId); err == nil && sess != nil {
		uuids = append(uuids, sess.Uuid)
	}
	if len(uuids) == 0 {
		return nil
	}
	return h.sessionStore.SoftDeleteByUuids(ctx, uuids)
}

// createGroupSession 建群/入群时创建群会话（幂等：已存在则跳过）
func (h *SessionEventHandler) createGroupSession(ctx context.Context, groupId, userId, groupName, groupAvatar string) error {
	existing, err := h.sessionStore.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err == nil && existing != nil {
		return nil
	}
	if !strings.HasPrefix(groupId, "G") {
		return nil
	}
	sess := model.Session{
		Uuid:        "S" + snowflake.GenerateIDString(),
		SendId:      userId,
		ReceiveId:   groupId,
		ReceiveName: groupName,
		Avatar:      groupAvatar,
	}
	return h.sessionStore.CreateSession(ctx, &sess)
}
