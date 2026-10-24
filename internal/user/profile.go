package user

import (
	"context"
	"time"

	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/errorx"
)

// UpdateUserInfo 修改用户信息
func (u *UserService) UpdateUserInfo(ctx context.Context, userId string, updateReq UpdateUserInfoRequest) error {
	user, err := u.store.FindByUuid(ctx, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	expectedVersion := user.Version
	if updateReq.Version != nil && *updateReq.Version > 0 {
		if user.Version != *updateReq.Version {
			return errorx.New(errorx.CodeConflict, "用户信息已被修改，请刷新后重试")
		}
		expectedVersion = *updateReq.Version
	}

	if updateReq.Email != nil {
		user.Email = *updateReq.Email
	}
	if updateReq.Nickname != nil {
		user.Nickname = *updateReq.Nickname
	}
	if updateReq.Birthday != nil {
		user.Birthday = *updateReq.Birthday
	}
	if updateReq.Signature != nil {
		user.Signature = *updateReq.Signature
	}
	if updateReq.Avatar != nil {
		user.Avatar = *updateReq.Avatar
	}

	ok, err := u.store.UpdateUserInfoWithVersion(ctx, user, expectedVersion)
	if err != nil {
		zap.L().Error("更新用户信息事务失败", zap.String("userId", userId), zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !ok {
		return errorx.New(errorx.CodeConflict, "用户信息已被修改，请刷新后重试")
	}

	if u.cache != nil {
		u.cache.SubmitTask(func() {
			if u.cacheHelper != nil {
				if err := u.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyUserInfo+userId); err != nil {
					zap.L().Error("清理用户信息缓存失败", zap.Error(err))
				}
				if err := u.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyUserPublicInfo+userId); err != nil {
					zap.L().Error("清理公开用户信息缓存失败", zap.Error(err))
				}
			}
		})
	}

	return nil
}

// GetUserInfo 获取用户完整信息（仅限自己调用）
func (u *UserService) GetUserInfo(ctx context.Context, requesterId, targetId string) (*GetUserInfoRespond, error) {
	if requesterId != targetId {
		return nil, errorx.New(errorx.CodeForbidden, "无权查看他人详细信息")
	}

	if u.cacheHelper == nil {
		user, err := u.store.FindByUuid(ctx, targetId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
			}
			return nil, err
		}
		return &GetUserInfoRespond{
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
			Version:   user.Version,
		}, nil
	}

	key := constants.CacheKeyUserInfo + targetId
	var rsp GetUserInfoRespond

	err := u.cacheHelper.GetOrLoad(
		ctx,
		key,
		func(loaderCtx context.Context) (interface{}, error) {
			user, err := u.store.FindByUuid(loaderCtx, targetId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
				}
				return nil, err
			}
			return GetUserInfoRespond{
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
				Version:   user.Version,
			}, nil
		},
		cacheinfra.RandomizedTTL(time.Hour),
		5*time.Minute,
		&rsp,
	)
	if err != nil {
		return nil, err
	}
	return &rsp, nil
}

// GetPublicUserInfo 获取用户公开信息（查看他人）
func (u *UserService) GetPublicUserInfo(ctx context.Context, targetId string) (*PublicUserInfoRespond, error) {
	if u.cacheHelper == nil {
		user, err := u.store.FindByUuid(ctx, targetId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
			}
			return nil, err
		}
		return &PublicUserInfoRespond{
			Uuid:      user.Uuid,
			Nickname:  user.Nickname,
			Avatar:    user.Avatar,
			Gender:    user.Gender,
			Birthday:  user.Birthday,
			Signature: user.Signature,
		}, nil
	}

	key := constants.CacheKeyUserPublicInfo + targetId
	var rsp PublicUserInfoRespond

	err := u.cacheHelper.GetOrLoad(
		ctx,
		key,
		func(loaderCtx context.Context) (interface{}, error) {
			user, err := u.store.FindByUuid(loaderCtx, targetId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
				}
				return nil, err
			}
			return PublicUserInfoRespond{
				Uuid:      user.Uuid,
				Nickname:  user.Nickname,
				Avatar:    user.Avatar,
				Gender:    user.Gender,
				Birthday:  user.Birthday,
				Signature: user.Signature,
			}, nil
		},
		cacheinfra.RandomizedTTL(30*time.Minute),
		5*time.Minute,
		&rsp,
	)
	if err != nil {
		return nil, err
	}
	return &rsp, nil
}

// BatchGetPublicUserInfo 批量获取公开用户信息
func (u *UserService) BatchGetPublicUserInfo(ctx context.Context, userIds []string) ([]PublicUserInfoRespond, error) {
	if len(userIds) == 0 {
		return []PublicUserInfoRespond{}, nil
	}
	userList, err := u.store.FindByUuids(ctx, userIds)
	if err != nil {
		zap.L().Error("batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	rsp := make([]PublicUserInfoRespond, 0, len(userList))
	for _, usr := range userList {
		rsp = append(rsp, PublicUserInfoRespond{
			Uuid:      usr.Uuid,
			Nickname:  usr.Nickname,
			Avatar:    usr.Avatar,
			Gender:    usr.Gender,
			Birthday:  usr.Birthday,
			Signature: usr.Signature,
		})
	}
	return rsp, nil
}

// GetUserStatus 获取用户账号状态（正常/禁用）
func (u *UserService) GetUserStatus(ctx context.Context, userId string) (int8, error) {
	user, err := u.store.FindByUuid(ctx, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return 0, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		return 0, errorx.ErrServerBusy
	}
	return user.Status, nil
}
