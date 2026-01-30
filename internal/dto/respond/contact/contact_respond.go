package contact

// FriendInfoRespond 好友信息响应
// 使用位置:
//   - internal/service/contact/service.go: GetFriendInfo
type FriendInfoRespond struct {
	FriendId        string `json:"friend_id"`
	FriendName      string `json:"friend_name"`
	FriendAvatar    string `json:"friend_avatar"`
	FriendPhone     string `json:"friend_phone"`
	FriendEmail     string `json:"friend_email"`
	FriendGender    int8   `json:"friend_gender"`
	FriendSignature string `json:"friend_signature"`
	FriendBirthday  string `json:"friend_birthday"`
}

// GroupDetailRespond 群聊详情响应
// 使用位置:
//   - internal/service/contact/service.go: GetGroupDetail
type GroupDetailRespond struct {
	GroupId     string `json:"group_id"`
	GroupName   string `json:"group_name"`
	GroupAvatar string `json:"group_avatar"`
	GroupNotice string `json:"group_notice"`
	MemberCnt   int    `json:"member_cnt"`
	OwnerId     string `json:"owner_id"`
	AddMode     int8   `json:"add_mode"`
}
