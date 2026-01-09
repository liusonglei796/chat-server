// Package chat 实现了聊天系统的核心服务层
// channel_server.go
// 核心职责：单机模式下的聊天服务器实现
// 1. 维护在线用户连接 (Channel 模式)
// 2. 处理消息的直接路由转发
// 3. 管理用户登录/登出事件
// 4. 不依赖外部消息队列，适合小规模或开发环境
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status_enum"
	"kama_chat_server/pkg/enum/message/message_type_enum"
	"kama_chat_server/pkg/util/snowflake"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// StandaloneServer 定义了 WebSocket 服务的核心结构
type StandaloneServer struct {
	// Clients 存储所有在线客户端的映射表，Key 为 UserUUID，Value 为 *UserConn
	// 使用 sync.Map 实现并发安全，无需手动加锁
	Clients sync.Map
	// Transmit 消息转发通道，用于处理接收到的广播或路由消息
	Transmit chan []byte
	// Login 客户端登录通道，当有新连接建立时写入此通道
	Login chan *UserConn
	// Logout 客户端登出通道，当连接断开时写入此通道
	Logout chan *UserConn

	// 依赖注入字段（遵循依赖倒置原则）
	messageRepo     mysql.MessageRepository
	groupMemberRepo mysql.GroupMemberRepository
	cacheService    myredis.AsyncCacheService
}

// NewStandaloneServer 创建 ChannelBroker 实例（依赖注入）
func NewStandaloneServer(
	messageRepo mysql.MessageRepository,
	groupMemberRepo mysql.GroupMemberRepository,
	cacheService myredis.AsyncCacheService,
) *StandaloneServer {
	return &StandaloneServer{
		// sync.Map 零值即可用，无需显式初始化
		// 初始化消息转发通道，设置缓冲区大小
		Transmit: make(chan []byte, constants.CHANNEL_SIZE),
		// 初始化登录通道
		Login: make(chan *UserConn, constants.CHANNEL_SIZE),
		// 初始化登出通道
		Logout:          make(chan *UserConn, constants.CHANNEL_SIZE),
		messageRepo:     messageRepo,
		groupMemberRepo: groupMemberRepo,
		cacheService:    cacheService,
	}
}

// normalizePath 将完整 URL 转换为相对路径
// 例如: https://127.0.0.1:8000/static/xxx -> /static/xxx
// 特殊处理: 保留 elemecdn 的默认头像链接
func normalizePath(path string) string {
	// 特殊处理默认头像（如果是远程链接且不含 /static/ 则原样返回）
	if strings.HasPrefix(path, "https://cube.elemecdn.com") {
		return path
	}
	// 查找 "/static/" 的位置
	idx := strings.Index(path, "/static/")
	// 如果没找到 "/static/"，说明不是本地静态资源路径，直接返回原路径
	if idx == -1 {
		return path
	}
	// 返回从 "/static/" 开始的子串，即相对路径
	return path[idx:]
}

// Start 启动 Channel Server 主循环
// 该方法包含两个主要部分的并发逻辑：
// 1. 消息消费循环 (Transmit channel): 接收消息 -> 反序列化 -> 根据类型调用对应的处理函数
// 2. 客户端管理循环 (Login/Logout channels): 处理用户的登录和登出事件，维护 Clients 映射表
// Kafka Consumer (消费者)动作：后台死循环 -> 调用 KafkaService.ChatReader.ReadMessage 从 Kafka 拿消息 -> 处理业务（入库、转发）。

func (s *StandaloneServer) Start() {
	// 从死循环中处理各种 channel 事件
	for {
		select {
		// 处理客户端登录事件
		case client, ok := <-s.Login:
			if !ok {
				return
			}
			if client == nil {
				continue
			}
			// 将新连接的客户端加入映射表 (sync.Map 自动处理并发安全)
			s.Clients.Store(client.Uuid, client)
			// 记录调试日志
			zap.L().Debug(fmt.Sprintf("欢迎来到kama聊天服务器，亲爱的用户%s\n", client.Uuid))
			// 向当事人发送欢迎消息
			if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("欢迎来到kama聊天服务器")); err != nil {
				zap.L().Error(err.Error())
			}

		// 处理客户端登出事件
		case client, ok := <-s.Logout:
			if !ok {
				return
			}
			if client == nil {
				continue
			}
			// 从映射表中移除断开的客户端 (sync.Map 自动处理并发安全)
			s.Clients.Delete(client.Uuid)
			// 记录日志
			zap.L().Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
			// 尝试发送退出消息（如果连接还未完全关闭）
			if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("已退出登录")); err != nil {
				zap.L().Error(err.Error())
			}

		// 处理消息转发事件（这是核心的消息处理循环）
		case data, ok := <-s.Transmit:
			if !ok {
				return
			}
			// 声明请求对象
			var chatMessageReq request.ChatMessageRequest
			// 将 JSON 数据反序列化为请求对象
			if err := json.Unmarshal(data, &chatMessageReq); err != nil {
				zap.L().Error(err.Error())
				continue // 反序列化失败则跳过该消息
			}

			// 根据消息类型分发到不同的处理函数
			switch chatMessageReq.Type {
			case message_type_enum.Text:
				// 处理文本消息
				s.handleTextMessage(chatMessageReq)
			case message_type_enum.File:
				// 处理文件/图片消息
				s.handleFileMessage(chatMessageReq)
			case message_type_enum.AudioOrVideo:
				// 处理音视频信令
				s.handleAVMessage(chatMessageReq)
			}
		}
	}
}

// handleTextMessage 处理文本消息
// 1. 生成 Snowflake ID
// 2. 将消息持久化到 MySQL
// 3. 根据接收者类型 (User/Group) 路由消息
// 4. 更新 Redis 缓存
func (s *StandaloneServer) handleTextMessage(req request.ChatMessageRequest) {
	// 构建数据库模型对象
	message := model.Message{
		Uuid:       snowflake.GenerateID(),     // 生成唯一消息ID
		SessionId:  req.SessionId,              // 会话ID
		Type:       req.Type,                   // 消息类型
		Content:    req.Content,                // 消息内容
		Url:        "",                         // 文本消息无 URL
		SendId:     req.SendId,                 // 发送者ID
		SendName:   req.SendName,               // 发送者昵称
		SendAvatar: req.SendAvatar,             // 发送者头像
		ReceiveId:  req.ReceiveId,              // 接收者ID
		FileSize:   "0B",                       // 文本消息大小为0
		FileType:   "",                         // 文件类型为空
		FileName:   "",                         // 文件名为空
		Status:     message_status_enum.Unsent, // 初始状态为未发送
		AVdata:     "",                         // 无音视频数据
	}
	// 规范化头像路径
	message.SendAvatar = normalizePath(message.SendAvatar)

	// 通过 Repository 接口存入数据库（遵循依赖倒置原则）
	if s.messageRepo != nil {
		if err := s.messageRepo.Create(&message); err != nil {
			zap.L().Error("创建消息失败", zap.Error(err))
		}
	}

	// 根据 ReceiveId 的前缀判断是单聊还是群聊
	if message.ReceiveId[0] == 'U' {
		// 单聊：发送给指定用户
		s.sendToUser(message, req.SendAvatar)
	} else if message.ReceiveId[0] == 'G' {
		// 群聊：发送给群组成员
		s.sendToGroup(message, req.SendAvatar)
	}
}

// handleFileMessage 处理文件消息
// 逻辑与文本消息类似，区别在于 Content 为空，Url 字段存储文件链接
func (s *StandaloneServer) handleFileMessage(req request.ChatMessageRequest) {
	// 构建数据库模型对象
	message := model.Message{
		Uuid:       snowflake.GenerateID(),
		SessionId:  req.SessionId,
		Type:       req.Type,
		Content:    "",      // 文件消息内容为空
		Url:        req.Url, // 存储文件链接
		SendId:     req.SendId,
		SendName:   req.SendName,
		SendAvatar: req.SendAvatar,
		ReceiveId:  req.ReceiveId,
		FileSize:   req.FileSize, // 记录文件大小
		FileType:   req.FileType, // 记录文件类型
		FileName:   req.FileName, // 记录文件名
		Status:     message_status_enum.Unsent,
		AVdata:     "",
	}
	// 规范化头像路径
	message.SendAvatar = normalizePath(message.SendAvatar)

	// 通过 Repository 接口存入数据库
	if s.messageRepo != nil {
		if err := s.messageRepo.Create(&message); err != nil {
			zap.L().Error("创建文件消息失败", zap.Error(err))
		}
	}

	// 路由分发
	if message.ReceiveId[0] == 'U' {
		s.sendToUser(message, req.SendAvatar)
	} else {
		s.sendToGroup(message, req.SendAvatar)
	}
}

// handleAVMessage 处理音视频通话信令
// 信令消息（如 start_call）用于 WebRTC 连接建立
// 大部分信令只需透传，特定关键信令（如 PROXY 类型中的 start_call 等）会持久化到数据库
func (s *StandaloneServer) handleAVMessage(req request.ChatMessageRequest) {
	// 反序列化 AVData
	var avData request.AVData
	if err := json.Unmarshal([]byte(req.AVdata), &avData); err != nil {
		zap.L().Error(err.Error())
		return
	}

	// 构建消息模型 (注意：这里大部分信令只是临时构建用于转发，不一定都入库)
	message := model.Message{
		Uuid:       snowflake.GenerateID(),
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
		Status:     message_status_enum.Unsent,
		AVdata:     req.AVdata, // 存储信令数据
	}

	// 只有特定的 Proxy 信令类型(如开始通话、接听、拒绝)才会被持久化
	// 这有助于后续的历史记录查询，而中间的 Candidate 交换等过程则不存储
	if avData.MessageId == "PROXY" && (avData.Type == "start_call" || avData.Type == "receive_call" || avData.Type == "reject_call") {
		message.SendAvatar = normalizePath(message.SendAvatar)
		if s.messageRepo != nil {
			if err := s.messageRepo.Create(&message); err != nil {
				zap.L().Error("创建音视频消息失败", zap.Error(err))
			}
		}
	}

	// 只能是单聊 (群聊暂不支持 WebRTC 信令转发逻辑)
	if req.ReceiveId[0] == 'U' {
		// 构建信令响应对象
		messageRsp := respond.AVMessageRespond{
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
		// 序列化为 JSON
		jsonMessage, err := json.Marshal(messageRsp)
		if err != nil {
			zap.L().Error(err.Error())
		}
		log.Println("返回的消息为：", messageRsp)

		// 构建回传消息包
		messageBack := &MessageBack{
			Message: jsonMessage,
			Uuid:    message.Uuid,
		}

		// 查找目标客户端 (sync.Map 自动处理并发安全)
		if value, ok := s.Clients.Load(message.ReceiveId); ok {
			// 如果在线，推送消息
			receiveClient := value.(*UserConn)
			receiveClient.SendBack <- messageBack
		}
		// 注意：通话信令通常不回显给发送者，避免前端重复触发逻辑
	}
}

// sendToUser 辅助方法：发送消息给单独用户
// 1. 构造响应 DTO
// 2. 通过 WebSocket 推送给接收者 (如果在线)
// 3. 回显给发送者 (如果在线)
// 4. 异步更新 Redis 中的双人聊天记录缓存
func (s *StandaloneServer) sendToUser(message model.Message, originalAvatar string) {
	// 构造返回给前端的响应体
	messageRsp := respond.GetMessageListRespond{
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

	// 序列化
	jsonMessage, err := json.Marshal(messageRsp)
	if err != nil {
		zap.L().Error(err.Error())
	}

	// 打印日志辅助调试
	log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

	// 封装为 MessageBack 对象
	messageBack := &MessageBack{
		Message: jsonMessage,
		Uuid:    message.Uuid,
	}

	// 消息投递 (sync.Map 自动处理并发安全)
	// 给接收者发
	if value, ok := s.Clients.Load(message.ReceiveId); ok {
		receiveClient := value.(*UserConn)
		receiveClient.SendBack <- messageBack
	}
	// 给发送者回显 (让发送者的界面也能显示自己刚发的消息)
	if value, ok := s.Clients.Load(message.SendId); ok {
		sendClient := value.(*UserConn)
		sendClient.SendBack <- messageBack
	}

	// 通过注入的缓存服务异步更新缓存
	if s.cacheService != nil {
		s.cacheService.SubmitTask(func() {
			userOneId := message.SendId
			userTwoId := message.ReceiveId
			if userOneId > userTwoId {
				userOneId, userTwoId = userTwoId, userOneId
			}
			key := "message_list_" + userOneId + "_" + userTwoId

			rspString, err := s.cacheService.GetOrError(context.Background(), key)
			if err == nil {
				var list []respond.GetMessageListRespond
				if err := json.Unmarshal([]byte(rspString), &list); err == nil {
					list = append(list, messageRsp)
					if rspByte, err := json.Marshal(list); err == nil {
						_ = s.cacheService.Set(context.Background(), key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
					}
				}
			}
		})
	}
}

// sendToGroup 辅助方法：发送消息给群组
// 1. 构造群组响应 DTO
// 2. 查询群成员列表
// 3. 遍历成员并通过 WebSocket 推送消息 (排除发送者自己)
// 4. 回显给发送者
// 5. 异步更新 Redis 中的群组聊天记录缓存
func (s *StandaloneServer) sendToGroup(message model.Message, originalAvatar string) {
	// 构造群聊响应体
	messageRsp := respond.GetGroupMessageListRespond{
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

	// 序列化
	jsonMessage, err := json.Marshal(messageRsp)
	if err != nil {
		zap.L().Error(err.Error())
	}

	log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

	messageBack := &MessageBack{
		Message: jsonMessage,
		Uuid:    message.Uuid,
	}

	// 通过 Repository 接口查询群成员列表
	var groupMembers []model.GroupMember
	if s.groupMemberRepo != nil {
		var err error
		groupMembers, err = s.groupMemberRepo.FindByGroupUuid(message.ReceiveId)
		if err != nil {
			zap.L().Error("查询群成员失败", zap.Error(err))
		}
	}

	// 分发消息 (sync.Map 自动处理并发安全)
	for _, gm := range groupMembers {
		if gm.UserUuid != message.SendId {
			// 给其他成员推送
			if value, ok := s.Clients.Load(gm.UserUuid); ok {
				receiveClient := value.(*UserConn)
				receiveClient.SendBack <- messageBack
			}
		} else {
			// 给自己(发送者)推送回显
			if value, ok := s.Clients.Load(message.SendId); ok {
				sendClient := value.(*UserConn)
				sendClient.SendBack <- messageBack
			}
		}
	}

	// 通过注入的缓存服务异步更新缓存
	if s.cacheService != nil {
		s.cacheService.SubmitTask(func() {
			key := "group_messagelist_" + message.ReceiveId
			rspString, err := s.cacheService.GetOrError(context.Background(), key)
			if err == nil {
				var list []respond.GetGroupMessageListRespond
				if err := json.Unmarshal([]byte(rspString), &list); err == nil {
					list = append(list, messageRsp)
					if rspByte, err := json.Marshal(list); err == nil {
						_ = s.cacheService.Set(context.Background(), key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
					}
				}
			}
		})
	}
}

// Close 关闭服务通道
func (s *StandaloneServer) Close() {
	close(s.Login)
	close(s.Logout)
	close(s.Transmit)
}

// GetClient 获取客户端
func (s *StandaloneServer) GetClient(userId string) *UserConn {
	value, ok := s.Clients.Load(userId)
	if !ok {
		return nil
	}
	return value.(*UserConn)
}

// Publish 实现 MessageBroker 接口：发布消息到 Channel
func (s *StandaloneServer) Publish(ctx context.Context, msg []byte) error {
	s.Transmit <- msg
	return nil
}

// RegisterClient 实现 MessageBroker 接口：注册客户端
func (s *StandaloneServer) RegisterClient(client *UserConn) {
	s.Login <- client
}

// UnregisterClient 实现 MessageBroker 接口：注销客户端
func (s *StandaloneServer) UnregisterClient(client *UserConn) {
	s.Logout <- client
}

// GetMessageRepo 实现 MessageBroker 接口：获取消息 Repository
func (s *StandaloneServer) GetMessageRepo() mysql.MessageRepository {
	return s.messageRepo
}
