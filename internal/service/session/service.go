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
	// 1. 检查联系人关系状态 (保持数据库查询，确保实时性)
	contact, err := s.repos.Contact.FindByUserIdAndContactId(sendId, receiveId, contact_type_enum.USER)
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
			var userRsp user.GetUserInfoRespond
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
			var groupRsp group.GetGroupInfoRespond
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

// GetUserSessionList 获取用户会话列表（分页）
func (s *sessionService) GetUserSessionList(ownerId string, page, pageSize int) ([]sessionrsp.UserSessionListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// 直接查库（分页场景不利用缓存，因为缓存 key 需要包含分页参数）
	sessionList, total, err := s.repos.Session.FindBySendIdPaged(ownerId, offset, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	sessionListRsp := make([]sessionrsp.UserSessionListRespond, 0, len(sessionList))
	for i := 0; i < len(sessionList); i++ {
		// 只筛选私聊会话（接收者以 'U' 开头）
		if len(sessionList[i].ReceiveId) > 0 && sessionList[i].ReceiveId[0] == 'U' {
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
	offset := (page - 1) * pageSize

	// 直接查库（分页场景不利用缓存）
	sessionList, total, err := s.repos.Session.FindBySendIdPaged(ownerId, offset, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	sessionListRsp := make([]sessionrsp.GroupSessionListRespond, 0, len(sessionList))
	for i := 0; i < len(sessionList); i++ {
		// 只筛选群聊会话（接收者以 'G' 开头）
		if len(sessionList[i].ReceiveId) > 0 && sessionList[i].ReceiveId[0] == 'G' {
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
	}

	return sessionListRsp, total, nil
}

// DeleteSession 删除会话
func (s *sessionService) DeleteSession(ownerId, sessionId string) error {
	// 校验 session 是否属于 ownerId (防止越权删除)
	// FindByUuid 只能查到 session 记录，但我们需要校验 send_id == ownerId
	// 这里我们假设 session 表的主键 uuid 是全局唯一的，
	// 我们可以查出来校验 SendId，或者直接用 Where(uuid, send_id).Delete

	// 为了确保安全，先查一下
	// 注意：SessionRepository.FindBySendIdAndReceiveId 不是查ById
	// 我们需要一个 FindBySessionId 或者直接执行带条件的删除

	// 既然 SoftDeleteByUuids 只接受 uuid list，我们可以在 repo 层加一个带 ownerId 的删除，或者先查后删。
	// 鉴于 SoftDeleteByUuids 比较通用，我们先查。
	// 但 SessionRepository 似乎没有 FindByUuid。
	// 回头看 CreateSession，Session ID 是 create时生成的。
	// 现有的 SoftDeleteByUuids 只是 Delete(Session{}, uuids)

	// 让我们尝试尽量利用现有 repo 接口。
	// 我们可以遍历 owner的所有session来看看有没有这个 sessionId? 不 太慢。
	// 我们应该给 SessionRepository 加一个 FindByUuid 或者 DeleteByUuidAndOwner
	// 但现在只能用 SoftDeleteByUuids。

	// 既然是 Service 层修复，最简单的改法是先查 Owner 的 Session 列表里有没有这个？
	// 或者... 等等，Session 表结构里 SendId 就是 Owner。
	// 所以只要确保这个 Session 的 SendId == ownerId。

	// 由于 repo 接口有限，这里先不做严格校验（需要改 Repo 接口），
	// 或者我们可以用 SessionRepository.FindBySendId(ownerId) 拿到所有，然后在内存里匹配。
	// 但如果 Session 很多会有性能问题。

	// 鉴于时间，我们先遵循"最小改动"，如果 SessionRepository 没有 FindByUuid，
	// 我们暂时不做校验，或者必须加 FindByUuid。
	// 但 SessionRepository 没有 FindByUuid。

	// 让我们看看 DeleteSession 在 Handler 里的调用。
	// 它是 Delete /session/delete?sessionId=xxx

	// 如果无法校验，那就是 IDOR。必须修复。
	// 方案：让 RemoveGroupMembers 那种方式，DeleteByUuids 是通用的。
	// 我们需要一个 CheckSessionOwner(sessionId, ownerId) 或者 DeleteOwnedSession(sessionId, ownerId)。

	// 临时方案：遍历 FindBySendId 的结果 (通常用户 Session 不会太多，几百个顶天了)
	// 虽然不优雅，但无需改 Repo。
	sessions, err := s.repos.Session.FindBySendId(ownerId)
	if err != nil {
		zap.L().Error("Find user sessions error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	isOwner := false
	for _, sess := range sessions {
		if sess.Uuid == sessionId {
			isOwner = true
			break
		}
	}
	if !isOwner {
		return errorx.New(errorx.CodeForbidden, "无权删除该会话或会话不存在")
	}

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
