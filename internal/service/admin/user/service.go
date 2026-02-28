package user

import (
	"context"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	adminreq "kama_chat_server/internal/dto/request/admin"
	adminrsp "kama_chat_server/internal/dto/respond/admin"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
)

// UserAdminService 用户管理后台服务
// 处理用户列表、状态管理、权限管理等管理员功能
type UserAdminService struct {
	repos *mysql.Repositories
	cache myredis.AsyncCacheService
}

// NewUserAdminService 构造函数
func NewUserAdminService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *UserAdminService {
	return &UserAdminService{
		repos: repos,
		cache: cacheService,
	}
}

// GetUserListPaged 分页获取用户列表
func (s *UserAdminService) GetUserListPaged(req adminreq.GetUserListRequest) (*adminrsp.PagedUserListRespond, error) {
	users, total, err := s.repos.User.FindAllPaged(req.Page, req.PageSize, req.Keyword, req.Status)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	list := make([]adminrsp.GetUserListRespond, 0, len(users))
	for _, user := range users {
		list = append(list, adminrsp.GetUserListRespond{
			Uuid:      user.Uuid,
			Telephone: user.Telephone,
			Nickname:  user.Nickname,
			Status:    user.Status,
			IsAdmin:   user.IsAdmin,
			IsDeleted: user.DeletedAt.Valid,
		})
	}

	return &adminrsp.PagedUserListRespond{
		Total: total,
		List:  list,
	}, nil
}

// BatchUpdateUserStatus 批量更新用户状态
// Action: enable(启用), disable(禁用), delete(删除)
func (s *UserAdminService) BatchUpdateUserStatus(req adminreq.BatchUpdateUserStatusRequest) error {
	if len(req.UserUUIDs) == 0 {
		return nil
	}

	switch req.Action {
	case "enable":
		if err := s.repos.User.UpdateUserStatusByUuids(req.UserUUIDs, user_status.NORMAL); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 异步清除用户信息缓存，确保状态变更即时生效
		s.cache.SubmitTask(func() {
			var patterns []string
			for _, uuid := range req.UserUUIDs {
				patterns = append(patterns, constants.CacheKeyUserInfo+uuid)
			}
			if err := s.cache.DeleteByPattern(context.Background(), patterns...); err != nil {
				zap.L().Error("批量清除用户缓存失败", zap.Error(err))
			}
		})

	case "disable":

		err := s.repos.Transaction(func(txRepos *mysql.Repositories) error {
			if err := txRepos.User.UpdateUserStatusByUuids(req.UserUUIDs, user_status.DISABLE); err != nil {
				zap.L().Error("Batch disable users error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			if err := txRepos.Session.SoftDeleteByUsers(req.UserUUIDs); err != nil {
				zap.L().Error("Batch delete sessions error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			return nil
		})
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 异步清除缓存
		s.cache.SubmitTask(func() {
			var patterns []string
			for _, uuid := range req.UserUUIDs {
				patterns = append(patterns,
					constants.CacheKeyUserInfo+uuid,
				)
			}
			if err := s.cache.DeleteByPattern(context.Background(), patterns...); err != nil {
				zap.L().Error("批量清除用户相关缓存失败", zap.Error(err))
			}
		})

	case "delete":
		// 删除用户（事务）
		err := s.repos.Transaction(func(txRepos *mysql.Repositories) error {
			if err := txRepos.User.SoftDeleteUserByUuids(req.UserUUIDs); err != nil {
				zap.L().Error("Batch delete users error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			if err := txRepos.Session.SoftDeleteByUsers(req.UserUUIDs); err != nil {
				zap.L().Error("Batch delete sessions error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			if err := txRepos.Friendship.SoftDeleteByUsers(req.UserUUIDs); err != nil {
				zap.L().Error("Batch delete friendships error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			if err := txRepos.Apply.SoftDeleteByUsers(req.UserUUIDs); err != nil {
				zap.L().Error("Batch delete contact applies error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			return nil
		})
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 异步清除缓存
		s.cache.SubmitTask(func() {
			var patterns []string
			for _, uuid := range req.UserUUIDs {
				patterns = append(patterns,
					constants.CacheKeyUserInfo+uuid,
					constants.CacheKeyFriendRelUser+uuid+"*",
				)
			}
			if err := s.cache.DeleteByPattern(context.Background(), patterns...); err != nil {
				zap.L().Error("批量清除用户相关缓存失败", zap.Error(err))
			}
		})

	default:
		return errorx.New(errorx.CodeInvalidParam, "不支持的操作类型")
	}

	return nil
}

// SetAdmin 设置管理员权限
func (s *UserAdminService) SetAdmin(userUUIDs []string, isAdmin int8) error {
	if len(userUUIDs) == 0 {
		return nil
	}

	// 1. 批量更新管理员状态
	if err := s.repos.User.UpdateUserIsAdminByUuids(userUUIDs, isAdmin); err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 异步批量清除用户信息缓存
	s.cache.SubmitTask(func() {
		var patterns []string
		for _, uuid := range userUUIDs {
			patterns = append(patterns, constants.CacheKeyUserInfo+uuid)
		}
		if err := s.cache.DeleteByPattern(context.Background(), patterns...); err != nil {
			zap.L().Error("批量清除用户缓存失败", zap.Error(err))
		}
	})

	return nil
}
