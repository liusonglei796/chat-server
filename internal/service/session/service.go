package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	sessionreq "kama_chat_server/internal/dto/request/session"
	"kama_chat_server/internal/dto/respond/group"
	sessionrsp "kama_chat_server/internal/dto/respond/session"
	"kama_chat_server/internal/dto/respond/user"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/contact/contact_type_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
	cacheutil "kama_chat_server/pkg/util/cache"
)

// sessionService 会话业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖
type sessionService struct {
	repos       *mysql.Repositories
	cache       myredis.AsyncCacheService
	cacheHelper *cacheutil.Helper // 缓存辅助工具（带 singleflight）
}

// NewSessionService 构造函数，注入所有依赖
func NewSessionService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *sessionService {
	return &sessionService{
		repos:       repos,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
	}
}

// CreateSession 创建会话
func (s *sessionService) CreateSession(sendId, receiveId string) (string, error) {
	// 1. 幂等性检查：先查询是否已存在会话
	existingSession, err := s.repos.Session.FindBySendIdAndReceiveId(sendId, receiveId)
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

	// 2. 验证发送者是否存在
	_, err = s.repos.User.FindByUuid(sendId)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
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

	// 3. 构建会话基础信息
	var session model.Session
	session.Uuid = fmt.Sprintf("S%s", snowflake.GenerateIDString())
	session.SendId = sendId
	session.ReceiveId = receiveId
	session.CreatedAt = time.Now()

	// 4. 根据接收者类型设置会话信息
	if receiveId[0] == 'U' {
		// 用户对用户会话
		receiveUser, err := s.repos.User.FindByUuid(receiveId)
		if err != nil {
			if errorx.GetCode(err) == errorx.CodeNotFound {
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
		if receiveUser.Status == user_status_enum.DISABLE {
			zap.L().Warn("接收用户已被禁用",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
			)
			return "", errorx.New(errorx.CodeInvalidParam, "该用户被禁用了")
		}
		// 验证好友关系 (必须是好友才能发起会话)
		isFriend, err := s.repos.Contact.IsFriend(sendId, receiveId)
		if err != nil {
			zap.L().Error("Check friend relationship error", zap.Error(err))
			return "", errorx.ErrServerBusy
		}
		if !isFriend {
			return "", errorx.New(errorx.CodeForbidden, "你们还不是好友")
		}

		session.ReceiveName = receiveUser.Nickname
		session.Avatar = receiveUser.Avatar
	} else {
		// 用户对群组会话
		receiveGroup, err := s.repos.Group.FindByUuid(receiveId)
		if err != nil {
			if errorx.GetCode(err) == errorx.CodeNotFound {
				zap.L().Warn("接收群组不存在",
					zap.String("send_id", sendId),
					zap.String("receive_id", receiveId),
					zap.String("operation", "create_session"),
				)
				return "", errorx.New(errorx.CodeNotFound, "接收群组不存在")
			}
			zap.L().Error("查询接收群组失败",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
				zap.Error(err),
			)
			return "", errorx.ErrServerBusy
		}
		if receiveGroup.Status == group_status_enum.DISABLE {
			zap.L().Warn("接收群组已被禁用",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
			)
			return "", errorx.New(errorx.CodeInvalidParam, "该群聊被禁用了")
		}
		// 验证群成员身份 (用户必须是群成员才能发起会话)
		_, errMember := s.repos.GroupMember.FindByGroupAndUser(receiveId, sendId)
		if errMember != nil {
			if errorx.IsNotFound(errMember) {
				return "", errorx.New(errorx.CodeForbidden, "你不是该群成员")
			}
			zap.L().Error("Check group membership error", zap.Error(err))
			return "", errorx.ErrServerBusy
		}
		session.ReceiveName = receiveGroup.Name
		session.Avatar = receiveGroup.Avatar
	}

	// 5. 创建会话
	if err = s.repos.Session.CreateSession(&session); err != nil {
		zap.L().Error("创建会话失败",
			zap.String("send_id", sendId),
			zap.String("receive_id", receiveId),
			zap.String("session_id", session.Uuid),
			zap.Error(err),
		)
		return "", errorx.ErrServerBusy
	}

	// 6. 异步清理缓存
	s.cache.SubmitTask(func() {
		s.clearSessionCacheForUser(sendId)
	})

	zap.L().Info("会话创建成功",
		zap.String("send_id", sendId),
		zap.String("receive_id", receiveId),
		zap.String("session_id", session.Uuid),
	)

	return session.Uuid, nil
}

// clearSessionCacheForUser 清理用户的会话缓存
func (s *sessionService) clearSessionCacheForUser(userId string) {
	if err := s.cache.DeleteByPattern(context.Background(), "group_session_list_"+userId+"*"); err != nil {
		zap.L().Error("清除群会话列表缓存失败", zap.Error(err))
	}
	if err := s.cache.DeleteByPattern(context.Background(), "session_list_"+userId+"*"); err != nil {
		zap.L().Error("清除会话列表缓存失败", zap.Error(err))
	}
	if err := s.cache.DeleteByPattern(context.Background(), "direct_session_list_"+userId+"*"); err != nil {
		zap.L().Error("清除私聊会话列表缓存失败", zap.Error(err))
	}
}

// CheckOpenSessionAllowed 检查是否允许发起会话
func (s *sessionService) CheckOpenSessionAllowed(sendId, receiveId string) (bool, error) {
	if len(receiveId) == 0 {
		return false, errorx.New(errorx.CodeInvalidParam, "接收方ID不能为空")
	}

	// 根据接收方类型执行不同的校验逻辑
	if receiveId[0] == 'U' {
		// 用户会话：检查联系关系状态
		contact, err := s.repos.Contact.FindByUserIdAndContactId(sendId, receiveId, contact_type_enum.USER)
		if err != nil {
			zap.L().Error("查询联系关系失败",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
				zap.Error(err),
			)
			return false, errorx.ErrServerBusy
		}
		if contact.Status == contact_status_enum.BE_BLACK {
			return false, errorx.New(errorx.CodeInvalidParam, "已被对方拉黑，无法发起会话")
		} else if contact.Status == contact_status_enum.BLACK {
			return false, errorx.New(errorx.CodeInvalidParam, "已拉黑对方，先解除拉黑状态才能发起会话")
		}
	} else if receiveId[0] == 'G' {
		// 群组会话：检查群成员身份
		_, err := s.repos.GroupMember.FindByGroupAndUser(receiveId, sendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return false, errorx.New(errorx.CodeForbidden, "你不是该群成员，无法发起会话")
			}
			zap.L().Error("查询群成员关系失败",
				zap.String("send_id", sendId),
				zap.String("receive_id", receiveId),
				zap.Error(err),
			)
			return false, errorx.ErrServerBusy
		}
	} else {
		return false, errorx.New(errorx.CodeInvalidParam, "无效的接收方ID格式")
	}

	// 检查接收方(用户或群组)是否可用 (使用缓存优化)
	if err := s.checkTargetStatusWithCache(receiveId); err != nil {
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
func (s *sessionService) checkTargetStatusWithCache(targetId string) error {
	if len(targetId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "目标ID为空")
	}

	// 处理用户
	if targetId[0] == 'U' {
		key := "user_info_" + targetId
		var userRsp user.GetUserInfoRespond

		err := s.cacheHelper.GetOrLoad(
			context.Background(),
			key,
			func() (interface{}, error) {
				u, err := s.repos.User.FindByUuid(targetId)
				if err != nil {
					if errorx.IsNotFound(err) {
						return nil, errorx.New(errorx.CodeUserNotExist, "对方用户不存在")
					}
					return nil, errorx.ErrServerBusy
				}
				return user.GetUserInfoRespond{
					Uuid:   u.Uuid,
					Status: u.Status,
				}, nil
			},
			cacheutil.RandomizedTTL(30*time.Minute), // 数据 TTL
			5*time.Minute, // 空值 TTL
			&userRsp,
		)
		if err != nil {
			return err
		}
		if userRsp.Status == user_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "对方已被禁用，无法发起会话")
		}
		return nil
	}

	// 处理群组
	if targetId[0] == 'G' {
		key := "group_info_" + targetId
		var groupRsp group.GetGroupInfoRespond

		err := s.cacheHelper.GetOrLoad(
			context.Background(),
			key,
			func() (interface{}, error) {
				g, err := s.repos.Group.FindByUuid(targetId)
				if err != nil {
					if errorx.IsNotFound(err) {
						return nil, errorx.New(errorx.CodeNotFound, "对方群组不存在")
					}
					return nil, errorx.ErrServerBusy
				}
				return group.GetGroupInfoRespond{
					Uuid:   g.Uuid,
					Status: g.Status,
				}, nil
			},
			cacheutil.RandomizedTTL(30*time.Minute), // 数据 TTL
			5*time.Minute, // 空值 TTL
			&groupRsp,
		)
		if err != nil {
			return err
		}
		if groupRsp.Status == group_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "对方群组已被禁用，无法发起会话")
		}
		return nil
	}

	// 未知类型
	return errorx.New(errorx.CodeInvalidParam, "无效的目标ID格式")
}

// OpenSession 打开会话
// sendId: 从 JWT 上下文获取的当前用户 ID，防止 IDOR 攻击
func (s *sessionService) OpenSession(sendId string, req sessionreq.OpenSessionRequest) (string, error) {
	cacheKey := "session_" + sendId + "_" + req.ReceiveId

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
	session, err := s.repos.Session.FindBySendIdAndReceiveId(sendId, req.ReceiveId)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			zap.L().Info("会话没有找到，将新建会话")
			return s.CreateSession(sendId, req.ReceiveId)
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
func (s *sessionService) GetUserSessionList(ownerId string, page, pageSize int) ([]sessionrsp.UserSessionListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 直接在数据库层按类型过滤，确保 total 准确
	sessionList, total, err := s.repos.Session.FindBySendIdAndTypePaged(ownerId, "U", page, pageSize)
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
		})
	}

	return sessionListRsp, total, nil
}

// GetGroupSessionList 获取群聊会话列表（分页）
func (s *sessionService) GetGroupSessionList(ownerId string, page, pageSize int) ([]sessionrsp.GroupSessionListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 直接在数据库层按类型过滤，确保 total 准确
	sessionList, total, err := s.repos.Session.FindBySendIdAndTypePaged(ownerId, "G", page, pageSize)
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
		})
	}

	return sessionListRsp, total, nil
}

// DeleteSession 删除会话
func (s *sessionService) DeleteSession(ownerId, sessionId string) error {
	// 1. 权限校验: 直接按 UUID 查询会话，验证归属关系
	session, err := s.repos.Session.FindByUuid(sessionId)
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
	if err := s.repos.Session.SoftDeleteByUuids([]string{sessionId}); err != nil {
		zap.L().Error("删除会话失败",
			zap.String("owner_id", ownerId),
			zap.String("session_id", sessionId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	// 3. 异步清理缓存
	s.cache.SubmitTask(func() {
		s.clearSessionCacheForUser(ownerId)
	})

	return nil
}
