package message

import (
	"context"
	"encoding/json"
	"strings"

	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/dto/event"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
)

// SessionEventHandler 将跨服务领域事件转换为本地 session 表操作
type SessionEventHandler struct {
	sessionRepo repository.SessionRepository
}

// NewSessionEventHandler 创建会话事件处理器
func NewSessionEventHandler(sessionRepo repository.SessionRepository) *SessionEventHandler {
	return &SessionEventHandler{sessionRepo: sessionRepo}
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
		return h.sessionRepo.SoftDeleteByUsers(ctx, []string{e.GroupId})
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
		return h.sessionRepo.UpdateByReceiveId(ctx, e.GroupId, updates)
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
		return h.sessionRepo.UpdateByReceiveId(ctx, e.UserId, updates)
	}
	return nil
}

// createGroupSession 建群/入群时创建群会话（幂等：已存在则跳过）
func (h *SessionEventHandler) createGroupSession(ctx context.Context, groupId, userId, groupName, groupAvatar string) error {
	existing, err := h.sessionRepo.FindBySendIdAndReceiveId(ctx, userId, groupId)
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
	return h.sessionRepo.CreateSession(ctx, &sess)
}
