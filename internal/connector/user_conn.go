package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	messagepb "kama_chat_server/api/gen/message"
	grpcinfra "kama_chat_server/internal/infra/grpc_client"
	metricsinfra "kama_chat_server/internal/infra/metrics"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/interceptor"
)

const (
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	writeWait  = 10 * time.Second
)

var defaultPongBytes = []byte(`{"action":"pong"}`)

// isPingPayload 极速字节检测是否为客户端心跳 ping 指令（零反序列化、零堆分配）
func isPingPayload(data []byte) bool {
	if len(data) > 64 {
		return false
	}
	return bytes.Contains(data, []byte(`"action":"ping"`)) ||
		bytes.Contains(data, []byte(`"action": "ping"`)) ||
		bytes.Contains(data, []byte(`"action":"PING"`))
}

// UserConn 一个在线用户的 WebSocket 连接抽象 (基于 coder/websocket)
// 纯粹作为传输层通道抽象，不持有全局连接中枢 (ConnHub)，生命周期事件通过 OnClose 回调通知外部。
type UserConn struct {
	Conn        *websocket.Conn
	Uuid        string
	sendQueue   chan []byte
	OnClose     func(c *UserConn)
	cleanupOnce sync.Once

	ctx    context.Context
	cancel context.CancelFunc

	lastActive atomic.Int64
}

// SafeSend 向该用户的 WebSocket 下行队列写入消息并以非阻塞方式投递
// 具备应用层背压水线保护，防止慢客户端/弱网无限堆积内存
func (c *UserConn) SafeSend(payload []byte, msgUuid ...string) {
	if c.ctx != nil && c.ctx.Err() != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			zap.L().Warn("send to closed user conn recovered", zap.String("userId", c.Uuid))
		}
	}()

	// 1. 应用层背压限流缓冲保护
	select {
	case c.sendQueue <- payload:
	default:
		uuid := ""
		if len(msgUuid) > 0 {
			uuid = msgUuid[0]
		}
		zap.L().Warn("user conn send queue full (backpressure), dropping message",
			zap.String("userId", c.Uuid),
			zap.String("msgUuid", uuid),
		)
		return
	}

	// 2. 刷新最后活跃时间
	c.lastActive.Store(time.Now().Unix())
}

// writePump 专职单向异步写循环，从 sendQueue 消费并下发至底层 Socket
func (c *UserConn) writePump() {
	defer c.cleanup()
	for {
		select {
		case <-c.ctx.Done():
			return
		case payload, ok := <-c.sendQueue:
			if !ok {
				return
			}
			if c.Conn != nil {
				writeCtx, cancel := context.WithTimeout(c.ctx, writeWait)
				err := c.Conn.Write(writeCtx, websocket.MessageText, payload)
				cancel()
				if err != nil {
					zap.L().Warn("ws write error", zap.String("userId", c.Uuid), zap.Error(err))
					return
				}
			}
		}
	}
}

// WsFrame 统一的客户端上行消息帧
// 同时承载控制信令（action: "sync" / "ping"）与业务聊天消息（SendMessageRequest）
type WsFrame struct {
	// 1. 控制信令字段（无向后兼容，信令严格归于 action）
	Action    string `json:"action,omitempty"`
	TimeoutMs int64  `json:"timeout_ms,omitempty"`

	// 2. 增量对账同步专属字段 (action: "sync")
	TargetId string `json:"target_id,omitempty"`
	AfterSeq int64  `json:"after_seq,omitempty"`
	Limit    int32  `json:"limit,omitempty"`

	// 3. 业务聊天消息专属字段 (对齐 messagepb.SendMessageRequest)
	SessionId   string `json:"session_id,omitempty"`
	Type        int32  `json:"type,omitempty"` // 业务消息类型: 0 文本, 2 文件, 4 撤回
	Content     string `json:"content,omitempty"`
	Url         string `json:"url,omitempty"`
	SendName    string `json:"send_name,omitempty"`
	SendAvatar  string `json:"send_avatar,omitempty"`
	ReceiveId   string `json:"receive_id,omitempty"`
	FileSize    string `json:"file_size,omitempty"`
	FileType    string `json:"file_type,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	ClientMsgId string `json:"client_msg_id,omitempty"`
}

// IsAction 判定当前信令动作（严格比对 action 字段，拒绝任何 type 兼容）
func (f *WsFrame) IsAction(action string) bool {
	if f == nil {
		return false
	}
	return strings.EqualFold(f.Action, action)
}

// ToSendMessageRequest 将已单次反序列化的帧直接转换为 gRPC SendMessageRequest 请求结构体
func (f *WsFrame) ToSendMessageRequest(userId string) *messagepb.SendMessageRequest {
	return &messagepb.SendMessageRequest{
		SessionId:   f.SessionId,
		Type:        f.Type,
		Content:     f.Content,
		Url:         f.Url,
		SendId:      userId,
		SendName:    f.SendName,
		SendAvatar:  f.SendAvatar,
		ReceiveId:   f.ReceiveId,
		FileSize:    f.FileSize,
		FileType:    f.FileType,
		FileName:    f.FileName,
		ClientMsgId: f.ClientMsgId,
	}
}

// readPump 专职单向读循环，循环读取客户端消息与心跳
func (c *UserConn) readPump(hub *ConnHub) {
	defer c.cleanup()
	for {
		typ, data, err := c.Conn.Read(c.ctx)
		if err != nil {
			// 连接断开或被上层 cancel
			return
		}
		if typ != websocket.MessageText && typ != websocket.MessageBinary {
			continue
		}

		c.lastActive.Store(time.Now().Unix())
		if hub != nil {
			hub.RenewUserLocation(c.Uuid)
		}

		// 1. 快速检查是否为心跳 Ping 指令（零反序列化零拷贝快路径，非阻塞就地回包）
		if isPingPayload(data) {
			c.SafeSend(defaultPongBytes, "")
			continue
		}

		// 2. 在当前连接的专属读协程（readPump）中直接串行处理并投递至 Kafka：
		// 严禁投递给 ants 协程池并发处理，否则同一连接连续发送的两条消息会因协程乱序调度而乱序进入 Kafka Partition！
		var producer IncomingMessageProducer
		if hub != nil {
			producer = hub.IncomingProducer()
		}
		HandleUserMessageTaskWithProducer(c, data, producer)
	}
}

// SendPing 发送心跳 Ping 探测帧
func (c *UserConn) SendPing() error {
	if (c.ctx != nil && c.ctx.Err() != nil) || c.Conn == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	err := c.Conn.Ping(ctx)
	if err != nil {
		zap.L().Info("ws ping failed, closing", zap.String("userId", c.Uuid), zap.Error(err))
		c.cleanup()
		return err
	}
	return nil
}

// cleanup 清理连接并注销状态
func (c *UserConn) cleanup() {
	c.cleanupOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		zap.L().Info("ws cleanup start", zap.String("userId", c.Uuid))

		metricsinfra.OnlineConnections.Dec()
		metricsinfra.TotalDisconnections.Inc()

		if c.OnClose != nil {
			c.OnClose(c)
		}
		close(c.sendQueue)

		if c.Conn != nil {
			_ = c.Conn.Close(websocket.StatusNormalClosure, "connection cleanup")
		}

		zap.L().Info("ws cleanup done", zap.String("userId", c.Uuid))
	})
}

// HandleUserMessageTask 异步业务协程任务（兼容无 Kafka 生产者的独立单测入口）
func HandleUserMessageTask(c *UserConn, data []byte) {
	HandleUserMessageTaskWithProducer(c, data, nil)
}

// HandleUserMessageTaskWithProducer 异步业务协程任务：
// 1. 增量对账同步 (action: "sync") 走 gRPC 查询；
// 2. 普通聊天消息上行优先投递至 Kafka 削峰主题 (Key = session_id)，由下游 message_service 顺序落库后通过方案二（回声确认 Echo ACK）下推闭环；
// 3. 当 Kafka 投递失败时遵循快速失败（Fail-fast）原则直接向客户端回推 error 帧，杜绝旁路流量击穿数据库。
func HandleUserMessageTaskWithProducer(c *UserConn, data []byte, producer IncomingMessageProducer) {
	if c.ctx != nil && c.ctx.Err() != nil {
		return
	}

	// 1. 单次反序列化：一次性提取信令动作、超时配置与业务载荷
	var frame WsFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		zap.L().Error("unmarshal ws frame error", zap.String("userId", c.Uuid), zap.Error(err))
		return
	}

	// 心跳 Ping 兜底检查（处理非常规排版的心跳包）
	if frame.IsAction("ping") {
		c.SafeSend(defaultPongBytes, "")
		return
	}

	// 2. 继承客户端传入的 timeout_ms，默认 6s，上限保护 15s
	timeout := 6 * time.Second
	if frame.TimeoutMs > 0 {
		timeout = time.Duration(frame.TimeoutMs) * time.Millisecond
	}
	if timeout > 15*time.Second {
		timeout = 15 * time.Second
	}

	// 3. 必须以连接级的 c.ctx 为根节点派生 Context；
	// 一旦客户端断开断网（触发 c.cancel()），下游调用立即级联取消，避免浪费算力。
	baseCtx := c.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx := context.WithValue(baseCtx, interceptor.UserIDKey, c.Uuid)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 4. 分发 1: 增量对账同步 (action: "sync")
	if frame.IsAction("sync") {
		limit := frame.Limit
		if limit <= 0 {
			limit = 50
		}
		syncResp, syncErr := grpcinfra.GetMessages(ctx, &messagepb.GetMessagesRequest{
			TargetId:    frame.TargetId,
			Cursor:      "after_seq:" + strconv.FormatInt(frame.AfterSeq, 10),
			PageSize:    limit,
			RequesterId: c.Uuid,
		})
		if syncErr != nil {
			zap.L().Warn("ws sync messages failed", zap.String("userId", c.Uuid), zap.Error(syncErr))
			errMsg, _ := json.Marshal(map[string]interface{}{
				"type":    "error",
				"message": syncErr.Error(),
			})
			c.SafeSend(errMsg, "")
			return
		}
		respPayload, _ := json.Marshal(map[string]interface{}{
			"type":      "sync_resp",
			"target_id": frame.TargetId,
			"messages":  syncResp.List,
			"has_more":  syncResp.HasMore,
		})
		c.SafeSend(respPayload, "")
		return
	}

	// 5. 分发 2: 普通聊天消息发送（零二次反序列化，直接装配 SendMessageRequest）
	req := frame.ToSendMessageRequest(c.Uuid)

	if producer != nil {
		if err := producer.PublishIncomingMessage(ctx, req); err != nil {
			zap.L().Warn("publish incoming message to kafka failed (fail-fast)",
				zap.String("userId", c.Uuid),
				zap.String("clientMsgId", req.ClientMsgId),
				zap.Error(err),
			)
			errMsg, _ := json.Marshal(map[string]interface{}{
				"type":          "error",
				"client_msg_id": req.ClientMsgId,
				"message":       err.Error(),
			})
			c.SafeSend(errMsg, "")
		}
		// 方案二（以推代拉 / 回声确认 Implicit Echo ACK）：
		// 消息成功写入 Kafka ISR 后不再由 Connector 原地伪造或阻塞等待 ACK，
		// 而是由 message_service 顺序消费落库并分配权威 seq_id 后，统一通过下行通道将完整消息回推给发送方完成确认闭环。
		return
	}

	resp, err := grpcinfra.SendMessage(ctx, req)
	if err != nil {
		zap.L().Warn("send message failed", zap.String("userId", c.Uuid), zap.Error(err))
		errMsg, _ := json.Marshal(map[string]interface{}{
			"type":          "error",
			"client_msg_id": req.ClientMsgId,
			"message":       err.Error(),
		})
		c.SafeSend(errMsg, "")
		return
	}

	ackMsg, _ := json.Marshal(map[string]interface{}{
		"type":          "ack",
		"message_uuid":  resp.MessageUuid,
		"created_at":    resp.CreatedAt,
		"client_msg_id": req.ClientMsgId,
	})
	c.SafeSend(ackMsg, resp.MessageUuid)
}

// NewUserConn 专职负责创建在线用户的 WebSocket 连接抽象，启动异步写泵并记录基础度量
func NewUserConn(conn *websocket.Conn, clientId string) *UserConn {
	ctx, cancel := context.WithCancel(context.Background())
	client := &UserConn{
		Conn:      conn,
		Uuid:      clientId,
		sendQueue: make(chan []byte, constants.CHANNEL_SIZE),
		ctx:       ctx,
		cancel:    cancel,
	}
	client.lastActive.Store(time.Now().Unix())

	metricsinfra.OnlineConnections.Inc()
	metricsinfra.TotalConnections.Inc()

	if conn != nil {
		go client.writePump()
	}

	zap.L().Info("WebSocket 连接创建成功", zap.String("userId", clientId))
	return client
}

// ClientLogout 当收到前端登出请求时调用
func ClientLogout(clientId string, hub *ConnHub) error {
	client := hub.GetClient(clientId)
	if client != nil {
		client.cleanup()
	}
	return nil
}
