package group

import (
	"context"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	adminreq "kama_chat_server/internal/dto/request/admin"
	adminrsp "kama_chat_server/internal/dto/respond/admin"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/errorx"
)

// groupAdminService 群组管理后台服务
// 处理群组列表、批量删除、状态管理等管理员功能
type groupAdminService struct {
	repos *mysql.Repositories
	cache myredis.AsyncCacheService
}

// NewGroupAdminService 构造函数
func NewGroupAdminService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *groupAdminService {
	return &groupAdminService{
		repos: repos,
		cache: cacheService,
	}
}

// GetGroupInfoList 分页获取群组列表
func (s *groupAdminService) GetGroupInfoList(req adminreq.GetGroupInfoListRequest) (*adminrsp.GetGroupListWrapper, error) {
	groupList, total, err := s.repos.Group.GetGroupList(req.Page, req.PageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	rsp := make([]adminrsp.GetGroupListRespond, 0, len(groupList))
	for _, group := range groupList {
		rp := adminrsp.GetGroupListRespond{
			Uuid:      group.Uuid,
			Name:      group.Name,
			OwnerId:   group.OwnerId,
			Status:    group.Status,
			IsDeleted: group.DeletedAt.Valid,
		}
		rsp = append(rsp, rp)
	}

	return &adminrsp.GetGroupListWrapper{
		List:  rsp,
		Total: total,
	}, nil
}

// DeleteGroups 批量删除群组
func (s *groupAdminService) DeleteGroups(groupUUIDs []string) error {
	if len(groupUUIDs) == 0 {
		return nil
	}

	// 1. 收集需要清理缓存的成员ID
	memberIds, err := s.repos.GroupMember.GetMemberIdsByGroupUuids(groupUUIDs)
	if err != nil {
		zap.L().Error("Find group members error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 事务执行删除操作
	err = s.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 删除群成员
		if err := txRepos.GroupMember.DeleteByGroupUuids(groupUUIDs); err != nil {
			zap.L().Error("Batch delete group members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除群组
		if err := txRepos.Group.SoftDeleteByUuids(groupUUIDs); err != nil {
			zap.L().Error("Batch soft delete groups error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除相关会话
		if err := txRepos.Session.SoftDeleteByUsers(groupUUIDs); err != nil {
			zap.L().Error("Batch soft delete sessions error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除相关联系人
		if err := txRepos.Contact.SoftDeleteByUsers(groupUUIDs); err != nil {
			zap.L().Error("Batch soft delete contacts error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除相关申请
		if err := txRepos.Apply.SoftDeleteByUsers(groupUUIDs); err != nil {
			zap.L().Error("Batch soft delete contact applies error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 3. 异步清理缓存
	s.cache.SubmitTask(func() {
		// 清理所有相关成员的缓存
		for _, memId := range memberIds {
			if err := s.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memId+"*"); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
			if err := s.cache.DeleteByPattern(context.Background(), "group_session_list_"+memId+"*"); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
		}

		// 清理群本身的缓存
		for _, grpId := range groupUUIDs {
			if err := s.cache.Delete(context.Background(), "group_info_"+grpId); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
			if err := s.cache.Delete(context.Background(), "group_memberlist_"+grpId); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
		}
	})

	return nil
}

// SetGroupsStatus 批量设置群组状态
func (s *groupAdminService) SetGroupsStatus(groupUUIDs []string, status int8) error {
	if len(groupUUIDs) == 0 {
		return nil
	}

	if status == group_status_enum.DISABLE {
		// 禁用群组（事务：更新状态 + 删除会话）
		err := s.repos.Transaction(func(txRepos *mysql.Repositories) error {
			if err := txRepos.Group.UpdateStatusByUuids(groupUUIDs, status); err != nil {
				zap.L().Error("Batch update group status error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			if err := txRepos.Session.SoftDeleteByUsers(groupUUIDs); err != nil {
				zap.L().Error("Batch delete sessions error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			return nil
		})
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
	} else {
		// 非禁用操作，直接更新状态
		if err := s.repos.Group.UpdateStatusByUuids(groupUUIDs, status); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
	}

	s.cache.SubmitTask(func() {
		var patterns []string
		for _, uuid := range groupUUIDs {
			patterns = append(patterns, "group_info_"+uuid)
		}
		if err := s.cache.DeleteByPatterns(context.Background(), patterns); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
	})

	return nil
}
