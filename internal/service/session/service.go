package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/random"
)

// sessionService 会话业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖
type sessionService struct {
	repos *mysql.Repositories
	cache myredis.AsyncCacheService
}

// NewSessionService 构造函数，注入所有依赖
func NewSessionService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *sessionService {
	return &sessionService{
		repos: repos,
		cache: cacheService,
	}
}

// CreateSession 创建会话
func (s *sessionService) CreateSession(req request.CreateSessionRequest) (string, error) {
	// 1. 幂等性检查：先查询是否已存在会话
	existingSession, err := s.repos.Session.FindBySendIdAndReceiveId(req.SendId, req.ReceiveId)
	if err != nil {
		// 如果不是"未找到"错误，则返回数据库错误
		if errorx.GetCode(err) != errorx.CodeNotFound {
			zap.L().Error("查询已有会话失败",
				zap.String("send_id", req.SendId),
				zap.String("receive_id", req.ReceiveId),
				zap.Error(err),
			)
			return "", errorx.ErrServerBusy
		}
		// 未找到会话，继续创建新会话
	} else {
		// 会话已存在，直接返回已有会话ID
		zap.L().Info("会话已存在，返回已有会话",
			zap.String("send_id", req.SendId),
			zap.String("receive_id", req.ReceiveId),
			zap.String("session_id", existingSession.Uuid),
		)
		return existingSession.Uuid, nil
	}

	// 2. 验证发送者是否存在
	_, err = s.repos.User.FindByUuid(req.SendId)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			zap.L().Warn("发送用户不存在",
				zap.String("send_id", req.SendId),
				zap.String("operation", "create_session"),
			)
			return "", errorx.New(errorx.CodeUserNotExist, "发送用户不存在")
		}
		zap.L().Error("查询发送用户失败",
			zap.String("send_id", req.SendId),
			zap.Error(err),
		)
		return "", errorx.ErrServerBusy
	}

	// 3. 构建会话基础信息
	var session model.Session
	session.Uuid = fmt.Sprintf("S%s", random.GetNowAndLenRandomString(11))
	session.SendId = req.SendId
	session.ReceiveId = req.ReceiveId
	session.CreatedAt = time.Now()

	// 4. 根据接收者类型设置会话信息
	if req.ReceiveId[0] == 'U' {
		// 用户对用户会话
		receiveUser, err := s.repos.User.FindByUuid(req.ReceiveId)
		if err != nil {
			if errorx.GetCode(err) == errorx.CodeNotFound {
				zap.L().Warn("接收用户不存在",
					zap.String("send_id", req.SendId),
					zap.String("receive_id", req.ReceiveId),
					zap.String("operation", "create_session"),
				)
				return "", errorx.New(errorx.CodeUserNotExist, "接收用户不存在")
			}
			zap.L().Error("查询接收用户失败",
				zap.String("send_id", req.SendId),
				zap.String("receive_id", req.ReceiveId),
				zap.Error(err),
			)
			return "", errorx.ErrServerBusy
		}
		if receiveUser.Status == user_status_enum.DISABLE {
			zap.L().Warn("接收用户已被禁用",
				zap.String("send_id", req.SendId),
				zap.String("receive_id", req.ReceiveId),
			)
			return "", errorx.New(errorx.CodeInvalidParam, "该用户被禁用了")
		}
		session.ReceiveName = receiveUser.Nickname
		session.Avatar = receiveUser.Avatar
	} else {
		// 用户对群组会话
		receiveGroup, err := s.repos.Group.FindByUuid(req.ReceiveId)
		if err != nil {
			if errorx.GetCode(err) == errorx.CodeNotFound {
				zap.L().Warn("接收群组不存在",
					zap.String("send_id", req.SendId),
					zap.String("receive_id", req.ReceiveId),
					zap.String("operation", "create_session"),
				)
				return "", errorx.New(errorx.CodeNotFound, "接收群组不存在")
			}
			zap.L().Error("查询接收群组失败",
				zap.String("send_id", req.SendId),
				zap.String("receive_id", req.ReceiveId),
				zap.Error(err),
			)
			return "", errorx.ErrServerBusy
		}
		if receiveGroup.Status == group_status_enum.DISABLE {
			zap.L().Warn("接收群组已被禁用",
				zap.String("send_id", req.SendId),
				zap.String("receive_id", req.ReceiveId),
			)
			return "", errorx.New(errorx.CodeInvalidParam, "该群聊被禁用了")
		}
		session.ReceiveName = receiveGroup.Name
		session.Avatar = receiveGroup.Avatar
	}

	// 5. 创建会话
	if err := s.repos.Session.CreateSession(&session); err != nil {
		zap.L().Error("创建会话失败",
			zap.String("send_id", req.SendId),
			zap.String("receive_id", req.ReceiveId),
			zap.String("session_id", session.Uuid),
			zap.Error(err),
		)
		return "", errorx.ErrServerBusy
	}

	// 6. 异步清理缓存
	s.cache.SubmitTask(func() {
		s.clearSessionCacheForUser(req.SendId)
	})

	zap.L().Info("会话创建成功",
		zap.String("send_id", req.SendId),
		zap.String("receive_id", req.ReceiveId),
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
	// 1. 检查联系人关系状态 (保持数据库查询，确保实时性)
	contact, err := s.repos.Contact.FindByUserIdAndContactId(sendId, receiveId)
	if err != nil {
		zap.L().Error("查询联系人关系失败",
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

	// 2. 检查接收方(用户或群组)是否可用 (使用缓存优化)
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

// checkTargetStatusWithCache 检查目标(用户或群组)状态，优先查缓存
func (s *sessionService) checkTargetStatusWithCache(targetId string) error {
	if len(targetId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "目标ID为空")
	}

	// 处理用户
	if targetId[0] == 'U' {
		key := "user_info_" + targetId
		// 1. 尝试从 Redis 获取
		if val, err := s.cache.Get(context.Background(), key); err == nil && val != "" {
			var userRsp respond.GetUserInfoRespond
			if err := json.Unmarshal([]byte(val), &userRsp); err == nil {
				if userRsp.Status == user_status_enum.DISABLE {
					return errorx.New(errorx.CodeInvalidParam, "对方已被禁用，无法发起会话")
				}
				return nil // 缓存命中且状态正常
			}
		}

		// 2. 缓存未命中，查库
		user, err := s.repos.User.FindByUuid(targetId)
		if err != nil {
			if errorx.GetCode(err) == errorx.CodeNotFound {
				return errorx.New(errorx.CodeUserNotExist, "对方用户不存在")
			}
			return errorx.ErrServerBusy
		}
		if user.Status == user_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "对方已被禁用，无法发起会话")
		}
		return nil
	}

	// 处理群组
	if targetId[0] == 'G' {
		key := "group_info_" + targetId
		// 1. 尝试从 Redis 获取
		if val, err := s.cache.Get(context.Background(), key); err == nil && val != "" {
			var groupRsp respond.GetGroupInfoRespond
			if err := json.Unmarshal([]byte(val), &groupRsp); err == nil {
				if groupRsp.Status == group_status_enum.DISABLE {
					return errorx.New(errorx.CodeInvalidParam, "对方群组已被禁用，无法发起会话")
				}
				return nil // 缓存命中且状态正常
			}
		}

		// 2. 缓存未命中，查库
		group, err := s.repos.Group.FindByUuid(targetId)
		if err != nil {
			if errorx.GetCode(err) == errorx.CodeNotFound {
				return errorx.New(errorx.CodeNotFound, "对方群组不存在")
			}
			return errorx.ErrServerBusy
		}
		if group.Status == group_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "对方群组已被禁用，无法发起会话")
		}
		return nil
	}

	// 未知类型，保守起见放行或报错？这里假设ID格式正确，或者报错
	return errorx.New(errorx.CodeInvalidParam, "无效的目标ID格式")
}

// OpenSession 打开会话
func (s *sessionService) OpenSession(req request.OpenSessionRequest) (string, error) {
	cacheKey := "session_" + req.SendId + "_" + req.ReceiveId

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
	session, err := s.repos.Session.FindBySendIdAndReceiveId(req.SendId, req.ReceiveId)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			zap.L().Info("会话没有找到，将新建会话")
			createReq := request.CreateSessionRequest{
				SendId:    req.SendId,
				ReceiveId: req.ReceiveId,
			}
			return s.CreateSession(createReq)
		}
		zap.L().Error(err.Error())
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

// GetUserSessionList 获取用户会话列表
func (s *sessionService) GetUserSessionList(ownerId string) ([]respond.UserSessionListRespond, error) {
	cacheKey := "direct_session_list_" + ownerId

	// 1. 尝试读缓存
	rspString, err := s.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var rsp []respond.UserSessionListRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp, nil
		}
		// 反序列化失败，记录日志并降级查库
		zap.L().Error("Unmarshal user session list cache failed", zap.Error(err))
	} else if err != nil {
		// Redis 报错（非key不存在），记录日志
		zap.L().Error(err.Error())
	}

	// 2. 查库（缓存Miss或反序列化失败）
	sessionList, err := s.repos.Session.FindBySendId(ownerId)
	if err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}

	sessionListRsp := make([]respond.UserSessionListRespond, 0, len(sessionList))
	for i := 0; i < len(sessionList); i++ {
		// 增加长度判断防止 panic
		if len(sessionList[i].ReceiveId) > 0 && sessionList[i].ReceiveId[0] == 'U' {
			sessionListRsp = append(sessionListRsp, respond.UserSessionListRespond{
				SessionId: sessionList[i].Uuid,
				Avatar:    sessionList[i].Avatar,
				UserId:    sessionList[i].ReceiveId,
				Username:  sessionList[i].ReceiveName,
			})
		}
	}

	// 3. 回写缓存
	s.cache.SubmitTask(func() {
		rspBytes, err := json.Marshal(sessionListRsp)
		if err != nil {
			zap.L().Error("Marshal failed", zap.Error(err))
			return
		}
		_ = s.cache.Set(context.Background(), cacheKey, string(rspBytes), time.Minute*constants.REDIS_TIMEOUT)
	})

	return sessionListRsp, nil
}

// GetGroupSessionList 获取群聊会话列表
func (s *sessionService) GetGroupSessionList(ownerId string) ([]respond.GroupSessionListRespond, error) {
	cacheKey := "group_session_list_" + ownerId

	// 1. 尝试读缓存
	rspString, err := s.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var rsp []respond.GroupSessionListRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp, nil
		}
		// 反序列化失败，视为缓存失效，记录日志并降级查库
		zap.L().Error("Unmarshal group session list cache failed", zap.Error(err))
	} else if err != nil {
		// Redis 系统错误
		zap.L().Error(err.Error())
	}

	// 2. 查库（缓存Miss或反序列化失败）
	sessionList, err := s.repos.Session.FindBySendId(ownerId)
	if err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}

	sessionListRsp := make([]respond.GroupSessionListRespond, 0, len(sessionList))
	for i := 0; i < len(sessionList); i++ {
		// 增加长度判断防止 panic
		if len(sessionList[i].ReceiveId) > 0 && sessionList[i].ReceiveId[0] == 'G' {
			sessionListRsp = append(sessionListRsp, respond.GroupSessionListRespond{
				SessionId: sessionList[i].Uuid,
				Avatar:    sessionList[i].Avatar,
				GroupId:   sessionList[i].ReceiveId,
				GroupName: sessionList[i].ReceiveName,
			})
		}
	}

	// 3. 回写缓存
	s.cache.SubmitTask(func() {
		rspBytes, err := json.Marshal(sessionListRsp)
		if err != nil {
			zap.L().Error("Marshal failed", zap.Error(err))
			return
		}
		_ = s.cache.Set(context.Background(), cacheKey, string(rspBytes), time.Minute*constants.REDIS_TIMEOUT)
	})

	return sessionListRsp, nil
}

// DeleteSession 删除会话
func (s *sessionService) DeleteSession(ownerId, sessionId string) error {
	// 建议：生产环境最好校验一下该 sessionId 是否真的属于 ownerId，防止越权删除
	if err := s.repos.Session.SoftDeleteByUuids([]string{sessionId}); err != nil {
		zap.L().Error("删除会话失败",
			zap.String("owner_id", ownerId),
			zap.String("session_id", sessionId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	// 异步清理缓存
	s.cache.SubmitTask(func() {
		s.clearSessionCacheForUser(ownerId)
	})

	return nil
}
