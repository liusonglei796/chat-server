package apply

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	applyreq "kama_chat_server/internal/dto/request/apply"
	applyrsp "kama_chat_server/internal/dto/respond/apply"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/contact/contact_type_enum"
	"kama_chat_server/pkg/enum/contact_apply/contact_apply_status_enum"
	"kama_chat_server/pkg/enum/group_info/add_mode_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
)

// applyService 申请业务逻辑实现
// 设计原则说明：
//  1. 读操作（如获取申请列表）：采用【直接查询数据库】模式。
//     原因：申请/审批流程对数据一致性要求极高（防脏读、防重复），且相比消息/联系列表，其访问频率较低，直查DB可确保逻辑安全且性能可控。
//  2. 写操作（如发起/通过申请）：采用【写库成功后删除缓存】模式。
//     原因：作为数据生产者，在变更状态后主动失效下游服务（如ContactService）的缓存，保证系统数据的最终一致性。
type applyService struct {
	repos *mysql.Repositories
	cache myredis.AsyncCacheService
}

// NewApplyService 构造函数
func NewApplyService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *applyService {
	return &applyService{
		repos: repos,
		cache: cacheService,
	}
}

// ApplyFriend 申请添加好友
// ApplyFriend 申请添加好友
// userId: 发起申请的用户ID
// req: 包含目标好友ID和申请信息的请求对象
func (u *applyService) ApplyFriend(userId string, req applyreq.ApplyFriendRequest) error {
	// 1. 参数校验
	// 校验好友ID是否为空，如果为空则返回参数无效错误，防止后续空指针或逻辑错误
	if len(req.FriendId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	// 2. 检查目标用户是否存在
	// 调用用户仓库查找目标用户信息，防止向不存在的用户发送申请
	user, err := u.repos.User.FindByUuid(req.FriendId)
	if err != nil {
		// 如果错误是记录未找到，则返回用户不存在的业务错误
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		// 其他数据库错误则返回服务器繁忙
		return errorx.ErrServerBusy
	}

	// 3. 检查目标用户状态
	// 检查目标用户账号状态是否为禁用，防止与已被封禁或注销的用户建立关系
	if user.Status == user_status_enum.DISABLE {
		return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
	}

	// 4. 检查是否已经是好友
	// 查询联系表，确认双方是否已经存在正常的好友关系
	// 避免重复添加好友，防止产生脏数据和冗余记录
	relation, err := u.repos.Contact.FindByUserIdAndContactId(userId, req.FriendId, contact_type_enum.USER)
	// 如果查询成功且关系存在，并且状态为正常，则提示已经是好友
	if err == nil && relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你们已经是好友")
	}

	// 5. 查询之前的申请记录
	// 查询是否已经存在申请记录（无论当前状态是待处理、已拒绝还是黑名单）
	// 这用于判断是创建新申请、更新旧申请，还是因为黑名单而被拦截
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(userId, req.FriendId)
	if err != nil {
		// 如果错误是未找到记录，说明是首次申请
		if errorx.IsNotFound(err) {
			// 初始化一个新的申请模型对象
			apply = &model.Apply{
				Uuid:        fmt.Sprintf("A%s", snowflake.GenerateIDString()), // 生成唯一的申请记录UUID
				ApplicantId: userId,                                           // 设置申请人ID
				TargetId:    req.FriendId,                                     // 设置目标用户ID
				ContactType: contact_type_enum.USER,                           // 设置联系类型为用户
				Status:      contact_apply_status_enum.PENDING,                // 初始状态为待处理
				Message:     req.Message,                                      // 设置申请附带的留言信息
				LastApplyAt: time.Now(),                                       // 设置申请时间为当前时间
			}
			// 保存新的申请记录到数据库
			if err := u.repos.Apply.CreateApply(apply); err != nil {
				// 如果保存失败，记录日志并返回服务器繁忙错误
				zap.L().Error("Create friend apply error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			// 新申请创建成功，直接返回nil
			return nil
		}
		// 如果查询过程出现其他数据库错误，记录日志并返回
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 6. 黑名单检查
	// 如果已存在的申请记录状态为 BLACK，说明对方已将申请人拉黑
	// 直接返回错误，防止被拉黑用户持续骚扰
	if apply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑，无法发送申请")
	}

	// 7. 更新旧申请记录
	// 如果存在旧申请且未被拉黑，则复用该记录
	// 更新申请时间为当前时间
	apply.LastApplyAt = time.Now()
	// 重置申请状态为待处理（PENDING），以便对方重新审核
	apply.Status = contact_apply_status_enum.PENDING
	// 更新最新的留言信息
	apply.Message = req.Message

	// 将更新后的申请记录保存到数据库
	if err := u.repos.Apply.Update(apply); err != nil {
		// 如果更新失败，记录日志并返回错误
		zap.L().Error("Update friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 申请处理成功，返回nil
	return nil
}

// ApplyGroup 申请加入群组
// ApplyGroup 申请加入群组的逻辑处理
// userId: 申请人的用户ID
// req: 包含目标群组ID和申请信息的请求对象
func (u *applyService) ApplyGroup(userId string, req applyreq.ApplyGroupRequest) error {
	// 1. 参数校验
	// 检查群组ID是否为空
	if len(req.GroupId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "群组ID不能为空")
	}

	// 2. 检查群组是否存在
	// 调用群组仓库查找目标群组
	// 防止申请加入不存在的群组
	group, err := u.repos.Group.FindByUuid(req.GroupId)
	if err != nil {
		// 如果未找到群组，返回群聊不存在的错误
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "该群聊不存在")
		}
		// 其他错误则返回服务器繁忙
		return errorx.ErrServerBusy
	}
	// 3. 检查群组状态
	// 防止加入已被封禁或解散的群组
	if group.Status == group_status_enum.DISABLE {
		return errorx.New(errorx.CodeInvalidParam, "该群聊已被禁用")
	}

	// 4. 检查是否已经是群成员
	// 查询联系表（群组也作为一种联系类型），确认是否已经在群中
	// 防止重复加群
	relation, err := u.repos.Contact.FindByUserIdAndContactId(userId, req.GroupId, contact_type_enum.GROUP)
	if err != nil {
		// 如果查询出错且不是未找到错误，记录日志并返回
		if !errorx.IsNotFound(err) {
			zap.L().Error("Find contact relation error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 如果是未找到，说明还不是成员，relation置为nil
		relation = nil
	}
	// 如果关系存在且状态正常，说明已经是群成员
	if relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你已在该群中")
	}

	// 5. 查询之前的申请记录
	// 核心修复：先查询申请记录，检查黑名单
	// 这里查询是为了后续判断是否被拉黑，以及复用已有记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(userId, req.GroupId)
	if err != nil {
		// 如果查询出错且不是未找到错误，记录日志并返回
		if !errorx.IsNotFound(err) {
			zap.L().Error("Find group apply error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 如果未找到记录，apply置为nil
		apply = nil
	}

	// 6. 黑名单检查
	// 无论为何种加群模式（包括免审核），如果处于黑名单中，都必须拒绝
	// 防止黑名单用户通过直接入群模式绕过限制
	if apply != nil && apply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "该群已将你拉黑，无法发送申请")
	}

	// 7. 处理免审核入群逻辑
	// 如果群组设置了免审核模式 (DIRECT)，则直接处理入群逻辑
	if group.AddMode == add_mode_enum.DIRECT {
		// 使用事务保证入群操作的原子性：
		// 1. 创建群成员记录
		// 2. 增加群成员计数
		// 3. 创建联系记录 (User -> Group)
		// 4. 清理旧的申请记录（如果有）
		err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
			// 创建群成员对象，Role默认为1（普通成员）
			member := model.GroupMember{
				GroupUuid: req.GroupId,
				UserUuid:  userId,
				Role:      1,
			}
			// 保存群成员记录
			if err := txRepos.GroupMember.CreateGroupMember(&member); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			// 增加群成员数量
			if err := txRepos.Group.IncrementMemberCount(req.GroupId); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			// 创建用户的联系记录（将群组作为联系）
			newContact := model.Contact{
				UserId:      userId,
				ContactId:   req.GroupId,
				ContactType: contact_type_enum.GROUP,
				Status:      contact_status_enum.NORMAL,
			}
			// 保存联系记录
			if err := txRepos.Contact.CreateContact(&newContact); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			// 如果存在之前的申请记录（非黑名单），则将其软删除，避免残留无效数据
			if apply != nil {
				_ = txRepos.Apply.SoftDelete(userId, req.GroupId)
			}
			return nil
		})

		// 如果事务执行失败，返回服务器繁忙
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 8. 异步更新缓存
		// 删除相关会话缓存、联系列表缓存、群组信息缓存，确保数据一致性
		// 客户端下次请求时将重新加载最新数据
		u.cache.SubmitTask(func() {
			_ = u.cache.DeleteByPattern(context.Background(), "group_session_list_"+userId+"*")
			_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+userId+"*")
			_ = u.cache.Delete(context.Background(), "group_info_"+req.GroupId)
			_ = u.cache.Delete(context.Background(), "group_memberlist_"+req.GroupId)
		})

		return nil
	}

	// 9. 处理需要审核的入群逻辑
	// 如果不是免审核模式，则创建或更新申请记录
	if apply == nil {
		// 情况A：没有旧申请，创建新申请
		apply = &model.Apply{
			Uuid:        fmt.Sprintf("A%s", snowflake.GenerateIDString()),
			ApplicantId: userId,
			TargetId:    req.GroupId,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_apply_status_enum.PENDING, // 初始状态为待处理
			Message:     req.Message,
			LastApplyAt: time.Now(),
		}
		// 保存新申请
		if err := u.repos.Apply.CreateApply(apply); err != nil {
			zap.L().Error("Create group apply error", zap.Error(err))
			return errorx.ErrServerBusy
		}
	} else {
		// 情况B：存在旧申请，更新其状态和信息
		apply.LastApplyAt = time.Now()
		apply.Status = contact_apply_status_enum.PENDING // 重置为待处理
		apply.Message = req.Message
		// 更新旧申请
		if err := u.repos.Apply.Update(apply); err != nil {
			zap.L().Error("Update group apply error", zap.Error(err))
			return errorx.ErrServerBusy
		}
	}

	return nil
}

// GetFriendApplyList 获取收到的好友申请列表
// GetFriendApplyList 获取收到的好友申请列表（分页）
// userId: 当前用户的ID
// page: 页码，从1开始
// pageSize: 每页数量
// 返回: 分页好友申请列表响应对象
func (u *applyService) GetFriendApplyList(userId string, page, pageSize int) (*applyrsp.PagedFriendApplyListRespond, error) {
	// 1. 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 2. 数据库分页查询待处理的申请记录
	applyList, total, err := u.repos.Apply.FindByTargetIdPendingPaged(userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 如果没有申请，直接返回空切片
	if total == 0 || len(applyList) == 0 {
		return &applyrsp.PagedFriendApplyListRespond{
			Total: total,
			List:  []applyrsp.FriendApplyListRespond{},
		}, nil
	}

	// 3. 收集当前页申请人ID
	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	// 4. 批量查询申请人信息
	// 根据收集到的UUID列表，一次性查询所有用户信息，避免N+1查询问题
	userList, err := u.repos.User.FindByUuids(userUuids)
	if err != nil {
		zap.L().Error("Batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 5. 构建用户信息Map
	// 将用户列表转换为Map，Key为UUID，Value为用户信息
	// 这样做是为了后续遍历申请列表时能够以O(1)复杂度获取对应用户信息
	userMap := make(map[string]model.UserInfo)
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	// 6. 组装响应数据
	rsp := make([]applyrsp.FriendApplyListRespond, 0, len(applyList))
	for _, apply := range applyList {
		// 从Map中获取申请人详细信息
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			// 理论上不应发生（外键约束或逻辑保证），如果发生则跳过该条记录
			continue
		}

		// 处理申请理由，如果为空则显示默认文本
		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}

		// 构建并追加响应对象
		rsp = append(rsp, applyrsp.FriendApplyListRespond{
			ApplicantId:     user.Uuid,
			ApplicantName:   user.Nickname,
			ApplicantAvatar: user.Avatar,
			Message:         message,
		})
	}

	return &applyrsp.PagedFriendApplyListRespond{
		Total: total,
		List:  rsp,
	}, nil
}

// GetGroupApplyList 获取收到的加群申请列表
// GetGroupApplyList 获取收到的加群申请列表
// userId: 操作者ID（必须是管理员或群主）
// groupId: 目标群组ID
func (u *applyService) GetGroupApplyList(userId, groupId string, page, pageSize int) (*applyrsp.PagedGroupApplyListRespond, error) {
	// 1. 权限校验
	// 检查操作者是否是该群的成员
	member, err := u.repos.GroupMember.FindByGroupAndUser(groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	// Role: 1=普通成员, 2=管理员, 3=群主
	// 只有管理员(2)及以上权限才能查看申请列表
	if member.Role < 2 {
		return nil, errorx.New(errorx.CodeForbidden, "你没有查看申请列表的权限")
	}

	// 2. 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 3. 数据库分页查询待处理的入群申请
	applyList, total, err := u.repos.Apply.FindByTargetIdPendingPaged(groupId, page, pageSize)
	if err != nil {
		zap.L().Error("Find group pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	if total == 0 || len(applyList) == 0 {
		return &applyrsp.PagedGroupApplyListRespond{
			Total: total,
			List:  []applyrsp.GroupApplyListRespond{},
		}, nil
	}

	// 4. 收集当前页申请人ID
	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	// 5. 批量查询申请人信息
	userList, err := u.repos.User.FindByUuids(userUuids)
	if err != nil {
		zap.L().Error("Batch find users info error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 6. 构建用户信息Map
	userMap := make(map[string]model.UserInfo)
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	// 7. 组装响应数据
	rsp := make([]applyrsp.GroupApplyListRespond, 0, len(applyList))
	for _, apply := range applyList {
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			continue
		}

		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}

		rsp = append(rsp, applyrsp.GroupApplyListRespond{
			ApplicantId:     user.Uuid,
			ApplicantName:   user.Nickname,
			ApplicantAvatar: user.Avatar,
			Message:         message,
		})
	}

	return &applyrsp.PagedGroupApplyListRespond{
		Total: total,
		List:  rsp,
	}, nil
}

// PassFriendApply 通过好友申请
// PassFriendApply 通过好友申请
// userId: 操作者ID（即被申请人）
// applicantId: 申请人ID（即发起好友申请的用户）
func (u *applyService) PassFriendApply(userId string, applicantId string) error {
	// 1. 查询申请记录
	// 确认申请是否存在，以及是否是指向当前用户的申请
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 开启事务
	// 建立好友关系涉及多张表的更新（更新申请状态、双方各创建一条联系记录）
	// 使用事务保证操作的原子性，要么全部成功，要么全部回滚
	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 3. 查询申请人信息并校验状态
		user, err := txRepos.User.FindByUuid(applicantId)
		if err != nil {
			zap.L().Error("Find user error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 再次检查用户状态：虽然申请时已校验，但用户可能在申请后、审批前被禁用
		// 防止审批通过时的竞态条件（Race Condition），确保不与禁用用户建立关系
		if user.Status == user_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
		}

		// 4. 更新申请状态
		// 将申请状态更新为已同意（AGREE）
		apply.Status = contact_apply_status_enum.AGREE
		if err := txRepos.Apply.Update(apply); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 5. 建立双向好友关系
		// 5.1 创建我方好友关系 (Me -> Friend)
		newContact := model.Contact{
			UserId:      userId,
			ContactId:   applicantId,
			ContactType: contact_type_enum.USER,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&newContact); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 5.2 创建对方好友关系 (Friend -> Me) - 双向好友
		anotherContact := model.Contact{
			UserId:      applicantId,
			ContactId:   userId,
			ContactType: contact_type_enum.USER,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&anotherContact); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 6. 异步清除缓存
	// 异步清除双方的联系列表缓存，确保双方下次请求联系列表时能获取到最新的好友关系
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+applicantId)
	})

	return nil
}

// PassGroupApply 通过入群申请
// operatorId 必须是群主或管理员
// PassGroupApply 通过入群申请
// operatorId: 操作者ID（必须是群主或管理员）
// groupId: 目标群组ID
// applicantId: 申请入群的用户ID
func (u *applyService) PassGroupApply(operatorId, groupId, applicantId string) error {
	// 1. 权限校验
	// 查询操作者在群组中的角色
	// 防止普通成员或其他未经授权的用户审批入群申请
	member, err := u.repos.GroupMember.FindByGroupAndUser(groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	// 校验权限：Role: 1=普通成员, 2=管理员, 3=群主
	if member.Role < 2 { // Role: 1=普通成员, 2=管理员, 3=群主
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

	// 2. 查找申请记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 3. 开启事务
	// 保证入群操作（更新申请、添加成员、增加计数、添加联系）的原子性
	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 3.1 获取并校验群组信息
		group, err := txRepos.Group.FindByUuid(groupId)
		if err != nil {
			zap.L().Error("Find group error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 再次检查群聊状态：虽然申请时已校验，但群聊可能在申请后、审批前被禁用
		// 防止审批通过时的竞态条件
		if group.Status == group_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该群聊已被禁用")
		}

		// 3.2 更新申请状态为 AGREED
		apply.Status = contact_apply_status_enum.AGREE

		if err := txRepos.Apply.Update(apply); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 3.3 创建联系关系 (User -> Group)
		// 即使是群组，在联系表中也有一条记录
		newContact := model.Contact{
			UserId:      applicantId,
			ContactId:   groupId,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&newContact); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 3.4 创建群成员记录
		// Role默认为1（普通成员）
		newMember := model.GroupMember{
			GroupUuid: groupId,
			UserUuid:  applicantId,
			Role:      1,
		}
		if err := txRepos.GroupMember.CreateGroupMember(&newMember); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 3.5 增加群成员计数
		if err := txRepos.Group.IncrementMemberCount(groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 4. 异步更新缓存
	// 删除用户的群会话缓存、联系列表缓存
	// 以及群组信息和成员列表缓存，确保所有相关方都能获取最新数据
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "group_session_list_"+applicantId+"*")
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+applicantId+"*")
		_ = u.cache.Delete(context.Background(), "group_info_"+groupId)
		_ = u.cache.Delete(context.Background(), "group_memberlist_"+groupId)
	})

	return nil
}

// RefuseFriendApply 拒绝好友申请
// userId: 操作者ID
// applicantId: 申请人ID
func (u *applyService) RefuseFriendApply(userId string, applicantId string) error {
	// 1. 查找申请记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	// 2. 更新状态为 REFUSE
	apply.Status = contact_apply_status_enum.REFUSE
	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// RefuseGroupApply 拒绝入群申请
// operatorId: 操作者ID（必须是群主或管理员）
// groupId: 目标群组ID
// applicantId: 申请入群的用户ID
func (u *applyService) RefuseGroupApply(operatorId, groupId, applicantId string) error {
	// 1. 权限校验
	// 查询操作者权限
	member, err := u.repos.GroupMember.FindByGroupAndUser(groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	// 校验权限：Role: 1=普通成员, 2=管理员, 3=群主
	// 只有管理员及以上权限才能审批
	if member.Role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

	// 2. 查找申请记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 3. 更新状态为 REFUSE
	apply.Status = contact_apply_status_enum.REFUSE
	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackFriendApply 拉黑好友申请
// userId: 操作者ID
// applicantId: 申请人ID
func (u *applyService) BlackFriendApply(userId string, applicantId string) error {
	// 1. 查找申请记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 更新状态为 BLACK
	// 拉黑后，对方将无法再次发送申请（ApplyFriend会在检查黑名单时拦截）
	apply.Status = contact_apply_status_enum.BLACK
	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update friend apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackGroupApply 拉黑入群申请
// operatorId: 操作者ID（必须是群主或管理员）
// groupId: 目标群组ID
// applicantId: 申请入群的用户ID
func (u *applyService) BlackGroupApply(operatorId, groupId, applicantId string) error {
	// 1. 权限校验
	member, err := u.repos.GroupMember.FindByGroupAndUser(groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	// 校验权限
	if member.Role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

	// 2. 查找申请记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 3. 更新状态为 BLACK
	// 拉黑后，对方将无法再次申请入群
	apply.Status = contact_apply_status_enum.BLACK
	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update group apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}
