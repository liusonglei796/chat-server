package mysqlinterface

import (
	"context"
	"kama_chat_server/internal/model"
	"time"
)

type UserRepository interface {
	FindByUuid(ctx context.Context, uuid string) (*model.UserInfo, error)
	FindByTelephone(ctx context.Context, telephone string) (*model.UserInfo, error)
	FindByUuids(ctx context.Context, uuids []string) ([]model.UserInfo, error)
	CreateUser(ctx context.Context, user *model.UserInfo) error
	UpdateUserInfo(ctx context.Context, user *model.UserInfo) error
	UpdateUserStatusByUuids(ctx context.Context, uuids []string, status int8) error
	UpdateUserIsAdminByUuids(ctx context.Context, uuids []string, isAdmin int8) error
	SoftDeleteUserByUuids(ctx context.Context, uuids []string) error
	FindAllPaged(ctx context.Context, page, pageSize int, keyword string, status *int8) ([]model.UserInfo, int64, error)
}

type GroupRepository interface {
	FindByUuid(ctx context.Context, uuid string) (*model.GroupInfo, error)
	FindByOwnerIdPaged(ctx context.Context, ownerId string, page, pageSize int) ([]model.GroupInfo, int64, error)
	FindByUuids(ctx context.Context, uuids []string) ([]model.GroupInfo, error)
	GetGroupList(ctx context.Context, page, pageSize int) ([]model.GroupInfo, int64, error)
	CreateGroup(ctx context.Context, group *model.GroupInfo) error
	Update(ctx context.Context, group *model.GroupInfo) error
	UpdateStatusByUuids(ctx context.Context, uuids []string, status int8) error
	IncrementMemberCount(ctx context.Context, uuid string) error
	DecrementMemberCountBy(ctx context.Context, uuid string, count int) error
	SoftDeleteByUuids(ctx context.Context, uuids []string) error
}

type FriendshipRepository interface {
	FindByUserIdAndFriendId(ctx context.Context, userId, friendId string) (*model.Friendship, error)
	FindFriendsByUserId(ctx context.Context, userId string, page, pageSize int) ([]model.Friendship, int64, error)
	CreateFriendship(ctx context.Context, fs *model.Friendship) error
	IsFriend(ctx context.Context, userId1, userId2 string) (bool, error)
	UpdateStatus(ctx context.Context, userId, friendId string, status int8) error
	UpdateRemark(ctx context.Context, userId, friendId, remark string) error
	SoftDelete(ctx context.Context, userId, friendId string) error
	SoftDeleteByUsers(ctx context.Context, userUuids []string) error
}

type SessionRepository interface {
	FindByUuid(ctx context.Context, uuid string) (*model.Session, error)
	FindBySendIdAndReceiveId(ctx context.Context, sendId, receiveId string) (*model.Session, error)
	FindBySendIdAndTypePaged(ctx context.Context, sendId string, receiveIdPrefix string, page, pageSize int) ([]model.Session, int64, error)
	FindBySendIdAndTypeCursor(ctx context.Context, sendId string, receiveIdPrefix, cursor string, pageSize int) (*model.CursorPageSessionResult, error)
	CreateSession(ctx context.Context, session *model.Session) error
	SoftDeleteByUuids(ctx context.Context, uuids []string) error
	SoftDeleteByUsers(ctx context.Context, userUuids []string) error
	UpdatePinStatus(ctx context.Context, uuid string, isPinned bool) error
	UpdateByReceiveId(ctx context.Context, receiveId string, updates map[string]interface{}) error
	UpdateLastMessage(ctx context.Context, sendId, receiveId, content string, msgType int8, msgTime time.Time) error
}

type MessageRepository interface {
	FindByUserIdsPaged(ctx context.Context, userOneId, userTwoId string, page, pageSize int) ([]model.Message, int64, error)
	FindByUserIdsCursor(ctx context.Context, userOneId, userTwoId, cursor string, pageSize int) (*model.CursorPageMessageResult, error)
	FindByGroupIdPaged(ctx context.Context, groupId string, page, pageSize int) ([]model.Message, int64, error)
	FindByGroupIdCursor(ctx context.Context, groupId, cursor string, pageSize int) (*model.CursorPageMessageResult, error)
	FindByUuid(ctx context.Context, uuid string) (*model.Message, error)
	UpdateStatus(ctx context.Context, uuid string, status int8) error
	UpdateContent(ctx context.Context, uuid, content string, msgType int8) error
	Create(ctx context.Context, message *model.Message) error
}

type ApplyRepository interface {
	FindByApplicantIdAndTargetId(ctx context.Context, applicantId, targetId string) (*model.Apply, error)
	FindByTargetIdPendingPaged(ctx context.Context, targetId string, page, pageSize int) ([]model.Apply, int64, error)
	CreateApply(ctx context.Context, apply *model.Apply) error
	Update(ctx context.Context, apply *model.Apply) error
	SoftDelete(ctx context.Context, applicantId, targetId string) error
	SoftDeleteByUsers(ctx context.Context, userUuids []string) error
}

type GroupMemberRepository interface {
	FindByGroupUuid(ctx context.Context, groupUuid string) ([]model.GroupMember, error)
	FindMembersWithUserInfoPaged(ctx context.Context, groupUuid string, page, pageSize int) ([]model.GroupMemberWithUserInfo, int64, error)
	FindByGroupAndUser(ctx context.Context, groupUuid, userUuid string) (*model.GroupMember, error)
	CreateGroupMember(ctx context.Context, member *model.GroupMember) error
	DeleteByGroupUuid(ctx context.Context, groupUuid string) error
	DeleteByUserUuids(ctx context.Context, groupUuid string, userUuids []string) error
	DeleteByGroupUuids(ctx context.Context, groupUuids []string) error
	GetMemberIdsByGroupUuids(ctx context.Context, groupUuids []string) ([]string, error)
	FindGroupUuidsByUserPaged(ctx context.Context, userUuid string, page, pageSize int) ([]string, int64, error)
	UpdateMuteUntil(ctx context.Context, groupUuid, userUuid string, muteUntil *time.Time) error
}
