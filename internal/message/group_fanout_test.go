package message

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"

	msgtype "kama_chat_server/pkg/enum/message/message_type"
)

// TestMessageModel_UkSessionSeqCompositeUniqueIndex 校验 (session_id, seq_id) 是否正确配置为复合唯一索引 uk_session_seq
func TestMessageModel_UkSessionSeqCompositeUniqueIndex(t *testing.T) {
	s, err := schema.Parse(&Message{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)

	// 1. 确保旧的普通非唯一索引 idx_session_seq 已不存在
	oldIdx := s.LookIndex("idx_session_seq")
	assert.Nil(t, oldIdx, "idx_session_seq 应已被升级移除，不应存在")

	// 2. 确保复合唯一索引 uk_session_seq 存在并配置正确
	ukIdx := s.LookIndex("uk_session_seq")
	require.NotNil(t, ukIdx, "uk_session_seq 复合唯一索引必须存在")
	assert.Equal(t, "UNIQUE", ukIdx.Class, "uk_session_seq 必须是唯一索引 (UNIQUE)")
	require.Len(t, ukIdx.Fields, 2, "uk_session_seq 必须包含 2 个字段 (session_id, seq_id)")

	assert.Equal(t, "session_id", ukIdx.Fields[0].Field.DBName)
	assert.Equal(t, "seq_id", ukIdx.Fields[1].Field.DBName)
}

// TestDispatchToGroup_GoNativeFanout 校验群聊消息下行直接通过 Go 原生并发 Fan-out 执行
func TestDispatchToGroup_GoNativeFanout(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	groupId := "G3001"
	memberCacheKey := "group:member_ids:" + groupId
	memberBytes, _ := json.Marshal([]string{"U1001", "U1002", "U1003"})
	_ = cache.Set(context.Background(), memberCacheKey, string(memberBytes), 5*time.Minute)

	msg := Message{
		Uuid:        "M1234567890",
		ClientMsgId: "c_group_01",
		SessionId:   groupId,
		SendId:      "U1001",
		ReceiveId:   groupId,
		Type:        int8(msgtype.Text),
		Content:     "hello group",
		SeqId:       42,
		CreatedAt:   time.Now(),
	}

	assert.NotPanics(t, func() {
		svc.dispatchToGroup(context.Background(), msg, "SenderNick", "https://avatar.com/1.png")
	})
}

// TestExecuteGroupFanout_EmptyMembersDoesNotPanic 校验扇出执行时成员为空的安全处理
func TestExecuteGroupFanout_EmptyMembersDoesNotPanic(t *testing.T) {
	msgStore := &mockMsgStore{}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	svc := NewMessageService(msgStore, sessStore, cache)

	groupId := "G3003"
	_ = cache.Set(context.Background(), "group:member_ids:"+groupId, "[]", 5*time.Minute)

	task := &GroupFanoutTask{
		GroupId:     groupId,
		MessageUuid: "M1111111111",
		MemberIds:   []string{},
		Payload:     json.RawMessage(`{"content":"test"}`),
	}

	err := svc.ExecuteGroupFanout(context.Background(), task)
	assert.NoError(t, err)
}

// TestGroupFanoutTask_JsonRoundTrip 校验 GroupFanoutTask 序列化与反序列化
func TestGroupFanoutTask_JsonRoundTrip(t *testing.T) {
	original := GroupFanoutTask{
		GroupId:     "G5555",
		MessageUuid: "M5555555555",
		MemberIds:   []string{"U1", "U2", "U3"},
		Payload:     json.RawMessage(`{"send_id":"U1","content":"ping"}`),
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored GroupFanoutTask
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Equal(t, original.GroupId, restored.GroupId)
	assert.Equal(t, original.MessageUuid, restored.MessageUuid)
	assert.Equal(t, original.MemberIds, restored.MemberIds)
	assert.JSONEq(t, string(original.Payload), string(restored.Payload))
}

// TestExecuteGroupFanout_NilTask 校验空指针任务安全性
func TestExecuteGroupFanout_NilTask(t *testing.T) {
	svc := NewMessageService(&mockMsgStore{}, &mockSessStore{}, newMockCache())
	assert.NotPanics(t, func() {
		err := svc.ExecuteGroupFanout(context.Background(), nil)
		assert.NoError(t, err)
	})
}
