// kafka_consumer.go
// 核心职责：分布式模式下的聊天服务器实现
// 1. 作为 Kafka 消费者，从消息队列读取全量消息
// 2. 维护本机在线用户连接 (Kafka 模式)
// 3. 消息路由：判断消息接收者是否在本机，若在则通过 WebSocket 推送
// 4. 处理复杂的业务逻辑：消息持久化(MySQL)、状态更新、缓存同步(Redis)
package chat

import (
	"encoding/json"
	"fmt"
	dao "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status_enum"
	"kama_chat_server/pkg/enum/message/message_type_enum"
	"kama_chat_server/pkg/util/snowflake"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// MsgConsumer 定义了基于 Kafka 的聊天服务结构
type MsgConsumer struct {
	// Clients 存储所有在线客户端的映射表，Key 为 UserUUID，Value 为 *UserConn
	// 使用 sync.Map 实现并发安全，无需手动加锁
	Clients sync.Map
	// Login 客户端登录通道，当有新连接建立时写入此通道
	Login chan *UserConn
	// Logout 客户端登出通道，当连接断开时写入此通道
	Logout chan *UserConn
}

// GlobalMsgConsumer 全局单例的 MsgConsumer 实例
var GlobalMsgConsumer *MsgConsumer

// kafkaQuit 用于接收系统信号以优雅退出（目前逻辑中使用较少）
var kafkaQuit = make(chan os.Signal, 1)

// InitKafkaServer 初始化 MsgConsumer 单例
func InitKafkaServer() {
	// 确保只初始化一次
	if GlobalMsgConsumer == nil {
		GlobalMsgConsumer = &MsgConsumer{
			// sync.Map 零值即可用，无需显式初始化
			// 初始化登录通道
			Login: make(chan *UserConn),
			// 初始化登出通道
			Logout: make(chan *UserConn),
		}
	}
	//signal.Notify(kafkaQuit, syscall.SIGINT, syscall.SIGTERM)
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
			zap.L().Error(fmt.Sprintf("kafka server panic: %v", r))
		}
		// 关闭通道
		close(k.Login)
		close(k.Logout)
	}()

	// 启动一个 Goroutine 专门负责从 Kafka 读取消息
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
			kafkaMessage, err := GlobalKafkaClient.Consumer.ReadMessage(ctx)
			if err != nil {
				zap.L().Error(err.Error())
				continue // 读取失败，重试
			}
			// 记录详细的 Kafka 消息元数据（调试用）
			log.Printf("topic=%s, partition=%d, offset=%d, key=%s, value=%s", kafkaMessage.Topic, kafkaMessage.Partition, kafkaMessage.Offset, kafkaMessage.Key, kafkaMessage.Value)
			zap.L().Info(fmt.Sprintf("topic=%s, partition=%d, offset=%d, key=%s, value=%s", kafkaMessage.Topic, kafkaMessage.Partition, kafkaMessage.Offset, kafkaMessage.Key, kafkaMessage.Value))

			// 获取消息体
			data := kafkaMessage.Value
			var chatMessageReq request.ChatMessageRequest
			// 反序列化为请求对象
			if err := json.Unmarshal(data, &chatMessageReq); err != nil {
				zap.L().Error(err.Error())
				continue // 反序列化失败，直接跳过
			}
			log.Println("原消息为：", data, "反序列化后为：", chatMessageReq)

			// 根据消息类型分发处理逻辑
			switch chatMessageReq.Type {
			case message_type_enum.Text:
				// 处理文本消息
				k.handleTextMessage(chatMessageReq)
			case message_type_enum.File:
				// 处理文件消息
				k.handleFileMessage(chatMessageReq)
			case message_type_enum.AudioOrVideo:
				// 处理音视频消息
				k.handleAVMessage(chatMessageReq)
			}
		}
	}()

	// 主循环：负责处理客户端的登录和登出事件
	// 这部分逻辑与 Channel 模式的 Server 类似，主要维护内存中的 Clients 映射表
	for {
		select {
		// 处理登录
		case client := <-k.Login:
			// 将新连接的客户端加入映射表 (sync.Map 自动处理并发安全)
			k.Clients.Store(client.Uuid, client)
			zap.L().Debug(fmt.Sprintf("欢迎来到kama聊天服务器，亲爱的用户%s\n", client.Uuid))
			// 发送欢迎语
			if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("欢迎来到kama聊天服务器")); err != nil {
				zap.L().Error(err.Error())
			}

		// 处理退出
		case client := <-k.Logout:
			// 从映射表中移除断开的客户端 (sync.Map 自动处理并发安全)
			k.Clients.Delete(client.Uuid)
			zap.L().Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
			// 发送退出提示
			if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("已退出登录")); err != nil {
				zap.L().Error(err.Error())
			}
		// 处理系统退出信号（如果启用）
		case <-kafkaQuit:
			return
		}
	}
}

// Close 关闭服务通道
func (k *MsgConsumer) Close() {
	close(k.Login)
	close(k.Logout)
}

// SendClientToLogin 将客户端发送到登录通道
// 注意：channel 本身是并发安全的，无需额外加锁
func (k *MsgConsumer) SendClientToLogin(client *UserConn) {
	k.Login <- client
}

// SendClientToLogout 将客户端发送到登出通道
// 注意：channel 本身是并发安全的，无需额外加锁
func (k *MsgConsumer) SendClientToLogout(client *UserConn) {
	k.Logout <- client
}

// GetClient 实现 MessageSender 和 ClientManager 接口
func (k *MsgConsumer) GetClient(userId string) *UserConn {
	value, ok := k.Clients.Load(userId)
	if !ok {
		return nil
	}
	return value.(*UserConn)
}

// handleTextMessage 处理文本消息
// 1. 生成 Snowflake ID
// 2. 将消息持久化到 MySQL
// 3. 根据接收者类型 (User/Group) 路由消息
// 4. 更新 Redis 缓存
func (k *MsgConsumer) handleTextMessage(req request.ChatMessageRequest) {
	// 构建数据库模型
	message := model.Message{
		Uuid:       snowflake.GenerateID(),
		SessionId:  req.SessionId,
		Type:       req.Type,
		Content:    req.Content,
		Url:        "",
		SendId:     req.SendId,
		SendName:   req.SendName,
		SendAvatar: req.SendAvatar,
		ReceiveId:  req.ReceiveId,
		FileSize:   "0B",
		FileType:   "",
		FileName:   "",
		Status:     message_status_enum.Unsent,
		AVdata:     "",
	}
	// 规范化头像路径
	message.SendAvatar = normalizePath(message.SendAvatar)

	// 入库
	if res := dao.GormDB.Create(&message); res.Error != nil {
		zap.L().Error(res.Error.Error())
	}

	// 路由分发
	if message.ReceiveId[0] == 'U' { // 发送给User
		k.sendToUser(message, req.SendAvatar)
	} else if message.ReceiveId[0] == 'G' { // 发送给Group
		k.sendToGroup(message, req.SendAvatar)
	}
}

// handleFileMessage 处理文件消息
// 逻辑与文本消息类似，区别在于 Content 为空，Url 字段存储文件链接
func (k *MsgConsumer) handleFileMessage(req request.ChatMessageRequest) {
	// 构建数据库模型
	message := model.Message{
		Uuid:       snowflake.GenerateID(),
		SessionId:  req.SessionId,
		Type:       req.Type,
		Content:    "",
		Url:        req.Url,
		SendId:     req.SendId,
		SendName:   req.SendName,
		SendAvatar: req.SendAvatar,
		ReceiveId:  req.ReceiveId,
		FileSize:   req.FileSize,
		FileType:   req.FileType,
		FileName:   req.FileName,
		Status:     message_status_enum.Unsent,
		AVdata:     "",
	}
	// 规范化头像路径
	message.SendAvatar = normalizePath(message.SendAvatar)

	// 入库
	if res := dao.GormDB.Create(&message); res.Error != nil {
		zap.L().Error(res.Error.Error())
	}

	// 路由分发
	if message.ReceiveId[0] == 'U' {
		k.sendToUser(message, req.SendAvatar)
	} else {
		k.sendToGroup(message, req.SendAvatar)
	}
}

// handleAVMessage 处理音视频通话信令
// 信令消息（如 start_call）用于 WebRTC 连接建立
// 大部分信令只需透传，特定关键信令（如 PROXY 类型中的 start_call 等）会持久化到数据库
func (k *MsgConsumer) handleAVMessage(req request.ChatMessageRequest) {
	var avData request.AVData
	if err := json.Unmarshal([]byte(req.AVdata), &avData); err != nil {
		zap.L().Error(err.Error())
		return
	}

	// 构建消息模型
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
		AVdata:     req.AVdata,
	}

	// 关键信令入库
	if avData.MessageId == "PROXY" && (avData.Type == "start_call" || avData.Type == "receive_call" || avData.Type == "reject_call") {
		message.SendAvatar = normalizePath(message.SendAvatar)
		if res := dao.GormDB.Create(&message); res.Error != nil {
			zap.L().Error(res.Error.Error())
		}
	}

	// 处理单聊信令转发
	if req.ReceiveId[0] == 'U' {
		// 构造响应
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
		jsonMessage, err := json.Marshal(messageRsp)
		if err != nil {
			zap.L().Error(err.Error())
		}
		log.Println("返回的消息为：", messageRsp)

		messageBack := &MessageBack{
			Message: jsonMessage,
			Uuid:    message.Uuid,
		}

		// 推送给接收者 (sync.Map 自动处理并发安全)
		if value, ok := k.Clients.Load(message.ReceiveId); ok {
			receiveClient := value.(*UserConn)
			receiveClient.SendBack <- messageBack
		}
	}
}

// sendToUser 辅助方法：发送消息给单独用户
// 1. 构造响应 DTO
// 2. 通过 WebSocket 推送给接收者 (如果在线)
// 3. 回显给发送者 (如果在线)
// 4. 异步更新 Redis 中的双人聊天记录缓存
func (k *MsgConsumer) sendToUser(message model.Message, originalAvatar string) {
	// 构造响应体
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

	jsonMessage, err := json.Marshal(messageRsp)
	if err != nil {
		zap.L().Error(err.Error())
	}

	log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

	messageBack := &MessageBack{
		Message: jsonMessage,
		Uuid:    message.Uuid,
	}

	// 消息投递 (sync.Map 自动处理并发安全)
	// 给接收者发
	if value, ok := k.Clients.Load(message.ReceiveId); ok {
		receiveClient := value.(*UserConn)
		receiveClient.SendBack <- messageBack
	}
	// 给发送者回显
	if value, ok := k.Clients.Load(message.SendId); ok {
		sendClient := value.(*UserConn)
		sendClient.SendBack <- messageBack
	}

	// Update Redis async
	go k.updateRedisUser(message, messageRsp)
}

// sendToGroup 辅助方法：发送消息给群组
// 1. 构造群组响应 DTO
// 2. 查询群成员列表
// 3. 遍历成员并通过 WebSocket 推送消息 (排除发送者自己)
// 4. 回显给发送者
// 5. 异步更新 Redis 中的群组聊天记录缓存
func (k *MsgConsumer) sendToGroup(message model.Message, originalAvatar string) {
	// 构造群聊响应
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

	jsonMessage, err := json.Marshal(messageRsp)
	if err != nil {
		zap.L().Error(err.Error())
	}

	log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

	messageBack := &MessageBack{
		Message: jsonMessage,
		Uuid:    message.Uuid,
	}

	// 查成员
	var groupMembers []model.GroupMember
	if res := dao.GormDB.Where("group_uuid = ?", message.ReceiveId).Find(&groupMembers); res.Error != nil {
		zap.L().Error(res.Error.Error())
	}

	// 分发消息 (sync.Map 自动处理并发安全)
	for _, gm := range groupMembers {
		if gm.UserUuid != message.SendId {
			// 推送给其他成员
			if value, ok := k.Clients.Load(gm.UserUuid); ok {
				receiveClient := value.(*UserConn)
				receiveClient.SendBack <- messageBack
			}
		} else {
			// 回显给自己
			if value, ok := k.Clients.Load(message.SendId); ok {
				sendClient := value.(*UserConn)
				sendClient.SendBack <- messageBack
			}
		}
	}

	// Update Redis async
	go k.updateRedisGroup(message, messageRsp)
}

// updateRedisUser 更新用户间聊天记录的 Redis 缓存
func (k *MsgConsumer) updateRedisUser(message model.Message, rsp respond.GetMessageListRespond) {
	key := "message_list_" + message.SendId + "_" + message.ReceiveId
	rspString, err := myredis.GetKeyNilIsErr(key)
	if err == nil {
		var list []respond.GetMessageListRespond
		if err := json.Unmarshal([]byte(rspString), &list); err == nil {
			list = append(list, rsp)
			if rspByte, err := json.Marshal(list); err == nil {
				myredis.SetKeyEx(key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
			}
		}
	}
}

// updateRedisGroup 更新群组聊天记录的 Redis 缓存
func (k *MsgConsumer) updateRedisGroup(message model.Message, rsp respond.GetGroupMessageListRespond) {
	key := "group_messagelist_" + message.ReceiveId
	rspString, err := myredis.GetKeyNilIsErr(key)
	if err == nil {
		var list []respond.GetGroupMessageListRespond
		if err := json.Unmarshal([]byte(rspString), &list); err == nil {
			list = append(list, rsp)
			if rspByte, err := json.Marshal(list); err == nil {
				myredis.SetKeyEx(key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
			}
		}
	}
}
