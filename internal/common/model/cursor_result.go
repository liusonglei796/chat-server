// Package model 定义数据库实体模型和查询结果结构
// 本文件定义游标分页查询结果结构，用于消息列表等场景
package model

// CursorPageMessageResult 游标分页查询结果
// 用于消息列表的游标分页查询
type CursorPageMessageResult struct {
	Messages   []Message // 消息列表
	NextCursor string    // 下一页游标（Unix时间戳字符串）
	HasMore    bool      // 是否还有更多数据
}

// CursorPageSessionResult 游标分页查询结果
// 用于会话列表的游标分页查询
type CursorPageSessionResult struct {
	Sessions   []Session // 会话列表
	NextCursor string    // 下一页游标（Unix时间戳字符串）
	HasMore    bool      // 是否还有更多数据
}
