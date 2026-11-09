package message

import "encoding/json"

// GroupFanoutTask 群消息下行内存并发扇出任务（Go 原生 Fan-out / Fan-in 载体）
type GroupFanoutTask struct {
	GroupId     string          `json:"group_id"`
	MessageUuid string          `json:"message_uuid"`
	MemberIds   []string        `json:"member_ids,omitempty"`
	Payload     json.RawMessage `json:"payload"`
}
