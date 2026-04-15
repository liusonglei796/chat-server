package constants

// ==================== Redis 缓存 Key 前缀 ====================
// 统一使用 `:` 作为分隔符，格式: {模块}:{子模块}:{标识}
const (
	// 用户相关
	CacheKeyUserInfo       = "user:info:"        // 完整用户信息（GetUserInfo）
	CacheKeyUserPublicInfo = "user:public_info:" // 公开用户信息（GetPublicUserInfo）
	CacheKeyUserToken      = "user:token:"       // Refresh Token ID（单点互踢）
	CacheKeySSOToken       = "sso:token:"        // SSO Access Token（单点登录）

	// 群组相关
	CacheKeyGroupInfo      = "group:info:"       // 群组详情
	CacheKeyGroupMembers   = "group:members:"    // 群成员详情列表
	CacheKeyGroupMemberIDs = "group:member_ids:" // 群成员 ID 列表（Kafka 消费者用）

	// 消息相关
	CacheKeyMessageList      = "message:list:"       // 私聊消息列表
	CacheKeyGroupMessageList = "message:group_list:" // 群聊消息列表

	// 会话相关
	CacheKeySessionOpen = "session:open:" // 单个会话缓存（OpenSession）

	// 好友关系（Redis Set）
	CacheKeyFriendRelUser = "friend_relation:user:" // 用户好友关系集合
)
