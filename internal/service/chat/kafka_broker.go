// kafka_broker.go
// 核心职责：Kafka 消息消费者，负责上行投递与下行分发
//
// 工作流程：
// 上行：[WebSocket] -> Publish() -> [Kafka Cluster]
// 下行：[Kafka Cluster] -> Start() 消费循环 -> dispatchToUser/dispatchToGroup -> [WebSocket]
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"kama_chat_server/internal/domain/repository"
	messagereq "kama_chat_server/internal/dto/request/message"
	messagersp "kama_chat_server/internal/dto/respond/message"
	cacheutil "kama_chat_server/internal/infrastructure/cache"
	"kama_chat_server/internal/infrastructure/metrics"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"

	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status"
	"kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/otel"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// MsgConsumer Kafka 消息消费者
// 负责：1) 接收 WebSocket 上行消息并投递到 Kafka  2) 从 Kafka 消费消息并分发到在线用户
type MsgConsumer struct {
	// Clients 存储所有在线客户端的映射表，Key 为 UserUUID，Value 为 *UserConn
	// 使用 sync.Map 实现并发安全，无需手动加锁
	Clients sync.Map
	// Login 客户端登录通道，当有新连接建立时写入此通道
	Login chan *UserConn
	// Logout 客户端登出通道，当连接断开时写入此通道
	Logout chan *UserConn
	// closeOnce 确保 channel 只被关闭一次，防止 double-close panic
	closeOnce sync.Once
	// 依赖注入字段（遵循依赖倒置原则）
	kafkaClient     *KafkaClient
	MessageRepo     repository.MessageRepository    // 导出字段，供 ws_gateway 的 Write goroutine 直接访问
	friendshipRepo  repository.FriendshipRepository // 用于消息权限校验（好友关系 + 拉黑检查）
	groupMemberRepo repository.GroupMemberRepository
	sessionRepo     repository.SessionRepository // 用于更新会话最后消息
	cacheService    repository.AsyncCacheService
	cacheHelper     *cacheutil.Helper         // Cache-Aside 辅助工具（含 singleflight 防击穿）
	userRepo        repository.UserRepository // 用于检查用户状态（是否被禁用）
	// quit 用于接收退出信号以优雅关闭消费循环
	quit chan os.Signal
}

// NewMsgConsumer 创建 Kafka 消息消费者实例（依赖注入）
func NewMsgConsumer(
	kafkaClient *KafkaClient,
	messageRepo repository.MessageRepository,
	friendshipRepo repository.FriendshipRepository,
	groupMemberRepo repository.GroupMemberRepository,
	sessionRepo repository.SessionRepository,
	cacheService repository.AsyncCacheService,
	userRepo repository.UserRepository, // 新增：用户仓库，用于检查用户状态
) *MsgConsumer {
	// 初始化 Cache-Aside 辅助工具（复用 cacheService 作为底层缓存实现）
	var helper *cacheutil.Helper
	if cacheService != nil {
		helper = cacheutil.NewHelper(cacheService)
	}

	return &MsgConsumer{
		// sync.Map 零值即可用，无需显式初始化
		Login:           make(chan *UserConn),
		Logout:          make(chan *UserConn),
		kafkaClient:     kafkaClient,
		MessageRepo:     messageRepo,
		friendshipRepo:  friendshipRepo,
		groupMemberRepo: groupMemberRepo,
		sessionRepo:     sessionRepo,
		cacheService:    cacheService,
		cacheHelper:     helper,
		userRepo:        userRepo,
		quit:            make(chan os.Signal, 1),
	}
}

// Publish 将消息投递到 Kafka（由 WebSocket Read goroutine 调用）
func (k *MsgConsumer) Publish(ctx context.Context, msg []byte) error {
	start := time.Now()
	defer func() {
		metrics.PublishDuration.Observe(time.Since(start).Seconds())
	}()

	// 默认使用 "0" 作为分区 Key
	key := []byte("0")

	// 尝试解析消息以获取发送者和接收者 ID，生成逻辑会话 Key (Logical Session Key) 以保证有序性
	var req struct {
		SendId    string `json:"send_id"`
		ReceiveId string `json:"receive_id"`
		Type      int    `json:"type"`
	}
	if err := json.Unmarshal(msg, &req); err == nil && req.ReceiveId != "" {
		// 指标埋点：发布消息到 Kafka
		msgType := "unknown"
		switch req.Type {
		case message_type.Text:
			msgType = "text"
		case message_type.File:
			msgType = "file"
		case message_type.AudioOrVideo:
			msgType = "audio_video"
		}
		metrics.MessagesPublished.WithLabelValues(msgType).Inc()

		if req.ReceiveId[0] == 'U' {
			// 私聊：确保 A->B 和 B->A 的 Key 一致（字典序排序）
			id1, id2 := req.SendId, req.ReceiveId
			if id1 > id2 {
				id1, id2 = id2, id1
			}
			key = []byte(id1 + "_" + id2)
		} else if req.ReceiveId[0] == 'G' {
			// 群聊：直接使用 GroupId 作为 Key
			key = []byte(req.ReceiveId)
		}
	}

	// Build headers with trace context propagation
	var headers []kafka.Header
	otel.InjectTraceContext(ctx, &headers)

	// [第三方库: github.com/segmentio/kafka-go] Writer.WriteMessages 写入消息，kafka.Message 为消息载体
	return k.kafkaClient.Producer.WriteMessages(ctx, kafka.Message{
		Key:     key,
		Value:   msg,
		Headers: headers,
	})
}

// normalizePath 函数已在 channel_server.go 中定义

// Start 启动 Kafka 消费者服务
// 该方法包含两个主要部分的并发逻辑：
// 1. 消息消费循环 (Goroutine): 从 Kafka 读取消息 -> 反序列化 -> 根据类型调用对应的处理函数 (Text/File/AV)
// 2. 客户端管理循环 (Main Loop): 处理用户的登录 (Login) 和 登出 (Logout) 事件，维护 Clients 映射表
func (k *MsgConsumer) Start() {
	// 使用 defer 确保函数退出时释放资源
	defer func() {
		// 捕获 panic 防止整个程序崩溃
		if r := recover(); r != nil {
			zap.L().Error(fmt.Sprintf("kafka server panic: %v", r)) // [第三方库: go.uber.org/zap] 全局日志记录器
		}
		// 通过 sync.Once 关闭通道，防止 double-close panic
		k.closeOnce.Do(func() {
			close(k.Login)
			close(k.Logout)
		})
	}()
	// 1. 启动一个 Goroutine 专门负责从 Kafka 读取消息
	go func() {
		// 同样需要捕获 panic
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error(fmt.Sprintf("kafka server panic: %v", r))
			}
		}()
		// Kafka 消费死循环
		for {
			// 从 Kafka 读取一条消息
			kafkaMessage, err := k.kafkaClient.Consumer.ReadMessage(context.Background())
			if err != nil {
				zap.L().Error("service error", zap.Error(err))
				metrics.KafkaConsumeErrors.Inc()
				continue // 读取失败，重试
			}
			// 记录详细的 Kafka 消息元数据
			zap.L().Debug("kafka message received",
				zap.String("topic", kafkaMessage.Topic),
				zap.Int("partition", kafkaMessage.Partition),
				zap.Int64("offset", kafkaMessage.Offset),
				zap.ByteString("key", kafkaMessage.Key),
				zap.ByteString("value", kafkaMessage.Value))

			// 获取消息体
			messageData := kafkaMessage.Value
			var chatMessageReq messagereq.ChatMessageRequest
			if err := json.Unmarshal(messageData, &chatMessageReq); err != nil {
				zap.L().Error("service error", zap.Error(err))
				continue
			}
			zap.L().Debug("kafka message deserialized", zap.String("type", fmt.Sprintf("%d", chatMessageReq.Type)), zap.String("sendId", chatMessageReq.SendId))

			// Extract trace context from Kafka message headers for distributed tracing
			msgCtx := otel.ExtractTraceContext(context.Background(), kafkaMessage.Headers)

			// 根据消息类型分发处理逻辑
			switch chatMessageReq.Type {
			case message_type.Text:
				k.handleTextMessage(msgCtx, chatMessageReq)
			case message_type.File:
				k.handleFileMessage(msgCtx, chatMessageReq)
			case message_type.AudioOrVideo:
				k.handleAVMessage(msgCtx, chatMessageReq)
			}
		}
	}()

	// 2. 主循环：负责处理客户端的登录和登出事件（同步阻塞）
	for {
		select {
		// 处理登录
		case client := <-k.Login:
			k.Clients.Store(client.Uuid, client)
			zap.L().Debug(fmt.Sprintf("欢迎来到kama聊天服务器，亲爱的用户%s\n", client.Uuid))
			if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("欢迎来到kama聊天服务器")); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}

		// 处理退出
		case client := <-k.Logout:
			k.Clients.Delete(client.Uuid)
			zap.L().Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
			if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("已退出登录")); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
		// 处理系统退出信号
		case <-k.quit:
			return
		}
	}
}

// Close 关闭服务通道（通过 sync.Once 确保只执行一次）
func (k *MsgConsumer) Close() {
	k.closeOnce.Do(func() {
		close(k.Login)
		close(k.Logout)
	})
}

// GetClient 从在线用户列表中获取指定用户的 WebSocket 连接
func (k *MsgConsumer) GetClient(userId string) *UserConn {
	value, ok := k.Clients.Load(userId)
	if !ok {
		return nil
	}
	return value.(*UserConn)
}

// RegisterClient 将用户连接加入登录队列
func (k *MsgConsumer) RegisterClient(client *UserConn) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Warn("RegisterClient: Login channel is closed", zap.Any("recover", r))
		}
	}()
	k.Login <- client
}

// UnregisterClient 将用户连接加入登出队列
func (k *MsgConsumer) UnregisterClient(client *UserConn) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Warn("UnregisterClient: Logout channel is closed", zap.Any("recover", r))
		}
	}()
	k.Logout <- client
}

// PushRecallNotify 向指定用户推送撤回通知（直接本地推送，不走 Kafka）
// messageUuid: 被撤回的消息 UUID
// receiveId: 需要收到通知的用户 UUID（私聊为对方，群聊则需外部遍历群成员逐个调用）
func (k *MsgConsumer) PushRecallNotify(messageUuid, receiveId string) {
	// 构造撤回通知 JSON：前端根据 type=Recall + message_uuid 移除/标记对应消息
	recallMsg := map[string]interface{}{
		"type":         message_type.Recall,
		"message_uuid": messageUuid,
	}
	jsonMsg, err := json.Marshal(recallMsg)
	if err != nil {
		zap.L().Error("序列化撤回通知失败", zap.Error(err))
		return
	}

	// 仅在用户在线时推送，不在线时忽略（下次拉取消息列表会看到已撤回状态）
	if value, ok := k.Clients.Load(receiveId); ok {
		client := value.(*UserConn)
		trySendBack(client, &MessageBack{Message: jsonMsg, Uuid: ""})
	}
}

// isDuplicateMessage 幂等检查：通过 Redis SETNX 判断消息是否已处理过
// clientMsgId: 客户端生成的消息唯一标识
// 返回值：
//   - isDuplicate: true 表示重复（已处理过），false 表示新消息
//   - isDegraded:  true 表示 Redis 异常，已降级放行（依赖 MySQL 唯一索引兜底）
func (k *MsgConsumer) isDuplicateMessage(clientMsgId string) (isDuplicate bool, isDegraded bool) {
	if clientMsgId == "" {
		return false, false // 未携带 client_msg_id，跳过幂等检查（兼容旧客户端）
	}
	dedupKey := "msg:dedup:" + clientMsgId
	ok, err := k.cacheService.SetNX(context.Background(), dedupKey, "1", 24*time.Hour)
	if err != nil {
		zap.L().Error("幂等检查失败，降级放行（依赖 MySQL 唯一索引兜底）",
			zap.String("client_msg_id", clientMsgId), zap.Error(err))
		metrics.MessagesDegrade.Inc() // 记录降级指标
		return false, true            // Redis 异常时放行，依赖 MySQL ON DUPLICATE KEY 兜底
	}
	if !ok {
		metrics.MessagesDuplicated.Inc() // SETNX 返回 false 表示键已存在，即重复消息
	}
	return !ok, false
}

// checkSendPermission 校验发送者是否有权向目标发消息
// 1. 检查发送者状态（是否被禁用）
// 2. 私聊: 检查好友关系（IsFriend 同时包含拉黑状态检查）
// 3. 群聊: 检查群成员身份
// 返回 nil 表示允许，返回 error 表示拒绝（error.Error() 包含拒绝原因）
func (k *MsgConsumer) checkSendPermission(sendId, receiveId string) error {
	ctx := context.Background()
	if len(receiveId) == 0 {
		return fmt.Errorf("接收者ID不能为空")
	}

	// 1. 检查发送者状态（严重安全漏洞修复：被禁用的用户不应能发送消息）
	if k.userRepo != nil {
		sender, err := k.userRepo.FindByUuid(ctx, sendId)
		if err != nil {
			zap.L().Error("查询发送者信息失败", zap.String("sendId", sendId), zap.Error(err))
			return fmt.Errorf("权限校验失败")
		}
		if sender.Status == user_status.DISABLE {
			zap.L().Warn("被禁用的用户尝试发送消息", zap.String("sendId", sendId))
			return fmt.Errorf("您的账号已被禁用，无法发送消息")
		}
	}

	if receiveId[0] == 'U' {
		// 私聊：校验好友关系（IsFriend 要求双向 status=NORMAL，拉黑后自动失败）
		if k.friendshipRepo != nil {
			isFriend, err := k.friendshipRepo.IsFriend(ctx, sendId, receiveId)
			if err != nil {
				zap.L().Error("检查好友关系失败", zap.String("sendId", sendId), zap.String("receiveId", receiveId), zap.Error(err))
				return fmt.Errorf("权限校验失败")
			}
			if !isFriend {
				return fmt.Errorf("你们还不是好友，无法发送消息")
			}
		}
	} else if receiveId[0] == 'G' {
		// 群聊：校验群成员身份
		if k.groupMemberRepo != nil {
			_, err := k.groupMemberRepo.FindByGroupAndUser(ctx, receiveId, sendId)
			if err != nil {
				return fmt.Errorf("你不是该群成员，无法发送消息")
			}
		}
	}

	return nil
}

// sendPermissionError 向发送者推送权限拒绝消息
func (k *MsgConsumer) sendPermissionError(sendId, reason string) {
	errMsg := map[string]interface{}{
		"type":    "error",
		"message": reason,
	}
	jsonMsg, err := json.Marshal(errMsg) // [标准库: encoding/json] 序列化为 JSON
	if err != nil {
		return
	}
	if value, ok := k.Clients.Load(sendId); ok {
		client := value.(*UserConn)
		trySendBack(client, &MessageBack{Message: jsonMsg, Uuid: ""})
	}
}

// trySendBack 非阻塞地向客户端 SendBack 通道写入消息
// 如果通道已满，丢弃消息并记录告警日志，防止阻塞 Kafka 消费者
func trySendBack(client *UserConn, msg *MessageBack) {
	select {
	case client.SendBack <- msg:
		// 成功发送
		metrics.MessagesDispatched.Inc()
	default:
		zap.L().Warn("SendBack channel full, message dropped",
			zap.String("userId", client.Uuid),
			zap.String("msgUuid", msg.Uuid))
		metrics.MessagesDropped.Inc()
	}
}

// buildMessageFromRequest 从请求构建消息模型
// 改进建议实现：提取公共逻辑，减少代码重复
func (k *MsgConsumer) buildMessageFromRequest(req messagereq.ChatMessageRequest) model.Message {
	return model.Message{
		Uuid:        "M" + snowflake.GenerateIDString(),
		ClientMsgId: req.ClientMsgId, // 携带客户端消息 ID，用于 MySQL 层面幂等兜底
		SessionId:   req.SessionId,
		Type:        req.Type,
		Content:     req.Content,
		Url:         req.Url,
		SendId:      req.SendId,
		SendName:    req.SendName,
		SendAvatar:  normalizePath(req.SendAvatar), // 规范化头像路径
		ReceiveId:   req.ReceiveId,
		FileSize:    req.FileSize,
		FileType:    req.FileType,
		FileName:    req.FileName,
		Status:      message_status.Unsent,
		AVdata:      req.AVdata,
	}
}

// persistMessage 持久化消息到数据库
func (k *MsgConsumer) persistMessage(message *model.Message) {
	if k.MessageRepo != nil {
		if err := k.MessageRepo.Create(context.Background(), message); err != nil {
			zap.L().Error("创建消息失败", zap.Error(err))
		}
	}
}

// updateSessionLastMessage 异步更新会话最后消息
//
// 注意：私聊场景需要更新双向会话，因为 A→B 和 B→A 是两个独立的会话记录
// 群聊场景只需更新发送者的会话（群成员各自维护自己的会话）
func (k *MsgConsumer) updateSessionLastMessage(message *model.Message, content string) {
	if k.sessionRepo == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// 1. 更新发送者的会话
		if err := k.sessionRepo.UpdateLastMessage(ctx,
			message.SendId,
			message.ReceiveId,
			content,
			message.Type,
			message.CreatedAt,
		); err != nil {
			zap.L().Error("更新发送者会话最后消息失败",
				zap.String("sendId", message.SendId),
				zap.String("receiveId", message.ReceiveId),
				zap.Error(err))
		}

		// 2. 私聊场景：同步更新接收者的会话（反向）
		// 私聊中，接收者的会话记录是：send_id=接收者, receive_id=发送者
		if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
			if err := k.sessionRepo.UpdateLastMessage(ctx,
				message.ReceiveId, // 接收者作为会话的发起方
				message.SendId,    // 发送者作为会话的接收方
				content,
				message.Type,
				message.CreatedAt,
			); err != nil {
				// 接收者可能没有主动创建过会话，这是正常情况，降级为警告日志
				zap.L().Warn("更新接收者会话最后消息失败（可能会话不存在）",
					zap.String("sendId", message.ReceiveId),
					zap.String("receiveId", message.SendId),
					zap.Error(err))
			}
		}
		// 群聊场景：不需要反向更新，群成员各自维护自己的 send_id=自己, receive_id=群ID 的会话
	}()
}

// handleTextMessage 处理文本消息
// 1. 生成 Snowflake ID
// 2. 将消息持久化到 MySQL
// 3. 根据接收者类型 (User/Group) 路由消息
// 4. 更新 Redis 缓存
func (k *MsgConsumer) handleTextMessage(ctx context.Context, req messagereq.ChatMessageRequest) {
	// 幂等检查：防止客户端重发消息导致重复消费
	isDup, _ := k.isDuplicateMessage(req.ClientMsgId)
	if isDup {
		zap.L().Warn("重复消息，跳过处理", zap.String("client_msg_id", req.ClientMsgId))
		return
	}

	// 指标埋点：消费文本消息
	metrics.MessagesConsumed.WithLabelValues("text").Inc()

	// 权限校验：检查发送者是否有权向目标发消息
	if err := k.checkSendPermission(req.SendId, req.ReceiveId); err != nil {
		zap.L().Warn("消息权限校验失败", zap.String("sendId", req.SendId), zap.String("receiveId", req.ReceiveId), zap.String("reason", err.Error()))
		k.sendPermissionError(req.SendId, err.Error())
		return
	}

	// 改进建议实现：使用提取的公共函数构建消息
	message := k.buildMessageFromRequest(req)
	// 文本消息特有字段设置
	message.Url = ""
	message.FileSize = "0B"
	message.FileType = ""
	message.FileName = ""
	message.AVdata = ""

	// 通过 Repository 接口入库
	k.persistMessage(&message)

	// 异步更新会话最后消息（用于会话列表排序和摘要显示）
	k.updateSessionLastMessage(&message, message.Content)

	// 路由分发
	if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' { // 发送给User
		k.dispatchToUser(message, req.SendAvatar)
	} else if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'G' { // 发送给Group
		k.dispatchToGroup(message, req.SendAvatar)
	}
}

// handleFileMessage 处理文件消息
// 逻辑与文本消息类似，区别在于 Content 为空，Url 字段存储文件链接
// 改进建议实现：使用提取的公共函数减少代码重复
func (k *MsgConsumer) handleFileMessage(ctx context.Context, req messagereq.ChatMessageRequest) {
	// 幂等检查：防止客户端重发消息导致重复消费
	isDup, _ := k.isDuplicateMessage(req.ClientMsgId)
	if isDup {
		zap.L().Warn("重复消息，跳过处理", zap.String("client_msg_id", req.ClientMsgId))
		return
	}

	// 指标埋点：消费文件消息
	metrics.MessagesConsumed.WithLabelValues("file").Inc()

	// 权限校验：检查发送者是否有权向目标发消息
	if err := k.checkSendPermission(req.SendId, req.ReceiveId); err != nil {
		zap.L().Warn("文件消息权限校验失败", zap.String("sendId", req.SendId), zap.String("receiveId", req.ReceiveId), zap.String("reason", err.Error()))
		k.sendPermissionError(req.SendId, err.Error())
		return
	}

	// 改进建议实现：使用提取的公共函数构建消息
	message := k.buildMessageFromRequest(req)
	// 文件消息特有字段设置
	message.Content = ""
	message.AVdata = ""

	// 通过 Repository 接口入库
	k.persistMessage(&message)

	// 异步更新会话最后消息（文件消息显示文件名作为摘要）
	content := "[文件] " + req.FileName
	k.updateSessionLastMessage(&message, content)

	// 路由分发
	if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
		k.dispatchToUser(message, req.SendAvatar)
	} else if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'G' {
		k.dispatchToGroup(message, req.SendAvatar)
	}
}

// handleAVMessage 处理音视频通话信令
//
// 与文本/文件消息的核心区别：音视频"消息"本质上是 WebRTC 信令（signaling），不是聊天内容。
// 因此有两个设计差异：
//
//  1. 不回显给发送者：文本/文件消息会通过 dispatchToUser 同时推送给收发双方（发送者需在 UI 看到自己发的消息），
//     而信令只需单向透传给接收者（发送者自己发的 offer/answer/candidate 自己已经知道了）。
//
// 2. 选择性入库：并非所有信令都入库，只有关键节点信令会持久化到数据库用于聊天记录展示：
//   - start_call   → 聊天记录显示"发起了通话"
//   - receive_call → 聊天记录显示"已接听"
//   - reject_call  → 聊天记录显示"已拒绝"
//     而 offer/answer/candidate 等 WebRTC 连接协商信令是纯技术细节，不入库也不显示。
//
// 3. 不更新会话最后消息：信令不应覆盖会话列表的最后消息摘要，因此不调用 updateSessionLastMessage。
func (k *MsgConsumer) handleAVMessage(ctx context.Context, req messagereq.ChatMessageRequest) {
	// 幂等检查：防止客户端重发消息导致重复消费
	isDup, _ := k.isDuplicateMessage(req.ClientMsgId)
	if isDup {
		zap.L().Warn("重复消息，跳过处理", zap.String("client_msg_id", req.ClientMsgId))
		return
	}

	// 指标埋点：消费音视频信令
	metrics.MessagesConsumed.WithLabelValues("audio_video").Inc()

	// 权限校验：检查发送者是否有权向目标发消息
	if err := k.checkSendPermission(req.SendId, req.ReceiveId); err != nil {
		zap.L().Warn("AV消息权限校验失败", zap.String("sendId", req.SendId), zap.String("receiveId", req.ReceiveId), zap.String("reason", err.Error()))
		k.sendPermissionError(req.SendId, err.Error())
		return
	}

	var avData messagereq.AVSignalData
	if err := json.Unmarshal([]byte(req.AVdata), &avData); err != nil { // [标准库: encoding/json] 反序列化音视频信令数据
		zap.L().Error("service error", zap.Error(err))
		return
	}

	// 构建消息模型
	message := model.Message{
		Uuid:       "M" + snowflake.GenerateIDString(),
		SessionId:  req.SessionId,
		Type:       req.Type,
		Content:    "",
		Url:        "",
		SendId:     req.SendId,
		SendName:   req.SendName,
		SendAvatar: req.SendAvatar,
		ReceiveId:  req.ReceiveId,
		FileSize:   "",
		FileType:   "",
		FileName:   "",
		Status:     message_status.Unsent,
		AVdata:     req.AVdata,
	}

	// 关键信令入库（仅 PROXY 类型中的 start_call/receive_call/reject_call 三种信令会持久化）
	if avData.MessageId == "PROXY" && (avData.Type == "start_call" || avData.Type == "receive_call" || avData.Type == "reject_call") {
		message.SendAvatar = normalizePath(message.SendAvatar)
		if k.MessageRepo != nil {
			if err := k.MessageRepo.Create(context.Background(), &message); err != nil {
				zap.L().Error("创建音视频消息失败", zap.Error(err))
			}
		}

		// 更新会话最后消息摘要（微信风格：会话列表显示通话状态）
		var summary string
		switch avData.Type {
		case "start_call":
			summary = "[通话] 发起了通话"
		case "receive_call":
			summary = "[通话] 已接听"
		case "reject_call":
			summary = "[通话] 已拒绝"
		}
		k.updateSessionLastMessage(&message, summary)
	}

	// 处理单聊信令转发（只推送给接收者，不回显给发送者）
	if len(req.ReceiveId) > 0 && req.ReceiveId[0] == 'U' {
		// 构造响应
		messageRsp := messagersp.AVMessageRespond{
			SendId:     message.SendId,
			SendName:   message.SendName,
			SendAvatar: message.SendAvatar,
			ReceiveId:  message.ReceiveId,
			Type:       message.Type,
			Content:    message.Content,
			Url:        message.Url,
			FileSize:   message.FileSize,
			FileName:   message.FileName,
			FileType:   message.FileType,
			CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
			AVdata:     message.AVdata,
		}
		jsonMessage, err := json.Marshal(messageRsp) // [标准库: encoding/json] 序列化响应体
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		zap.L().Debug("AV message response", zap.String("sendId", messageRsp.SendId), zap.String("receiveId", messageRsp.ReceiveId))

		messageBack := &MessageBack{
			Message: jsonMessage,
			Uuid:    message.Uuid,
		}

		// 推送给接收者 (sync.Map 自动处理并发安全，非阻塞写入)
		if value, ok := k.Clients.Load(message.ReceiveId); ok {
			receiveClient := value.(*UserConn)
			trySendBack(receiveClient, messageBack)
		}
	}
}

// dispatchToUser 将消息分发到私聊双方的 SendBack channel
// 1. 构造响应 DTO
// 2. 写入接收者的 SendBack channel（由 Write goroutine 真正推送到 WebSocket）
// 3. 回显到发送者的 SendBack channel
// 4. 异步更新 Redis 中的双人聊天记录缓存
func (k *MsgConsumer) dispatchToUser(message model.Message, originalAvatar string) {
	// 构造响应体
	messageRsp := messagersp.GetMessageListRespond{
		SendId:     message.SendId,
		SendName:   message.SendName,
		SendAvatar: originalAvatar,
		ReceiveId:  message.ReceiveId,
		Type:       message.Type,
		Content:    message.Content,
		Url:        message.Url,
		FileSize:   message.FileSize,
		FileName:   message.FileName,
		FileType:   message.FileType,
		CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	jsonMessage, err := json.Marshal(messageRsp) // [标准库: encoding/json] 序列化私聊响应体
	if err != nil {
		zap.L().Error("service error", zap.Error(err)) // [第三方库: go.uber.org/zap]
	}

	zap.L().Debug("message response built", zap.String("sendId", message.SendId), zap.String("receiveId", message.ReceiveId))

	messageBack := &MessageBack{
		Message: jsonMessage,
		Uuid:    message.Uuid,
	}

	// 消息分发到 SendBack channel (sync.Map 自动处理并发安全，非阻塞写入)
	// 给接收者分发
	if value, ok := k.Clients.Load(message.ReceiveId); ok {
		receiveClient := value.(*UserConn)
		trySendBack(receiveClient, messageBack)
	}
	// 给发送者回显（让发送者确认消息已处理）
	if value, ok := k.Clients.Load(message.SendId); ok {
		sendClient := value.(*UserConn)
		trySendBack(sendClient, messageBack)
	}

	// 通过注入的缓存服务异步更新缓存
	if k.cacheService != nil {
		k.cacheService.SubmitTask(func() {
			// 确保缓存 Key 中 ID 顺序与 GetMessageList 查询一致（较小的ID在前）
			id1, id2 := message.SendId, message.ReceiveId
			if id1 > id2 {
				id1, id2 = id2, id1
			}
			key := constants.CacheKeyMessageList + id1 + "_" + id2
			rspString, err := k.cacheService.GetOrError(context.Background(), key)
			if err == nil {
				var list []messagersp.GetMessageListRespond
				if err := json.Unmarshal([]byte(rspString), &list); err == nil {
					list = append(list, messageRsp)
					if rspByte, err := json.Marshal(list); err == nil {
						_ = k.cacheService.Set(context.Background(), key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
					}
				}
			}
		})
	}
}

// dispatchToGroup 将消息分发到群组各成员的 SendBack channel
// 1. 构造群组响应 DTO
// 2. 查询群成员列表（优先走 Redis 缓存）
// 3. 遍历成员，写入各自的 SendBack channel（排除发送者）
// 4. 回显到发送者的 SendBack channel
// 5. 异步更新 Redis 中的群组聊天记录缓存
func (k *MsgConsumer) dispatchToGroup(message model.Message, originalAvatar string) {
	// 构造群聊响应
	messageRsp := messagersp.GetMessageListRespond{
		SendId:     message.SendId,
		SendName:   message.SendName,
		SendAvatar: originalAvatar,
		ReceiveId:  message.ReceiveId,
		Type:       message.Type,
		Content:    message.Content,
		Url:        message.Url,
		FileSize:   message.FileSize,
		FileName:   message.FileName,
		FileType:   message.FileType,
		CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	jsonMessage, err := json.Marshal(messageRsp) // [标准库: encoding/json] 序列化群聊响应体
	if err != nil {
		zap.L().Error("service error", zap.Error(err)) // [第三方库: go.uber.org/zap]
	}

	zap.L().Debug("message response built", zap.String("sendId", message.SendId), zap.String("receiveId", message.ReceiveId))

	messageBack := &MessageBack{
		Message: jsonMessage,
		Uuid:    message.Uuid,
	}

	// 通过缓存优先获取群成员 ID 列表，减少 DB 查询
	var groupMembers []model.GroupMember
	if k.groupMemberRepo != nil {
		groupMembers = k.getGroupMembersCached(message.ReceiveId)
	}

	// 分发到各成员的 SendBack channel (sync.Map 自动处理并发安全，非阻塞写入)
	for _, gm := range groupMembers {
		if gm.UserUuid != message.SendId {
			// 分发给其他成员
			if value, ok := k.Clients.Load(gm.UserUuid); ok {
				receiveClient := value.(*UserConn)
				trySendBack(receiveClient, messageBack)
			}
		} else {
			// 回显给发送者
			if value, ok := k.Clients.Load(message.SendId); ok {
				sendClient := value.(*UserConn)
				trySendBack(sendClient, messageBack)
			}
		}
	}

	// 通过注入的缓存服务异步更新缓存
	if k.cacheService != nil {
		k.cacheService.SubmitTask(func() {
			key := constants.CacheKeyGroupMessageList + message.ReceiveId
			rspString, err := k.cacheService.GetOrError(context.Background(), key)
			if err == nil {
				var list []messagersp.GetMessageListRespond
				if err := json.Unmarshal([]byte(rspString), &list); err == nil {
					list = append(list, messageRsp)
					if rspByte, err := json.Marshal(list); err == nil {
						_ = k.cacheService.Set(context.Background(), key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
					}
				}
			}
		})
	}
}

// getGroupMembersCached 通过 cache.Helper 实现 Cache-Aside 模式获取群成员列表
// 相比手写版本，额外获得:
//   - singleflight: 并发请求同一 groupId 只查一次 DB（防击穿）
//   - RandomizedTTL: TTL 自动加随机偏移（防雪崩）
func (k *MsgConsumer) getGroupMembersCached(groupId string) []model.GroupMember {
	ctx := context.Background()
	// 降级处理：cacheHelper 未初始化时直接查 DB
	if k.cacheHelper == nil {
		groupMembers, err := k.groupMemberRepo.FindByGroupUuid(ctx, groupId)
		if err != nil {
			zap.L().Error("查询群成员失败", zap.Error(err))
			return nil
		}
		return groupMembers
	}

	cacheKey := constants.CacheKeyGroupMemberIDs + groupId

	var memberIds []string
	// [项目工具包: internal/infrastructure/cache] Helper.GetOrLoad 实现 Cache-Aside 模式
	// 内部自动完成: 查缓存 → singleflight 防击穿 → loader 回调查 DB → 序列化回写缓存
	err := k.cacheHelper.GetOrLoad(
		context.Background(),
		cacheKey,
		func(ctx context.Context) (interface{}, error) {
			// loader: 缓存 miss 时查 DB，仅存储 UserUuid 列表（减少缓存体积）
			members, err := k.groupMemberRepo.FindByGroupUuid(ctx, groupId)
			if err != nil {
				return nil, err
			}
			ids := make([]string, len(members))
			for i, gm := range members {
				ids[i] = gm.UserUuid
			}
			return ids, nil
		},
		5*time.Minute, // TTL（内部会通过 RandomizedTTL 加 ±10% 随机偏移防雪崩）
		0,             // nullTTL = 0 表示不缓存空值
		&memberIds,
	)
	if err != nil {
		zap.L().Error("查询群成员失败", zap.Error(err))
		return nil
	}

	// 将 ID 列表转换为 GroupMember 模型
	members := make([]model.GroupMember, len(memberIds))
	for i, id := range memberIds {
		members[i] = model.GroupMember{UserUuid: id}
	}
	return members
}
