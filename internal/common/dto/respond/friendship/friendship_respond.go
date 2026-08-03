package friendship

// FriendInfoRespond 好友信息响应
type FriendInfoRespond struct {
	FriendId        string `json:"friend_id"`
	FriendName      string `json:"friend_name"`
	FriendAvatar    string `json:"friend_avatar"`
	FriendPhone     string `json:"friend_phone"`
	FriendEmail     string `json:"friend_email"`
	FriendGender    int8   `json:"friend_gender"`
	FriendSignature string `json:"friend_signature"`
	FriendBirthday  string `json:"friend_birthday"`
	Remark          string `json:"remark"`
}
