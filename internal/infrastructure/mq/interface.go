package mq

import (
	ws "kama_chat_server/internal/gateway/websocket"
)

// MessageSender 消息发送接口
// 用于解耦 MQ 层和 Gateway 层的依赖关系
// MQ 层只需知道"有个东西能发消息"，不需要知道具体实现
type MessageSender interface {
	// SendMessage 向指定用户发送消息
	// userId: 目标用户ID
	// message: 消息内容 (JSON bytes)
	// uuid: 消息唯一标识 (用于状态更新)
	SendMessage(userId string, message []byte, uuid string) error

	// GetClient 获取指定用户的客户端连接 (用于判断是否在线)
	// 返回 nil 表示用户不在线
	GetClient(userId string) *ws.Client

	// GetClients 获取所有在线客户端
	GetClients() map[string]*ws.Client
}

// messageSender 用于存储注入的 MessageSender 实现
var messageSender MessageSender

// SetMessageSender 注入 MessageSender 实现
func SetMessageSender(sender MessageSender) {
	messageSender = sender
}

// GetMessageSender 获取 MessageSender 实现
func GetMessageSender() MessageSender {
	return messageSender
}
