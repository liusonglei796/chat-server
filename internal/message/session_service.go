package message

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grouppb "kama_chat_server/api/gen/group"
	userpb "kama_chat_server/api/gen/user"
	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/dberr"
	grpcinfra "kama_chat_server/internal/infra/grpc_client"
	snowflakeinfra "kama_chat_server/internal/infra/snowflake"
	"kama_chat_server/pkg/constants"
	group_status "kama_chat_server/pkg/enum/group/group_status"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// SessionService 会话业务逻辑实现
type SessionService struct {
	sessionStore SessionStore
	messageStore MessageStore
	cache        cacheinfra.AsyncCacheService
	cacheHelper  *cacheinfra.Helper
	guard        idempotent.Guard
}

// NewSessionService 构造函数，注入所有依赖
func NewSessionService(
	sessionStore SessionStore,
	messageStore MessageStore,
	cacheService cacheinfra.AsyncCacheService,
	guard ...idempotent.Guard,
) *SessionService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	return &SessionService{
		sessionStore: sessionStore,
		messageStore: messageStore,
		cache:        cacheService,
		cacheHelper:  cacheinfra.NewHelper(cacheService),
		guard:        g,
	}
}

// CreateSession 创建会话
func (s *SessionService) CreateSession(ctx context.Context, sendId, receiveId string) (string, error) {
	existingSession, err := s.sessionStore.FindBySendIdAndReceiveId(ctx, sendId, receiveId)
	if err != nil {
		if errorx.GetCode(err) != errorx.CodeNotFound {
			zap.L().Error("查询已有会话失败",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
				zap.Error(err),
			)
			return "", errorx.ErrServerBusy
		}
	} else {
		zap.L().Info("会话已存在，返回已有会话",
			zap.String("send_id", sendId),
			zap.String("receive_id", receiveId),
			zap.String("session_id", existingSession.Uuid),
		)
		return existingSession.Uuid, nil
	}

	sendStatus, err := grpcinfra.GetUserStatus(ctx, sendId)
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

	var session Session
	session.Uuid = fmt.Sprintf("S%s", snowflakeinfra.GenerateIDString())
	session.SendId = sendId
	session.ReceiveId = receiveId
	session.CreatedAt = time.Now()

	if receiveId[0] == 'U' {
		recvStatus, err := grpcinfra.GetUserStatus(ctx, receiveId)
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

		isFriend, err := grpcinfra.CheckFriendshipStatus(ctx, sendId, receiveId)
		if err != nil {
			zap.L().Error("Check friend relationship error", zap.Error(err))
			return "", errorx.ErrServerBusy
		}
		if isFriend != 1 {
			return "", errorx.New(errorx.CodeForbidden, "你们还不是好友")
		}
	} else {
		_, err := grpcinfra.GetGroupDetail(ctx, sendId, receiveId)
		if err != nil {
			zap.L().Error("query group via grpc error", zap.Error(err))
			return "", err
		}
	}

	if err = s.sessionStore.CreateSession(ctx, &session); err != nil {
		if dberr.IsDuplicateKey(err) {
			if existing, queryErr := s.sessionStore.FindBySendIdAndReceiveIdUnscoped(ctx, sendId, receiveId); queryErr == nil && existing != nil {
				return existing.Uuid, nil
			}
		}
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

	if receiveId[0] == 'U' {
		fsStatus, err := grpcinfra.CheckFriendshipStatus(ctx, sendId, receiveId)
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
		isMember, err := grpcinfra.CheckGroupMember(ctx, receiveId, sendId)
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

type cachedTargetStatus struct {
	Uuid   string `json:"uuid"`
	Status int8   `json:"status"`
}

func (s *SessionService) checkTargetStatusWithCache(ctx context.Context, sendId, targetId string) error {
	if len(targetId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "目标ID为空")
	}

	if targetId[0] == 'U' {
		key := constants.CacheKeyUserInfo + targetId
		var userRsp cachedTargetStatus

		err := s.cacheHelper.GetOrLoad(
			ctx,
			key,
			func(loaderCtx context.Context) (interface{}, error) {
				userStatus, err := grpcinfra.GetUserStatus(loaderCtx, targetId)
				if err != nil {
					if status.Code(err) == codes.NotFound {
						return nil, errorx.New(errorx.CodeUserNotExist, "对方用户不存在")
					}
					return nil, errorx.ErrServerBusy
				}
				return cachedTargetStatus{
					Uuid:   targetId,
					Status: userStatus,
				}, nil
			},
			cacheinfra.RandomizedTTL(30*time.Minute),
			5*time.Minute,
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

	if targetId[0] == 'G' {
		key := constants.CacheKeyGroupInfo + targetId
		var groupRsp cachedTargetStatus

		err := s.cacheHelper.GetOrLoad(
			ctx,
			key,
			func(loaderCtx context.Context) (interface{}, error) {
				if _, err := grpcinfra.GetGroupDetail(loaderCtx, sendId, targetId); err != nil {
					return nil, err
				}
				return cachedTargetStatus{Uuid: targetId, Status: group_status.NORMAL}, nil
			},
			cacheinfra.RandomizedTTL(30*time.Minute),
			5*time.Minute,
			&groupRsp,
		)
		if err != nil {
			return err
		}
		return nil
	}

	return errorx.New(errorx.CodeInvalidParam, "无效的目标ID格式")
}

// OpenSession 打开会话
func (s *SessionService) OpenSession(ctx context.Context, sendId string, req OpenSessionRequest) (string, error) {
	action := func(execCtx context.Context) (string, error) {
		cacheKey := constants.CacheKeySessionOpen + sendId + "_" + req.ReceiveId

		rspString, err := s.cache.Get(execCtx, cacheKey)
		if err == nil && rspString != "" {
			var session Session
			if err := json.Unmarshal([]byte(rspString), &session); err == nil {
				return session.Uuid, nil
			}
			zap.L().Error("Unmarshal session cache failed", zap.Error(err))
		}

		session, err := s.sessionStore.FindBySendIdAndReceiveId(execCtx, sendId, req.ReceiveId)
		if err != nil {
			if errorx.GetCode(err) == errorx.CodeNotFound {
				zap.L().Info("会话没有找到，将新建会话")
				return s.CreateSession(execCtx, sendId, req.ReceiveId)
			}
			zap.L().Error("service error", zap.Error(err))
			return "", errorx.ErrServerBusy
		}

		s.cache.SubmitTask(func() {
			if data, err := json.Marshal(session); err == nil {
				_ = s.cache.Set(context.Background(), cacheKey, string(data), time.Minute*constants.REDIS_TIMEOUT)
			}
		})

		return session.Uuid, nil
	}

	if s.guard != nil {
		var key string
		if req.ClientToken != "" {
			key = "session:open:" + req.ClientToken
		} else {
			key = "session:open:" + sendId + ":" + req.ReceiveId
		}
		rsp, _, err := idempotent.ExecuteJSON(ctx, s.guard, key, 5*time.Second, 24*time.Hour, action)
		return rsp, err
	}

	return action(ctx)
}

// GetUserSessionList 获取用户单聊会话列表（分页）
func (s *SessionService) GetUserSessionList(ctx context.Context, ownerId string, page, pageSize int) ([]UserSessionListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	sessionList, total, err := s.sessionStore.FindBySendIdAndTypePaged(ctx, ownerId, "U", page, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	return s.hydrateUserSessions(ctx, sessionList), total, nil
}

// GetGroupSessionList 获取群聊会话列表（分页）
func (s *SessionService) GetGroupSessionList(ctx context.Context, ownerId string, page, pageSize int) ([]GroupSessionListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	sessionList, total, err := s.sessionStore.FindBySendIdAndTypePaged(ctx, ownerId, "G", page, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	return s.hydrateGroupSessions(ctx, ownerId, sessionList), total, nil
}

// GetUserSessionListCursor 获取用户单聊会话列表（游标分页）
func (s *SessionService) GetUserSessionListCursor(ctx context.Context, ownerId, cursor string, pageSize int) ([]UserSessionListRespond, string, bool, error) {
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	result, err := s.sessionStore.FindBySendIdAndTypeCursor(ctx, ownerId, "U", cursor, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	return s.hydrateUserSessions(ctx, result.Sessions), result.NextCursor, result.HasMore, nil
}

// GetGroupSessionListCursor 获取群聊会话列表（游标分页）
func (s *SessionService) GetGroupSessionListCursor(ctx context.Context, ownerId, cursor string, pageSize int) ([]GroupSessionListRespond, string, bool, error) {
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	result, err := s.sessionStore.FindBySendIdAndTypeCursor(ctx, ownerId, "G", cursor, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	return s.hydrateGroupSessions(ctx, ownerId, result.Sessions), result.NextCursor, result.HasMore, nil
}

func (s *SessionService) hydrateUserSessions(ctx context.Context, sessions []Session) []UserSessionListRespond {
	userIds := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		userIds = append(userIds, sess.ReceiveId)
	}

	userMap := make(map[string]*userpb.PublicUserInfo)
	if len(userIds) > 0 {
		if users, err := grpcinfra.BatchGetPublicUserInfo(ctx, userIds); err == nil {
			for _, u := range users {
				userMap[u.Uuid] = u
			}
		} else {
			zap.L().Warn("hydrateUserSessions: BatchGetPublicUserInfo failed", zap.Error(err))
		}
	}

	rsp := make([]UserSessionListRespond, 0, len(sessions))
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

		rsp = append(rsp, UserSessionListRespond{
			SessionId:       sess.Uuid,
			Avatar:          avatar,
			UserId:          sess.ReceiveId,
			Username:        username,
			LastMessage:     sess.LastMessage,
			LastMessageTime: lastMessageTime,
			LastMessageType: sess.LastMessageType,
			LastSeqId:       sess.LastSeqId,
			IsPinned:        sess.IsPinned,
		})
	}
	return rsp
}

func (s *SessionService) hydrateGroupSessions(ctx context.Context, ownerId string, sessions []Session) []GroupSessionListRespond {
	groupMap := make(map[string]*grouppb.GetGroupDetailResponse)
	for _, sess := range sessions {
		if _, exists := groupMap[sess.ReceiveId]; !exists {
			if gDetail, err := grpcinfra.GetGroupDetail(ctx, ownerId, sess.ReceiveId); err == nil && gDetail != nil {
				groupMap[sess.ReceiveId] = gDetail
			} else {
				zap.L().Warn("hydrateGroupSessions: GetGroupDetail failed", zap.String("groupId", sess.ReceiveId), zap.Error(err))
			}
		}
	}

	rsp := make([]GroupSessionListRespond, 0, len(sessions))
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

		rsp = append(rsp, GroupSessionListRespond{
			SessionId:       sess.Uuid,
			Avatar:          avatar,
			GroupId:         sess.ReceiveId,
			GroupName:       groupName,
			LastMessage:     sess.LastMessage,
			LastMessageTime: lastMessageTime,
			LastMessageType: sess.LastMessageType,
			LastSeqId:       sess.LastSeqId,
			IsPinned:        sess.IsPinned,
		})
	}
	return rsp
}

// DeleteSession 删除会话
func (s *SessionService) DeleteSession(ctx context.Context, ownerId, sessionId string) error {
	session, err := s.sessionStore.FindByUuid(ctx, sessionId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil
		}
		zap.L().Error("查询会话失败", zap.String("session_id", sessionId), zap.Error(err))
		return errorx.ErrServerBusy
	}
	if session.SendId != ownerId {
		return errorx.New(errorx.CodeForbidden, "无权删除该会话")
	}

	if err := s.sessionStore.SoftDeleteByUuids(ctx, []string{sessionId}); err != nil {
		zap.L().Error("删除会话失败",
			zap.String("owner_id", ownerId),
			zap.String("session_id", sessionId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	cacheKey := constants.CacheKeySessionOpen + ownerId + "_" + session.ReceiveId
	s.cache.SubmitTask(func() {
		_ = s.cache.Delete(context.Background(), cacheKey)
	})

	return nil
}

// PinSession 置顶/取消置顶会话
func (s *SessionService) PinSession(ctx context.Context, userId, sessionId string, isPinned bool) error {
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

// DeleteGroupMemberSessions 批量软删除指定群成员在该群的会话
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
	sess := Session{
		Uuid:      "S" + snowflakeinfra.GenerateIDString(),
		SendId:    userId,
		ReceiveId: groupId,
	}
	if err := s.sessionStore.CreateSession(ctx, &sess); err != nil {
		if dberr.IsDuplicateKey(err) {
			if existing, qErr := s.sessionStore.FindBySendIdAndReceiveIdUnscoped(ctx, userId, groupId); qErr == nil && existing != nil {
				return existing.Uuid, nil
			}
		}
		return "", err
	}
	return sess.Uuid, nil
}

// DeleteFriendSessions 软删除双方的好友私聊会话
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
