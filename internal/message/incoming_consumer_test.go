package message

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	messagepb "kama_chat_server/api/gen/message"
	msgtype "kama_chat_server/pkg/enum/message/message_type"
)

// TestGetMessageListRespond_IncludesEchoAckMetadata 验证下行推送载荷包含方案二回声确认所需的 message_uuid 与 client_msg_id
func TestGetMessageListRespond_IncludesEchoAckMetadata(t *testing.T) {
	msg := Message{
		Uuid:        "M999888777",
		ClientMsgId: "c_echo_test_001",
		SessionId:   "P_U100_U200",
		SendId:      "U100",
		ReceiveId:   "U200",
		Type:        int8(msgtype.Text),
		Content:     "echo ack content",
		SeqId:       7,
		CreatedAt:   time.Now(),
	}

	rsp := GetMessageListRespond{
		MessageUuid: msg.Uuid,
		ClientMsgId: msg.ClientMsgId,
		SendId:      msg.SendId,
		ReceiveId:   msg.ReceiveId,
		Type:        msg.Type,
		Content:     msg.Content,
		SeqId:       msg.SeqId,
		CreatedAt:   msg.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	raw, err := json.Marshal(rsp)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, "M999888777", decoded["message_uuid"])
	assert.Equal(t, "c_echo_test_001", decoded["client_msg_id"])
	assert.Equal(t, float64(7), decoded["seq_id"])
}

// TestIncomingMessageConsumer_PoisonPill 验证非法 Protobuf 载荷不会阻塞消费分区
func TestIncomingMessageConsumer_PoisonPill(t *testing.T) {
	svc := NewMessageService(&mockMsgStore{}, &mockSessStore{}, nil)
	consumer := &IncomingMessageConsumer{
		service: svc,
		ctx:     context.Background(),
	}

	record := &kgo.Record{
		Key:   []byte("P_U1_U2"),
		Value: []byte("not-a-valid-protobuf-wire-payload-\xff\xff\xff"),
	}
	err := consumer.ProcessRecord(context.Background(), record)
	assert.NoError(t, err, "毒丸消息应静默跳过并返回 nil 以便提交 offset")
}

// TestIsNonRetryableBusinessError 验证确定性业务错误识别
func TestIsNonRetryableBusinessError(t *testing.T) {
	assert.True(t, isNonRetryableBusinessError(messagepb.ErrorForbidden("被对方拒收")))
	assert.True(t, isNonRetryableBusinessError(messagepb.ErrorInvalidParam("参数错误")))
	assert.False(t, isNonRetryableBusinessError(messagepb.ErrorServerBusy("服务繁忙")))
	assert.False(t, isNonRetryableBusinessError(nil))
}
