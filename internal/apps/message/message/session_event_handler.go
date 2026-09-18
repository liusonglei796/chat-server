package message

import (
	"context"
	"encoding/json"
	"strings"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
)

// SessionEventHandler 将跨服务领域事件转换为本地 session 表操作及终端信令推送
type SessionEventHandler struct {
	sessionStore store.SessionStore
	cache        store.AsyncCacheService
}

// NewSessionEventHandler 创建会话事件处理器
func NewSessionEventHandler(sessionStore store.SessionStore, cache ...store.AsyncCacheService) *SessionEventHandler {
	var c store.AsyncCacheService
	if len(cache) > 0 {
		c = cache[0]
	}
	return &SessionEventHandler{sessionStore: sessionStore, cache: c}
}

func (h *SessionEventHandler) pushNotice(ctx context.Context, userId, prefix string, noticeData map[string]interface{}) {
	if h.cache == nil {
		return
	}
	gatewayAddr, err := h.cache.Get(ctx, constants.CacheKeyUserGateway+userId)
	if err != nil || gatewayAddr == "" {
		return
	}
	msgUuid := prefix + "_" + snowflake.GenerateIDString()
	noticeBytes, _ := json.Marshal(noticeData)
	_ = grpc_client.PushToGateway(ctx, gatewayAddr, userId, msgUuid, noticeBytes)
}

// Handle 按事件类型分发处理
func (h *SessionEventHandler) Handle(ctx context.Context, eventType string, payload []byte) error {
	switch eventType {
	case event.EventGroupCreated:
		var e event.GroupCreatedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		if err := h.createGroupSession(ctx, e.GroupId, e.OwnerId); err != nil {
			return err
		}
		h.pushNotice(ctx, e.OwnerId, "GRP_CREATE", map[string]interface{}{
			"type":       "system",
			"event":      "group_created",
			"group_id":   e.GroupId,
			"group_name": e.GroupName,
		})
		return nil

	case event.EventGroupJoined:
		var e event.GroupJoinedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		if err := h.createGroupSession(ctx, e.GroupId, e.UserId); err != nil {
			return err
		}
		h.pushNotice(ctx, e.UserId, "GRP_JOIN", map[string]interface{}{
			"type":       "system",
			"event":      "group_joined",
			"group_id":   e.GroupId,
			"group_name": e.GroupName,
		})
		return nil

	case event.EventGroupDismissed:
		var e event.GroupDismissedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		return h.sessionStore.SoftDeleteByUsers(ctx, []string{e.GroupId})

	case event.EventGroupUpdated:
		// session 表已移除冗余字段，无需同步
		return nil

	case event.EventUserUpdated:
		// session 表已移除冗余字段，无需同步
		return nil

	case event.EventFriendBlacked:
		var e event.FriendBlackedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		if err := h.softDeleteFriendSessions(ctx, e.UserId, e.FriendId); err != nil {
			return err
		}
		h.pushNotice(ctx, e.FriendId, "BLACK", map[string]interface{}{
			"type":      "system",
			"event":     "friend_blacked",
			"user_id":   e.UserId,
			"friend_id": e.FriendId,
		})
		return nil

	case event.EventGroupMemberRemoved:
		var e event.GroupMemberRemovedEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		if err := h.softDeleteGroupMemberSessions(ctx, e.GroupId, e.MemberUuids); err != nil {
			return err
		}
		for _, uid := range e.MemberUuids {
			h.pushNotice(ctx, uid, "KICK", map[string]interface{}{
				"type":     "system",
				"event":    "kicked_from_group",
				"group_id": e.GroupId,
				"content":  "你已被管理员移出该群",
			})
		}
		return nil

	case event.EventGroupMemberLeft:
		var e event.GroupMemberLeftEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return err
		}
		if err := h.softDeleteGroupMemberSessions(ctx, e.GroupId, []string{e.UserId}); err != nil {
			return err
		}
		h.pushNotice(ctx, e.UserId, "LEFT", map[string]interface{}{
			"type":     "system",
			"event":    "left_group",
			"group_id": e.GroupId,
			"content":  "你已退出该群",
		})
		return nil
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

// softDeleteGroupMemberSessions 成员退群或被移出时软删除对应的群会话
func (h *SessionEventHandler) softDeleteGroupMemberSessions(ctx context.Context, groupId string, userIds []string) error {
	if len(userIds) == 0 {
		return nil
	}
	uuids := make([]string, 0, len(userIds))
	for _, uid := range userIds {
		if sess, err := h.sessionStore.FindBySendIdAndReceiveId(ctx, uid, groupId); err == nil && sess != nil {
			uuids = append(uuids, sess.Uuid)
		}
	}
	if len(uuids) == 0 {
		return nil
	}
	return h.sessionStore.SoftDeleteByUuids(ctx, uuids)
}

// createGroupSession 建群/入群时创建群会话（幂等：已存在则跳过）
func (h *SessionEventHandler) createGroupSession(ctx context.Context, groupId, userId string) error {
	existing, err := h.sessionStore.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err == nil && existing != nil {
		return nil
	}
	if !strings.HasPrefix(groupId, "G") {
		return nil
	}
	sess := model.Session{
		Uuid:      "S" + snowflake.GenerateIDString(),
		SendId:    userId,
		ReceiveId: groupId,
	}
	return h.sessionStore.CreateSession(ctx, &sess)
}
