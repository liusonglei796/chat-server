package contact_status_enum

// 联系人/群组关系状态
// NORMAL: 正常关系
// BE_BLACK: 我被对方拉黑
// BLACK: 我拉黑对方
// BE_DELETE: 我被对方删除
// DELETE: 我删除对方
// SILENCE: 被禁言（群组）
// QUIT_GROUP: 主动退群
// KICK_OUT_GROUP: 被踢出群
const (
	NORMAL = iota
	BE_BLACK
	BLACK
	BE_DELETE
	DELETE
	SILENCE
	QUIT_GROUP
	KICK_OUT_GROUP
)
