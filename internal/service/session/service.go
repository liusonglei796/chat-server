package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"kama_chat_server/internal/domain/repository"
	sessionreq "kama_chat_server/internal/dto/request/session"
	"kama_chat_server/internal/dto/respond/group"
	sessionrsp "kama_chat_server/internal/dto/respond/session"
	"kama_chat_server/internal/dto/respond/user"
	"kama_chat_server/internal/grpc_client"
	cacheutil "kama_chat_server/internal/infrastructure/cache"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/group/group_status"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
)

// SessionService 会话业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖
type SessionService struct {
	sessionRepo repository.SessionRepository
	messageRepo repository.MessageRepository
	cache       repository.AsyncCacheService
	cacheHelper *cacheutil.Helper // 缓存辅助工具（带 singleflight）
}

// NewSessionService 构造函数，注入所有依赖
func NewSessionService(
	sessionRepo repository.SessionRepository,
	messageRepo repository.MessageRepository,
	cacheService repository.AsyncCacheService,
) *SessionService {
	return &SessionService{
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
	}
}

// CreateSession 创建会话
func (s *SessionService) CreateSession(ctx context.Context, sendId, receiveId string) (string, error) {
	// 1. 幂等性检查：先查询是否已存在会话
	existingSession, err := s.sessionRepo.FindBySendIdAndReceiveId(ctx, sendId, receiveId)
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

		nickname, avatar, err := grpc_client.GetUserNicknameAvatar(ctx, receiveId)
		if err != nil {
			zap.L().Error("get receiver info via grpc error", zap.Error(err))
			return "", errorx.ErrServerBusy
		}
		session.ReceiveName = nickname
		session.Avatar = avatar
	} else {
		// 用户对群组会话
		group, err := grpc_client.GetGroupDetail(ctx, sendId, receiveId)
		if err != nil {
			zap.L().Error("query group via grpc error", zap.Error(err))
			return "", err
		}
		session.ReceiveName = group.GroupName
		session.Avatar = group.GroupAvatar
	}

	// 5. 创建会话
	if err = s.sessionRepo.CreateSession(ctx, &session); err != nil {
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
		var userRsp user.GetUserInfoRespond

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
				return user.GetUserInfoRespond{
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
		var groupRsp group.GetGroupInfoRespond

		err := s.cacheHelper.GetOrLoad(
			ctx,
			key,
			func(loaderCtx context.Context) (interface{}, error) {
				if _, err := grpc_client.GetGroupDetail(loaderCtx, sendId, targetId); err != nil {
					return nil, err
				}
				return group.GetGroupInfoRespond{Uuid: targetId, Status: group_status.NORMAL}, nil
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
	session, err := s.sessionRepo.FindBySendIdAndReceiveId(ctx, sendId, req.ReceiveId)
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
	sessionList, total, err := s.sessionRepo.FindBySendIdAndTypePaged(ctx, ownerId, "U", page, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	sessionListRsp := make([]sessionrsp.UserSessionListRespond, 0, len(sessionList))
	for i := 0; i < len(sessionList); i++ {
		var lastMessageTime string
		if sessionList[i].LastMessageAt.Valid {
			lastMessageTime = sessionList[i].LastMessageAt.Time.Format("2006-01-02 15:04:05")
		}

		sessionListRsp = append(sessionListRsp, sessionrsp.UserSessionListRespond{
			SessionId:       sessionList[i].Uuid,
			Avatar:          sessionList[i].Avatar,
			UserId:          sessionList[i].ReceiveId,
			Username:        sessionList[i].ReceiveName,
			LastMessage:     sessionList[i].LastMessage,
			LastMessageTime: lastMessageTime,
			LastMessageType: sessionList[i].LastMessageType,
			IsPinned:        sessionList[i].IsPinned,
		})
	}

	return sessionListRsp, total, nil
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
	sessionList, total, err := s.sessionRepo.FindBySendIdAndTypePaged(ctx, ownerId, "G", page, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	sessionListRsp := make([]sessionrsp.GroupSessionListRespond, 0, len(sessionList))
	for i := 0; i < len(sessionList); i++ {
		var lastMessageTime string
		if sessionList[i].LastMessageAt.Valid {
			lastMessageTime = sessionList[i].LastMessageAt.Time.Format("2006-01-02 15:04:05")
		}

		sessionListRsp = append(sessionListRsp, sessionrsp.GroupSessionListRespond{
			SessionId:       sessionList[i].Uuid,
			Avatar:          sessionList[i].Avatar,
			GroupId:         sessionList[i].ReceiveId,
			GroupName:       sessionList[i].ReceiveName,
			LastMessage:     sessionList[i].LastMessage,
			LastMessageTime: lastMessageTime,
			LastMessageType: sessionList[i].LastMessageType,
			IsPinned:        sessionList[i].IsPinned,
		})
	}

	return sessionListRsp, total, nil
}

// GetUserSessionListCursor 获取用户单聊会话列表（游标分页）
// cursor: 上一页最后一条会话的时间戳（Unix时间戳字符串）
func (s *SessionService) GetUserSessionListCursor(ctx context.Context, ownerId, cursor string, pageSize int) ([]sessionrsp.UserSessionListRespond, string, bool, error) {
	// 设置默认分页参数
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 游标分页查询
	result, err := s.sessionRepo.FindBySendIdAndTypeCursor(ctx, ownerId, "U", cursor, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	sessionListRsp := make([]sessionrsp.UserSessionListRespond, 0, len(result.Sessions))
	for i := 0; i < len(result.Sessions); i++ {
		var lastMessageTime string
		if result.Sessions[i].LastMessageAt.Valid {
			lastMessageTime = result.Sessions[i].LastMessageAt.Time.Format("2006-01-02 15:04:05")
		}

		sessionListRsp = append(sessionListRsp, sessionrsp.UserSessionListRespond{
			SessionId:       result.Sessions[i].Uuid,
			Avatar:          result.Sessions[i].Avatar,
			UserId:          result.Sessions[i].ReceiveId,
			Username:        result.Sessions[i].ReceiveName,
			LastMessage:     result.Sessions[i].LastMessage,
			LastMessageTime: lastMessageTime,
			LastMessageType: result.Sessions[i].LastMessageType,
			IsPinned:        result.Sessions[i].IsPinned,
		})
	}

	return sessionListRsp, result.NextCursor, result.HasMore, nil
}

// GetGroupSessionListCursor 获取群聊会话列表（游标分页）
// cursor: 上一页最后一条会话的时间戳（Unix时间戳字符串）
func (s *SessionService) GetGroupSessionListCursor(ctx context.Context, ownerId, cursor string, pageSize int) ([]sessionrsp.GroupSessionListRespond, string, bool, error) {
	// 设置默认分页参数
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 游标分页查询
	result, err := s.sessionRepo.FindBySendIdAndTypeCursor(ctx, ownerId, "G", cursor, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	sessionListRsp := make([]sessionrsp.GroupSessionListRespond, 0, len(result.Sessions))
	for i := 0; i < len(result.Sessions); i++ {
		var lastMessageTime string
		if result.Sessions[i].LastMessageAt.Valid {
			lastMessageTime = result.Sessions[i].LastMessageAt.Time.Format("2006-01-02 15:04:05")
		}

		sessionListRsp = append(sessionListRsp, sessionrsp.GroupSessionListRespond{
			SessionId:       result.Sessions[i].Uuid,
			Avatar:          result.Sessions[i].Avatar,
			GroupId:         result.Sessions[i].ReceiveId,
			GroupName:       result.Sessions[i].ReceiveName,
			LastMessage:     result.Sessions[i].LastMessage,
			LastMessageTime: lastMessageTime,
			LastMessageType: result.Sessions[i].LastMessageType,
			IsPinned:        result.Sessions[i].IsPinned,
		})
	}

	return sessionListRsp, result.NextCursor, result.HasMore, nil
}

// DeleteSession 删除会话
func (s *SessionService) DeleteSession(ctx context.Context, ownerId, sessionId string) error {
	// 1. 权限校验: 直接按 UUID 查询会话，验证归属关系
	session, err := s.sessionRepo.FindByUuid(ctx, sessionId)
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
	if err := s.sessionRepo.SoftDeleteByUuids(ctx, []string{sessionId}); err != nil {
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
	session, err := s.sessionRepo.FindByUuid(ctx, sessionId)
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

	if err := s.sessionRepo.UpdatePinStatus(ctx, sessionId, isPinned); err != nil {
		zap.L().Error("更新会话置顶状态失败",
			zap.String("session_id", sessionId),
			zap.Bool("is_pinned", isPinned),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	return nil
}
