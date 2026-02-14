package friendship

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	friendshiprsp "kama_chat_server/internal/dto/respond/friendship"
	userrsp "kama_chat_server/internal/dto/respond/user"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	cacheutil "kama_chat_server/pkg/util/cache"
)

// friendshipService 好友关系业务逻辑实现
type friendshipService struct {
	repos       *mysql.Repositories
	cache       myredis.AsyncCacheService
	cacheHelper *cacheutil.Helper
}

// NewFriendshipService 构造函数，注入所有依赖
func NewFriendshipService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *friendshipService {
	return &friendshipService{
		repos:       repos,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
	}
}

// buildUserInfoRespond 将用户模型转换为缓存用的响应结构（DRY: 消除 GetFriendList/GetFriendInfo 的重复转换）
func buildUserInfoRespond(user *model.UserInfo) userrsp.GetUserInfoRespond {
	return userrsp.GetUserInfoRespond{
		Uuid:      user.Uuid,
		Telephone: user.Telephone,
		Nickname:  user.Nickname,
		Avatar:    user.Avatar,
		Birthday:  user.Birthday,
		Email:     user.Email,
		Gender:    user.Gender,
		Signature: user.Signature,
		CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		IsAdmin:   user.IsAdmin,
		Status:    user.Status,
	}
}

// clearFriendRelationCache 清理好友关系相关的缓存（DRY: DeleteFriend/BlackFriend 共用）
func (s *friendshipService) clearFriendRelationCache(userId, friendId string) {
	s.cache.SubmitTask(func() {
		_ = s.cache.RemoveFromSet(context.Background(), constants.CacheKeyFriendRelUser+userId, friendId)
		_ = s.cache.RemoveFromSet(context.Background(), constants.CacheKeyFriendRelUser+friendId, userId)
		_ = s.cache.DeleteByPattern(context.Background(), constants.CacheKeySessionDirect+userId+"*")
		_ = s.cache.DeleteByPattern(context.Background(), constants.CacheKeySessionDirect+friendId+"*")
	})
}

// GetFriendList 获取好友列表（分页）
func (s *friendshipService) GetFriendList(userId string, page, pageSize int) ([]userrsp.MyUserListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 从数据库分页查询好友关系
	friendships, total, err := s.repos.Friendship.FindFriendsByUserId(userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find friendship list error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	if len(friendships) == 0 {
		return []userrsp.MyUserListRespond{}, total, nil
	}

	// 批量获取好友信息（使用 cacheHelper，带 singleflight 防击穿）
	userListRsp := make([]userrsp.MyUserListRespond, 0, len(friendships))
	for _, fs := range friendships {
		cacheKey := constants.CacheKeyUserInfo + fs.FriendId
		friendId := fs.FriendId // 闭包捕获

		var userInfo userrsp.GetUserInfoRespond
		err := s.cacheHelper.GetOrLoad(
			context.Background(),
			cacheKey,
			func() (interface{}, error) {
				user, err := s.repos.User.FindByUuid(friendId)
				if err != nil {
					return nil, err
				}
				return buildUserInfoRespond(user), nil
			},
			cacheutil.RandomizedTTL(5*time.Minute),
			time.Minute,
			&userInfo,
		)
		if err != nil {
			zap.L().Error("Get user info error", zap.String("friendId", friendId), zap.Error(err))
		} else {
			userListRsp = append(userListRsp, userrsp.MyUserListRespond{
				UserId:   userInfo.Uuid,
				UserName: userInfo.Nickname,
				Avatar:   userInfo.Avatar,
			})
		}
	}

	return userListRsp, total, nil
}

// GetFriendInfo 获取好友详情 (userId 必须与 friendId 是好友关系)
func (s *friendshipService) GetFriendInfo(userId, friendId string) (friendshiprsp.FriendInfoRespond, error) {
	if len(friendId) == 0 {
		return friendshiprsp.FriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	isFriend, err := s.repos.Friendship.IsFriend(userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return friendshiprsp.FriendInfoRespond{}, errorx.ErrServerBusy
	}
	if !isFriend {
		return friendshiprsp.FriendInfoRespond{}, errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	cacheKey := constants.CacheKeyUserInfo + friendId
	var userRsp userrsp.GetUserInfoRespond

	err = s.cacheHelper.GetOrLoad(
		context.Background(),
		cacheKey,
		func() (interface{}, error) {
			user, err := s.repos.User.FindByUuid(friendId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
				}
				return nil, err
			}
			if user.Status == user_status.DISABLE {
				return nil, errorx.New(errorx.CodeInvalidParam, "该用户处于禁用状态")
			}
			return buildUserInfoRespond(user), nil
		},
		cacheutil.RandomizedTTL(time.Hour),
		5*time.Minute,
		&userRsp,
	)
	if err != nil {
		return friendshiprsp.FriendInfoRespond{}, err
	}

	// 获取好友备注（从 Friendship 记录中获取）
	var remark string
	fs, err := s.repos.Friendship.FindByUserIdAndFriendId(userId, friendId)
	if err == nil {
		remark = fs.Remark
	}

	return friendshiprsp.FriendInfoRespond{
		FriendId:        userRsp.Uuid,
		FriendName:      userRsp.Nickname,
		FriendAvatar:    userRsp.Avatar,
		FriendBirthday:  userRsp.Birthday,
		FriendEmail:     userRsp.Email,
		FriendPhone:     userRsp.Telephone,
		FriendGender:    userRsp.Gender,
		FriendSignature: userRsp.Signature,
		Remark:          remark,
	}, nil
}

// DeleteFriend 删除好友（双向删除）
func (s *friendshipService) DeleteFriend(userId, friendId string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能删除自己")
	}

	isFriend, err := s.repos.Friendship.IsFriend(userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !isFriend {
		return errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	err = s.repos.Transaction(func(txRepos *mysql.Repositories) error {
		if err := txRepos.Friendship.SoftDelete(userId, friendId); err != nil {
			zap.L().Error("Delete friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 双向删除申请记录（非关键）
		if err := txRepos.Apply.SoftDelete(userId, friendId); err != nil {
			zap.L().Warn("Delete apply record error (non-critical)", zap.Error(err))
		}
		if err := txRepos.Apply.SoftDelete(friendId, userId); err != nil {
			zap.L().Warn("Delete reverse apply record error (non-critical)", zap.Error(err))
		}

		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	s.clearFriendRelationCache(userId, friendId)

	return nil
}

// BlackFriend 拉黑好友
func (s *friendshipService) BlackFriend(userId string, friendId string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能拉黑自己")
	}

	err := s.repos.Transaction(func(txRepos *mysql.Repositories) error {
		myFs, err := txRepos.Friendship.FindByUserIdAndFriendId(userId, friendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeForbidden, "你们还不是好友，无法拉黑")
			}
			zap.L().Error("Find friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if myFs.Status != friendship_status.NORMAL {
			return errorx.New(errorx.CodeInvalidParam, "当前状态不允许拉黑")
		}

		if err := txRepos.Friendship.UpdateStatus(userId, friendId, friendship_status.BLACK); err != nil {
			zap.L().Error("Update status to BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if err := txRepos.Friendship.UpdateStatus(friendId, userId, friendship_status.BE_BLACK); err != nil {
			zap.L().Error("Update status to BE_BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除双方的私聊会话
		sessionsToDelete := make([]string, 0, 2)
		if sess, err := txRepos.Session.FindBySendIdAndReceiveId(userId, friendId); err == nil && sess != nil {
			sessionsToDelete = append(sessionsToDelete, sess.Uuid)
		}
		if sess, err := txRepos.Session.FindBySendIdAndReceiveId(friendId, userId); err == nil && sess != nil {
			sessionsToDelete = append(sessionsToDelete, sess.Uuid)
		}
		if len(sessionsToDelete) > 0 {
			if err := txRepos.Session.SoftDeleteByUuids(sessionsToDelete); err != nil {
				zap.L().Error("拉黑时清理会话失败", zap.Error(err))
				return errorx.ErrServerBusy
			}
		}

		return nil
	})

	if err != nil {
		if e, ok := err.(*errorx.CodeError); ok {
			return e
		}
		zap.L().Error("BlackFriend transaction error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	s.clearFriendRelationCache(userId, friendId)

	return nil
}

// UnblackFriend 取消拉黑好友
func (s *friendshipService) UnblackFriend(userId string, friendId string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "参数错误")
	}

	err := s.repos.Transaction(func(txRepos *mysql.Repositories) error {
		myFs, err := txRepos.Friendship.FindByUserIdAndFriendId(userId, friendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
			zap.L().Error("Find friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if myFs.Status != friendship_status.BLACK {
			return errorx.New(errorx.CodeInvalidParam, "未拉黑该好友，无需解除拉黑")
		}

		theirFs, err := txRepos.Friendship.FindByUserIdAndFriendId(friendId, userId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "对方好友关系不存在")
			}
			zap.L().Error("Find reverse friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if theirFs.Status != friendship_status.BE_BLACK {
			return errorx.New(errorx.CodeInvalidParam, "数据状态异常，请联系管理员")
		}

		if err := txRepos.Friendship.UpdateStatus(userId, friendId, friendship_status.NORMAL); err != nil {
			zap.L().Error("Update friendship status error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if err := txRepos.Friendship.UpdateStatus(friendId, userId, friendship_status.NORMAL); err != nil {
			zap.L().Error("Update reverse friendship status error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		return nil
	})

	if err != nil {
		if e, ok := err.(*errorx.CodeError); ok {
			return e
		}
		zap.L().Error("UnblackFriend transaction error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	s.cache.SubmitTask(func() {
		_ = s.cache.AddToSet(context.Background(), constants.CacheKeyFriendRelUser+userId, friendId)
		_ = s.cache.AddToSet(context.Background(), constants.CacheKeyFriendRelUser+friendId, userId)
	})

	return nil
}

// UpdateRemark 更新好友备注
func (s *friendshipService) UpdateRemark(userId, friendId, remark string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能给自己设置备注")
	}

	// 校验好友关系
	isFriend, err := s.repos.Friendship.IsFriend(userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !isFriend {
		return errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	if err := s.repos.Friendship.UpdateRemark(userId, friendId, remark); err != nil {
		zap.L().Error("Update remark error",
			zap.String("user_id", userId),
			zap.String("friend_id", friendId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	return nil
}
