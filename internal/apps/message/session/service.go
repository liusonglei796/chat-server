package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grouppb "kama_chat_server/api/gen/group"
	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/common/domain/store"
	grouprsp "kama_chat_server/internal/common/dto/respond/group"
	sessionreq "kama_chat_server/internal/common/dto/request/session"
	sessionrsp "kama_chat_server/internal/common/dto/respond/session"
	userrsp "kama_chat_server/internal/common/dto/respond/user"
	"kama_chat_server/internal/common/grpc_client"
	cacheutil "kama_chat_server/internal/common/infrastructure/cache"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/group/group_status"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
)

// SessionService 会话业务逻辑实现
// 通过构造函数注入 Store 和 Cache 依赖
type SessionService struct {
	sessionStore store.SessionStore
	messageStore store.MessageStore
	cache       store.AsyncCacheService
	cacheHelper *cacheutil.Helper // 缓存辅助工具（带 singleflight）
}

// NewSessionService 构造函数，注入所有依赖
func NewSessionService(
	sessionStore store.SessionStore,
	messageStore store.MessageStore,
	cacheService store.AsyncCacheService,
) *SessionService {
	return &SessionService{
		sessionStore: sessionStore,
		messageStore: messageStore,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
	}
}

// CreateSession 创建会话
func (s *SessionService) CreateSession(ctx context.Context, sendId, receiveId string) (string, error) {
	// 1. 幂等性检查：先查询是否已存在会话
	existingSession, err := s.sessionStore.FindBySendIdAndReceiveId(ctx, sendId, receiveId)
	if err != nil {
		// 如果不是"未找到"错误，则返回数据库错误
		if errorx.GetCode(err) != errorx.CodeNotFound {
			zap.L().Error("查询已有会话失败",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
				zap.Error(err),
			)
			return "", errorx.ErrServerBusy
		}
		// 未找到会话，继续创建新会话
	} else {
		// 会话已存在，直接返回已有会话ID
		zap.L().Info("会话已存在，返回已有会话",
			zap.String("send_id", sendId),
			zap.String("receive_id", receiveId),
			zap.String("session_id", existingSession.Uuid),
		)
		return existingSession.Uuid, nil
	}

	// 2. 验证发送者是否存在与状态
	sendStatus, err := grpc_client.GetUserStatus(ctx, sendId)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			zap.L().Warn("发送用户不存在",
				zap.String("send_id", sendId),
				zap.String("operation", "create_session"),
			)
			return "", errorx.New(errorx.CodeUserNotExist, "发送用户不存在")
		}
		zap.L().Error("查询发送用户失败",
			zap.String("send_id", sendId),
			zap.Error(err),
		)
		return "", errorx.ErrServerBusy
	}
	if sendStatus == user_status.DISABLE {
		zap.L().Warn("发送用户已被禁用",
			zap.String("send_id", sendId),
			zap.String("operation", "create_session"),
		)
		return "", errorx.New(errorx.CodeUserNotExist, "发送用户不存在")
	}

	// 3. 构建会话基础信息
	var session model.Session
	session.Uuid = fmt.Sprintf("S%s", snowflake.GenerateIDString())
	session.SendId = sendId
	session.ReceiveId = receiveId
	session.CreatedAt = time.Now()

	// 4. 根据接收者类型设置会话信息
	if receiveId[0] == 'U' {
		// 用户对用户会话
		recvStatus, err := grpc_client.GetUserStatus(ctx, receiveId)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				zap.L().Warn("接收用户不存在",
					zap.String("send_id", sendId),
					zap.String("receive_id", receiveId),
					zap.String("operation", "create_session"),
				)
				return "", errorx.New(errorx.CodeUserNotExist, "接收用户不存在")
			}
			zap.L().Error("查询接收用户失败",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
				zap.Error(err),
			)
			return "", errorx.ErrServerBusy
		}
		if recvStatus == user_status.DISABLE {
			zap.L().Warn("接收用户已被禁用",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
			)
			return "", errorx.New(errorx.CodeInvalidParam, "该用户被禁用了")
		}
		// 验证好友关系 (必须是好友才能发起会话)
		isFriend, err := grpc_client.CheckFriendshipStatus(ctx, sendId, receiveId)
		if err != nil {
			zap.L().Error("Check friend relationship error", zap.Error(err))
			return "", errorx.ErrServerBusy
		}
		if isFriend != 1 {
			return "", errorx.New(errorx.CodeForbidden, "你们还不是好友")
		}
	} else {
		// 用户对群组会话
		_, err := grpc_client.GetGroupDetail(ctx, sendId, receiveId)
		if err != nil {
			zap.L().Error("query group via grpc error", zap.Error(err))
			return "", err
		}
	}

	// 5. 创建会话
	if err = s.sessionStore.CreateSession(ctx, &session); err != nil {
		zap.L().Error("创建会话失败",
			zap.String("send_id", sendId),
			zap.String("receive_id", receiveId),
			zap.String("session_id", session.Uuid),
			zap.Error(err),
		)
		return "", errorx.ErrServerBusy
	}

	zap.L().Info("会话创建成功",
		zap.String("send_id", sendId),
		zap.String("receive_id", receiveId),
		zap.String("session_id", session.Uuid),
	)

	return session.Uuid, nil
}

// CheckOpenSessionAllowed 检查是否允许发起会话
func (s *SessionService) CheckOpenSessionAllowed(ctx context.Context, sendId, receiveId string) (bool, error) {
	if len(receiveId) == 0 {
		return false, errorx.New(errorx.CodeInvalidParam, "接收方ID不能为空")
	}

	// 根据接收方类型执行不同的校验逻辑
	if receiveId[0] == 'U' {
		// 用户会话：检查好友关系状态
		fsStatus, err := grpc_client.CheckFriendshipStatus(ctx, sendId, receiveId)
		if err != nil {
			zap.L().Error("查询好友关系失败",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
				zap.Error(err),
			)
			return false, errorx.ErrServerBusy
		}
		if fsStatus == 0 {
			return false, errorx.New(errorx.CodeForbidden, "你们还不是好友，无法发起会话")
		} else if fsStatus == 3 {
			return false, errorx.New(errorx.CodeInvalidParam, "已被对方拉黑，无法发起会话")
		} else if fsStatus == 2 {
			return false, errorx.New(errorx.CodeInvalidParam, "已拉黑对方，先解除拉黑状态才能发起会话")
		}
	} else if receiveId[0] == 'G' {
		// 群组会话：检查群成员身份
		isMember, err := grpc_client.CheckGroupMember(ctx, receiveId, sendId)
		if err != nil {
			zap.L().Error("查询群成员关系失败",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
				zap.Error(err),
			)
			return false, errorx.ErrServerBusy
		}
		if !isMember {
			return false, errorx.New(errorx.CodeForbidden, "你不是该群成员，无法发起会话")
		}
	} else {
		return false, errorx.New(errorx.CodeInvalidParam, "无效的接收方ID格式")
	}

	// 检查接收方(用户或群组)是否可用 (使用缓存优化)
	if err := s.checkTargetStatusWithCache(ctx, sendId, receiveId); err != nil {
		zap.L().Warn("接收方状态不可用",
			zap.String("send_id", sendId),
			zap.String("receive_id", receiveId),
			zap.Error(err),
		)
		return false, err
	}

	return true, nil
}

// checkTargetStatusWithCache 检查目标(用户或群组)状态，使用 cacheHelper
func (s *SessionService) checkTargetStatusWithCache(ctx context.Context, sendId, targetId string) error {
	if len(targetId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "目标ID为空")
	}

	// 处理用户
	if targetId[0] == 'U' {
		key := constants.CacheKeyUserInfo + targetId
		var userRsp userrsp.GetUserInfoRespond

		err := s.cacheHelper.GetOrLoad(
			ctx,
			key,
			func(loaderCtx context.Context) (interface{}, error) {
				userStatus, err := grpc_client.GetUserStatus(loaderCtx, targetId)
				if err != nil {
					if status.Code(err) == codes.NotFound {
						return nil, errorx.New(errorx.CodeUserNotExist, "对方用户不存在")
					}
					return nil, errorx.ErrServerBusy
				}
				return userrsp.GetUserInfoRespond{
					Uuid:   targetId,
					Status: userStatus,
				}, nil
			},
			cacheutil.RandomizedTTL(30*time.Minute), // 数据 TTL
			5*time.Minute, // 空值 TTL
			&userRsp,
		)
		if err != nil {
			return err
		}
		if userRsp.Status == user_status.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "对方已被禁用，无法发起会话")
		}
		return nil
	}

	// 处理群组
	if targetId[0] == 'G' {
		key := constants.CacheKeyGroupInfo + targetId
		var groupRsp grouprsp.GetGroupInfoRespond

		err := s.cacheHelper.GetOrLoad(
			ctx,
			key,
			func(loaderCtx context.Context) (interface{}, error) {
				if _, err := grpc_client.GetGroupDetail(loaderCtx, sendId, targetId); err != nil {
					return nil, err
				}
				return grouprsp.GetGroupInfoRespond{Uuid: targetId, Status: group_status.NORMAL}, nil
			},
			cacheutil.RandomizedTTL(30*time.Minute), // 数据 TTL
			5*time.Minute, // 空值 TTL
			&groupRsp,
		)
		if err != nil {
			return err
		}
		return nil
	}

	// 未知类型
	return errorx.New(errorx.CodeInvalidParam, "无效的目标ID格式")
}

// OpenSession 打开会话
// sendId: 从 JWT 上下文获取的当前用户 ID，防止 IDOR 攻击
func (s *SessionService) OpenSession(ctx context.Context, sendId string, req sessionreq.OpenSessionRequest) (string, error) {
	cacheKey := constants.CacheKeySessionOpen + sendId + "_" + req.ReceiveId

	// 1. 查缓存
	rspString, err := s.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var session model.Session
		if err := json.Unmarshal([]byte(rspString), &session); err == nil {
			return session.Uuid, nil
		}
		// 反序列化失败，记录日志并降级查库（不要直接返回空）
		zap.L().Error("Unmarshal session cache failed", zap.Error(err))
	}

	// 2. 查库（缓存未命中或反序列化失败）
	session, err := s.sessionStore.FindBySendIdAndReceiveId(ctx, sendId, req.ReceiveId)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			zap.L().Info("会话没有找到，将新建会话")
			return s.CreateSession(ctx, sendId, req.ReceiveId)
		}
		zap.L().Error("service error", zap.Error(err))
		return "", errorx.ErrServerBusy
	}

	// 3. 【优化点】缓存回写
	s.cache.SubmitTask(func() {
		if data, err := json.Marshal(session); err == nil {
			_ = s.cache.Set(context.Background(), cacheKey, string(data), time.Minute*constants.REDIS_TIMEOUT)
		}
	})

	return session.Uuid, nil
}

// GetUserSessionList 获取用户单聊会话列表（分页）
func (s *SessionService) GetUserSessionList(ctx context.Context, ownerId string, page, pageSize int) ([]sessionrsp.UserSessionListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 直接在数据库层按类型过滤，确保 total 准确
	sessionList, total, err := s.sessionStore.FindBySendIdAndTypePaged(ctx, ownerId, "U", page, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	return s.hydrateUserSessions(ctx, sessionList), total, nil
}

// GetGroupSessionList 获取群聊会话列表（分页）
func (s *SessionService) GetGroupSessionList(ctx context.Context, ownerId string, page, pageSize int) ([]sessionrsp.GroupSessionListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 直接在数据库层按类型过滤，确保 total 准确
	sessionList, total, err := s.sessionStore.FindBySendIdAndTypePaged(ctx, ownerId, "G", page, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	return s.hydrateGroupSessions(ctx, ownerId, sessionList), total, nil
}

// GetUserSessionListCursor 获取用户单聊会话列表（游标分页）
// cursor: 上一页最后一条会话的时间戳（Unix时间戳字符串）
func (s *SessionService) GetUserSessionListCursor(ctx context.Context, ownerId, cursor string, pageSize int) ([]sessionrsp.UserSessionListRespond, string, bool, error) {
	// 设置默认分页参数
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 游标分页查询
	result, err := s.sessionStore.FindBySendIdAndTypeCursor(ctx, ownerId, "U", cursor, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	return s.hydrateUserSessions(ctx, result.Sessions), result.NextCursor, result.HasMore, nil
}

// GetGroupSessionListCursor 获取群聊会话列表（游标分页）
// cursor: 上一页最后一条会话的时间戳（Unix时间戳字符串）
func (s *SessionService) GetGroupSessionListCursor(ctx context.Context, ownerId, cursor string, pageSize int) ([]sessionrsp.GroupSessionListRespond, string, bool, error) {
	// 设置默认分页参数
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 游标分页查询
	result, err := s.sessionStore.FindBySendIdAndTypeCursor(ctx, ownerId, "G", cursor, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	return s.hydrateGroupSessions(ctx, ownerId, result.Sessions), result.NextCursor, result.HasMore, nil
}

// hydrateUserSessions 动态通过 RPC 批量聚合用户信息，消除 session 表冗余字段
func (s *SessionService) hydrateUserSessions(ctx context.Context, sessions []model.Session) []sessionrsp.UserSessionListRespond {
	userIds := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		userIds = append(userIds, sess.ReceiveId)
	}

	userMap := make(map[string]*userpb.PublicUserInfo)
	if len(userIds) > 0 {
		if users, err := grpc_client.BatchGetPublicUserInfo(ctx, userIds); err == nil {
			for _, u := range users {
				userMap[u.Uuid] = u
			}
		} else {
			zap.L().Warn("hydrateUserSessions: BatchGetPublicUserInfo failed", zap.Error(err))
		}
	}

	rsp := make([]sessionrsp.UserSessionListRespond, 0, len(sessions))
	for _, sess := range sessions {
		var lastMessageTime string
		if sess.LastMessageAt.Valid {
			lastMessageTime = sess.LastMessageAt.Time.Format("2006-01-02 15:04:05")
		}

		var username, avatar string
		if u, ok := userMap[sess.ReceiveId]; ok {
			username = u.Nickname
			avatar = u.Avatar
		}

		rsp = append(rsp, sessionrsp.UserSessionListRespond{
			SessionId:       sess.Uuid,
			Avatar:          avatar,
			UserId:          sess.ReceiveId,
			Username:        username,
			LastMessage:     sess.LastMessage,
			LastMessageTime: lastMessageTime,
			LastMessageType: sess.LastMessageType,
			IsPinned:        sess.IsPinned,
		})
	}
	return rsp
}

// hydrateGroupSessions 动态通过 RPC 批量聚合群组信息，消除 session 表冗余字段
func (s *SessionService) hydrateGroupSessions(ctx context.Context, ownerId string, sessions []model.Session) []sessionrsp.GroupSessionListRespond {
	groupMap := make(map[string]*grouppb.GetGroupDetailResponse)
	for _, sess := range sessions {
		if _, exists := groupMap[sess.ReceiveId]; !exists {
			if gDetail, err := grpc_client.GetGroupDetail(ctx, ownerId, sess.ReceiveId); err == nil && gDetail != nil {
				groupMap[sess.ReceiveId] = gDetail
			} else {
				zap.L().Warn("hydrateGroupSessions: GetGroupDetail failed", zap.String("groupId", sess.ReceiveId), zap.Error(err))
			}
		}
	}

	rsp := make([]sessionrsp.GroupSessionListRespond, 0, len(sessions))
	for _, sess := range sessions {
		var lastMessageTime string
		if sess.LastMessageAt.Valid {
			lastMessageTime = sess.LastMessageAt.Time.Format("2006-01-02 15:04:05")
		}

		var groupName, avatar string
		if g, ok := groupMap[sess.ReceiveId]; ok {
			groupName = g.GroupName
			avatar = g.GroupAvatar
		}

		rsp = append(rsp, sessionrsp.GroupSessionListRespond{
			SessionId:       sess.Uuid,
			Avatar:          avatar,
			GroupId:         sess.ReceiveId,
			GroupName:       groupName,
			LastMessage:     sess.LastMessage,
			LastMessageTime: lastMessageTime,
			LastMessageType: sess.LastMessageType,
			IsPinned:        sess.IsPinned,
		})
	}
	return rsp
}

// DeleteSession 删除会话
func (s *SessionService) DeleteSession(ctx context.Context, ownerId, sessionId string) error {
	// 1. 权限校验: 直接按 UUID 查询会话，验证归属关系
	session, err := s.sessionStore.FindByUuid(ctx, sessionId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "会话不存在")
		}
		zap.L().Error("查询会话失败", zap.String("session_id", sessionId), zap.Error(err))
		return errorx.ErrServerBusy
	}
	if session.SendId != ownerId {
		return errorx.New(errorx.CodeForbidden, "无权删除该会话")
	}

	// 2. 软删除会话
	if err := s.sessionStore.SoftDeleteByUuids(ctx, []string{sessionId}); err != nil {
		zap.L().Error("删除会话失败",
			zap.String("owner_id", ownerId),
			zap.String("session_id", sessionId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	return nil
}

// PinSession 置顶/取消置顶会话
func (s *SessionService) PinSession(ctx context.Context, userId, sessionId string, isPinned bool) error {
	// 权限校验: 只能操作自己的会话
	session, err := s.sessionStore.FindByUuid(ctx, sessionId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "会话不存在")
		}
		zap.L().Error("查询会话失败", zap.String("session_id", sessionId), zap.Error(err))
		return errorx.ErrServerBusy
	}
	if session.SendId != userId {
		return errorx.New(errorx.CodeForbidden, "无权操作该会话")
	}

	if err := s.sessionStore.UpdatePinStatus(ctx, sessionId, isPinned); err != nil {
		zap.L().Error("更新会话置顶状态失败",
			zap.String("session_id", sessionId),
			zap.Bool("is_pinned", isPinned),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	return nil
}

// DeleteGroupMemberSessions 批量软删除指定群成员在该群的会话（供跨服务 RPC 调用）
func (s *SessionService) DeleteGroupMemberSessions(ctx context.Context, groupId string, userIds []string) error {
	if len(userIds) == 0 {
		return nil
	}
	uuids := make([]string, 0, len(userIds))
	for _, uid := range userIds {
		if sess, err := s.sessionStore.FindBySendIdAndReceiveId(ctx, uid, groupId); err == nil && sess != nil {
			uuids = append(uuids, sess.Uuid)
		}
	}
	if len(uuids) == 0 {
		return nil
	}
	return s.sessionStore.SoftDeleteByUuids(ctx, uuids)
}

// CreateGroupSession 为指定用户创建该群的会话（幂等）
func (s *SessionService) CreateGroupSession(ctx context.Context, groupId, userId, groupName, groupAvatar string) (string, error) {
	existing, err := s.sessionStore.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err == nil && existing != nil {
		return existing.Uuid, nil
	}
	sess := model.Session{
		Uuid:      "S" + snowflake.GenerateIDString(),
		SendId:    userId,
		ReceiveId: groupId,
	}
	if err := s.sessionStore.CreateSession(ctx, &sess); err != nil {
		return "", err
	}
	return sess.Uuid, nil
}

// DeleteFriendSessions 软删除双方的好友私聊会话（拉黑/删好友）
func (s *SessionService) DeleteFriendSessions(ctx context.Context, userOneId, userTwoId string) error {
	uuids := make([]string, 0, 2)
	if sess, err := s.sessionStore.FindBySendIdAndReceiveId(ctx, userOneId, userTwoId); err == nil && sess != nil {
		uuids = append(uuids, sess.Uuid)
	}
	if sess, err := s.sessionStore.FindBySendIdAndReceiveId(ctx, userTwoId, userOneId); err == nil && sess != nil {
		uuids = append(uuids, sess.Uuid)
	}
	if len(uuids) == 0 {
		return nil
	}
	return s.sessionStore.SoftDeleteByUuids(ctx, uuids)
}

