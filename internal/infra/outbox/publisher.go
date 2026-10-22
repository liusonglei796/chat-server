package outbox

import (
	"github.com/twmb/franz-go/pkg/kgo"
)

// EventTypeHeader 领域事件类型在消息头中的 key
const EventTypeHeader = "event_type"

// ExtractEventType 从消息头中读取事件类型，供各服务领域事件消费者解析
func ExtractEventType(headers []kgo.RecordHeader) string {
	for _, h := range headers {
		if h.Key == EventTypeHeader {
			return string(h.Value)
		}
	}
	return ""
}
