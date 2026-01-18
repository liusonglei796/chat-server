package contact_apply_status_enum

// 好友/入群申请状态
// PENDING: 待处理
// AGREE: 已同意
// REFUSE: 已拒绝
// BLACK: 拉黑申请人
const (
	PENDING = iota
	AGREE
	REFUSE
	BLACK
)
