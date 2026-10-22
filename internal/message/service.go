package message

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	bwsnowflake "github.com/bwmarrin/snowflake"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	userpb "kama_chat_server/api/gen/user"
	cacheinfra "kama_chat_server/internal/infra/cache"
	configinfra "kama_chat_server/internal/infra/config"
	grpcinfra "kama_chat_server/internal/infra/grpc_client"
	metricsinfra "kama_chat_server/internal/infra/metrics"
	snowflakeinfra "kama_chat_server/internal/infra/snowflake"
	"kama_chat_server/pkg/constants"
	msgtype "kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// MessageService 消息业务逻辑实现
type MessageService struct {
	messageStore MessageStore
	sessionStore SessionStore
	cache        cacheinfra.AsyncCacheService
	guard        idempotent.Guard
	// pushRecallNotify 撤回通知回调
	pushRecallNotify func(messageUuid, receiveId string)
}

// NewMessageService 构造函数，注入所有依赖
func NewMessageService(
	messageStore MessageStore,
	sessionStore SessionStore,
	cacheService cacheinfra.AsyncCacheService,
	guard ...idempotent.Guard,
) *MessageService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if ig, ok := cacheService.(idempotent.Guard); ok && ig != nil {
		g = ig
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc != nil && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	m := &MessageService{
		messageStore: messageStore,
		sessionStore: sessionStore,
		cache:        cacheService,
		guard:        g,
	}
	m.pushRecallNotify = m.PushRecallNotify
	return m
}

// SetPushRecallNotify 注入撤回通知回调
func (m *MessageService) SetPushRecallNotify(notify func(messageUuid, receiveId string)) {
	m.pushRecallNotify = notify
}

// GetMessageList 获取聊天记录（分页）
func (m *MessageService) GetMessageList(ctx context.Context, requesterId, partnerId string, page, pageSize int) ([]GetMessageListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	fsStatus, err := grpcinfra.CheckFriendshipStatus(ctx, requesterId, partnerId)
	if err != nil {
		zap.L().Error("check friendship via grpc error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}
	if fsStatus != 1 {
		return nil, 0, errorx.New(errorx.CodeForbidden, "你们不是好友，无法查看聊天记录")
	}

	userOneId := requesterId
	if userOneId > partnerId {
		userOneId, partnerId = partnerId, userOneId
	}

	messageList, total, err := m.messageStore.FindByUserIdsPaged(ctx, userOneId, partnerId, page, pageSize)
	if err != nil {
		zap.L().Error("find messages by user ids error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	return m.hydrateMessages(ctx, messageList), total, nil
}

// GetGroupMessageList 获取群聊消息记录（分页）
func (m *MessageService) GetGroupMessageList(ctx context.Context, userId, groupId string, page, pageSize int) ([]GetMessageListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	_, err := m.sessionStore.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, 0, errorx.New(errorx.CodeForbidden, "您没有该群的会话记录")
		}
		zap.L().Error("Find session error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	messageList, total, err := m.messageStore.FindByGroupIdPaged(ctx, groupId, page, pageSize)
	if err != nil {
		zap.L().Error("find group messages error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	return m.hydrateMessages(ctx, messageList), total, nil
}

// ResolveSessionId 归一化会话唯一标识：
// 1. 群聊：统一为 groupId（G开头）
// 2. 单聊：按字典序组合双方 UUID 为 P_{min}_{max}，保证双向查询与存储完全一致
func ResolveSessionId(sendId, receiveId string) string {
	if strings.HasPrefix(receiveId, "G") {
		return receiveId
	}
	if sendId > receiveId {
		sendId, receiveId = receiveId, sendId
	}
	return fmt.Sprintf("P_%s_%s", sendId, receiveId)
}

// snowflakeUuidToScore 将消息 UUID 或游标转换为 Redis ZSET score（微秒安全精度）
func snowflakeUuidToScore(uuid string) float64 {
	if uuid == "" {
		return 0
	}
	if strings.HasPrefix(uuid, "M") {
		raw := strings.TrimPrefix(uuid, "M")
		if sfId, err := bwsnowflake.ParseString(raw); err == nil {
			return float64(sfId.Time())*1000 + float64(int64(sfId)&0xfff)
		}
	}
	if ts, err := strconv.ParseInt(uuid, 10, 64); err == nil {
		if ts < 1e11 {
			return float64(ts * 1000000)
		} else if ts < 1e14 {
			return float64(ts * 1000)
		}
		return float64(ts)
	}
	return 0
}

func (m *MessageService) cacheMessageToZSet(message *Message) {
	if m.cache == nil || message.SessionId == "" {
		return
	}
	msgCopy := *message
	m.cache.SubmitTask(func() {
		key := constants.CacheKeyMessageZSet + msgCopy.SessionId
		var score float64
		if msgCopy.SeqId > 0 {
			score = float64(msgCopy.SeqId)
		} else {
			score = snowflakeUuidToScore(msgCopy.Uuid)
		}
		if score <= 0 {
			score = float64(msgCopy.CreatedAt.UnixMicro())
		}
		if score <= 0 {
			score = float64(time.Now().UnixMicro())
		}
		data, err := json.Marshal(msgCopy)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = m.cache.ZAdd(ctx, key, score, string(data))
		_ = m.cache.ZRemRangeByRank(ctx, key, 0, -201)
		_ = m.cache.Expire(ctx, key, 7*24*time.Hour)
	})
}

func (m *MessageService) updateRecalledMessageInZSet(sessionId, messageUuid string) {
	if m.cache == nil || sessionId == "" || messageUuid == "" {
		return
	}
	m.cache.SubmitTask(func() {
		key := constants.CacheKeyMessageZSet + sessionId
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		members, err := m.cache.ZRevRangeByScore(ctx, key, "+inf", "-inf", 0, 200)
		if err != nil || len(members) == 0 {
			return
		}
		for _, member := range members {
			var msg Message
			if err := json.Unmarshal([]byte(member), &msg); err == nil && msg.Uuid == messageUuid {
				_ = m.cache.ZRem(ctx, key, member)
				msg.Content = ""
				msg.Type = int8(msgtype.Recall)
				var score float64
				if msg.SeqId > 0 {
					score = float64(msg.SeqId)
				} else {
					score = snowflakeUuidToScore(msg.Uuid)
				}
				if updatedData, err := json.Marshal(msg); err == nil {
					_ = m.cache.ZAdd(ctx, key, score, string(updatedData))
				}
				break
			}
		}
	})
}

func (m *MessageService) getMessagesBySessionIdCursorWithCache(ctx context.Context, sessionId, cursor string, pageSize int) ([]GetMessageListRespond, string, bool, error) {
	key := constants.CacheKeyMessageZSet + sessionId

	if m.cache != nil {
		maxScore := "+inf"
		if cursor != "" {
			if score := snowflakeUuidToScore(cursor); score > 0 {
				maxScore = fmt.Sprintf("(%f", score)
			}
		}

		members, err := m.cache.ZRevRangeByScore(ctx, key, maxScore, "-inf", 0, int64(pageSize+1))
		if err == nil && len(members) > 0 {
			var cachedMessages []Message
			for _, item := range members {
				var msg Message
				if err := json.Unmarshal([]byte(item), &msg); err == nil {
					cachedMessages = append(cachedMessages, msg)
				}
			}

			if len(cachedMessages) > 0 {
				hasMore := len(cachedMessages) > pageSize
				if hasMore {
					cachedMessages = cachedMessages[:pageSize]
					nextCursor := resolveNextCursor(cachedMessages)
					return m.hydrateMessages(ctx, cachedMessages), nextCursor, true, nil
				}

				totalInCache, cardErr := m.cache.ZCard(ctx, key)
				if cardErr == nil && totalInCache < 200 {
					return m.hydrateMessages(ctx, cachedMessages), "", false, nil
				}
			}
		}
	}

	result, err := m.messageStore.FindBySessionIdCursor(ctx, sessionId, cursor, pageSize)
	if err != nil {
		zap.L().Error("find messages by session id cursor error", zap.String("sessionId", sessionId), zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	if m.cache != nil && cursor == "" && len(result.Messages) > 0 {
		messagesToCache := make([]Message, len(result.Messages))
		copy(messagesToCache, result.Messages)
		m.cache.SubmitTask(func() {
			for _, msg := range messagesToCache {
				var score float64
				if msg.SeqId > 0 {
					score = float64(msg.SeqId)
				} else {
					score = snowflakeUuidToScore(msg.Uuid)
				}
				if score <= 0 {
					score = float64(msg.CreatedAt.UnixMicro())
				}
				if data, err := json.Marshal(msg); err == nil {
					_ = m.cache.ZAdd(context.Background(), key, score, string(data))
				}
			}
			_ = m.cache.ZRemRangeByRank(context.Background(), key, 0, -201)
			_ = m.cache.Expire(context.Background(), key, 7*24*time.Hour)
		})
	}

	return m.hydrateMessages(ctx, result.Messages), result.NextCursor, result.HasMore, nil
}

// GetMessageListCursor 获取两个用户之间的聊天记录（游标分页，统一 session_id + ZSET 缓存）
func (m *MessageService) GetMessageListCursor(ctx context.Context, requesterId, partnerId, cursor string, pageSize int) ([]GetMessageListRespond, string, bool, error) {
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	fsStatus, err := grpcinfra.CheckFriendshipStatus(ctx, requesterId, partnerId)
	if err != nil {
		zap.L().Error("check friendship via grpc error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}
	if fsStatus != 1 {
		return nil, "", false, errorx.New(errorx.CodeForbidden, "你们不是好友，无法查看聊天记录")
	}

	sessionId := ResolveSessionId(requesterId, partnerId)
	return m.getMessagesBySessionIdCursorWithCache(ctx, sessionId, cursor, pageSize)
}

// GetGroupMessageListCursor 获取群聊消息记录（游标分页，统一 session_id + ZSET 缓存）
func (m *MessageService) GetGroupMessageListCursor(ctx context.Context, userId, groupId, cursor string, pageSize int) ([]GetMessageListRespond, string, bool, error) {
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	_, err := m.sessionStore.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, "", false, errorx.New(errorx.CodeForbidden, "您没有该群的会话记录")
		}
		zap.L().Error("Find session error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	sessionId := groupId
	return m.getMessagesBySessionIdCursorWithCache(ctx, sessionId, cursor, pageSize)
}

// UploadAvatar 上传头像
func (m *MessageService) UploadAvatar(ctx context.Context, c *gin.Context) (string, error) {
	if err := c.Request.ParseMultipartForm(constants.FILE_MAX_SIZE); err != nil {
		zap.L().Error("parse multipart form error", zap.Error(err))
		return "", errorx.New(errorx.CodeInvalidParam, "文件过大，请上传小于 30MB 的文件")
	}
	mForm := c.Request.MultipartForm
	if len(mForm.File) == 0 {
		return "", errorx.New(errorx.CodeInvalidParam, "no file uploaded")
	}

	for _, headers := range mForm.File {
		for _, fileHeader := range headers {
			if fileHeader.Size > constants.AVATAR_MAX_SIZE {
				return "", errorx.New(errorx.CodeInvalidParam, "头像文件过大，最大支持 5MB")
			}
			filename, err := m.saveFile(fileHeader, configinfra.GetConfig().StaticAvatarPath, "image/jpeg", "image/png", "image/gif")
			if err != nil {
				zap.L().Error("save avatar error", zap.Error(err))
				if errorx.GetCode(err) == errorx.CodeInvalidParam {
					continue
				}
				return "", errorx.ErrServerBusy
			}
			zap.L().Info("upload avatar success", zap.String("filename", filename))
			return filename, nil
		}
	}
	return "", errorx.New(errorx.CodeInvalidParam, "no file found")
}

// UploadFile 上传文件
func (m *MessageService) UploadFile(ctx context.Context, c *gin.Context) ([]string, error) {
	if err := c.Request.ParseMultipartForm(constants.FILE_MAX_SIZE); err != nil {
		zap.L().Error("parse multipart form error", zap.Error(err))
		return nil, errorx.New(errorx.CodeInvalidParam, "文件过大，请上传小于 30MB 的文件")
	}

	var uploadedFiles []string
	dstDir := configinfra.GetConfig().StaticFilePath
	mForm := c.Request.MultipartForm

	for _, headers := range mForm.File {
		for _, fileHeader := range headers {
			if fileHeader.Size > constants.UPLOAD_FILE_MAX_SIZE {
				for _, f := range uploadedFiles {
					_ = os.Remove(filepath.Join(dstDir, f))
				}
				return nil, errorx.New(errorx.CodeInvalidParam, "单个文件过大，最大支持 30MB")
			}

			filename, err := m.saveFile(fileHeader, dstDir)
			if err != nil {
				zap.L().Error("save file error", zap.Error(err))
				for _, f := range uploadedFiles {
					_ = os.Remove(filepath.Join(dstDir, f))
				}
				return nil, errorx.ErrServerBusy
			}

			zap.L().Info("upload file success", zap.String("filename", filename), zap.Int64("size", fileHeader.Size))
			uploadedFiles = append(uploadedFiles, filename)
		}
	}

	return uploadedFiles, nil
}

func (m *MessageService) saveFile(fileHeader *multipart.FileHeader, dstDir string, allowedMimes ...string) (string, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	buffer := make([]byte, 512)
	if _, err := src.Read(buffer); err != nil && err != io.EOF {
		return "", err
	}
	contentType := http.DetectContentType(buffer)

	if _, err := src.Seek(0, 0); err != nil {
		return "", err
	}

	if len(allowedMimes) > 0 {
		isAllowed := false
		for _, mime := range allowedMimes {
			if strings.HasPrefix(contentType, mime) {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			return "", errorx.Newf(errorx.CodeInvalidParam, "invalid file type: %s", contentType)
		}
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	newFileName := snowflakeinfra.GenerateIDString() + ext
	dst := filepath.Join(dstDir, newFileName)

	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return "", err
	}

	return newFileName, nil
}

// RecallMessage 撤回消息
func (m *MessageService) RecallMessage(ctx context.Context, userId string, req RecallMessageRequest) error {
	msg, err := m.messageStore.FindByUuid(ctx, req.MessageUuid)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeInvalidParam, "消息不存在")
		}
		zap.L().Error("查询消息失败", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if msg.SendId != userId {
		return errorx.New(errorx.CodeForbidden, "只能撤回自己发送的消息")
	}

	if msg.Type == int8(msgtype.Recall) {
		return nil
	}

	if time.Since(msg.CreatedAt) > 2*time.Minute {
		return errorx.New(errorx.CodeForbidden, "消息发送超过2分钟，无法撤回")
	}

	ok, err := m.messageStore.UpdateContent(ctx, req.MessageUuid, "", int8(msgtype.Recall))
	if err != nil {
		zap.L().Error("撤回消息失败", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !ok {
		cur, queryErr := m.messageStore.FindByUuid(ctx, req.MessageUuid)
		if queryErr != nil || cur == nil {
			return errorx.New(errorx.CodeNotFound, "消息不存在")
		}
		if cur.SendId != userId {
			return errorx.New(errorx.CodeForbidden, "只能撤回自己发送的消息")
		}
		if time.Since(cur.CreatedAt) > 2*time.Minute {
			return errorx.New(errorx.CodeForbidden, "消息发送超过2分钟，无法撤回")
		}
		if cur.Type == int8(msgtype.Recall) {
			return nil
		}
		return errorx.New(errorx.CodeInvalidParam, "该消息已被撤回")
	}

	if m.pushRecallNotify != nil {
		m.pushRecallNotify(req.MessageUuid, msg.ReceiveId)
		if len(msg.ReceiveId) > 0 && msg.ReceiveId[0] == 'U' && msg.SendId != "" {
			recallMsg := map[string]interface{}{
				"type":         msgtype.Recall,
				"message_uuid": req.MessageUuid,
			}
			if jsonMsg, err := json.Marshal(recallMsg); err == nil {
				m.trySendBack(msg.SendId, "", jsonMsg)
			}
		}
	}

	m.updateSessionLastMessage(msg, "[一条消息已被撤回]")

	sessionId := msg.SessionId
	if sessionId == "" {
		sessionId = ResolveSessionId(msg.SendId, msg.ReceiveId)
	}
	m.updateRecalledMessageInZSet(sessionId, req.MessageUuid)

	return nil
}

// GetMessageByUuid 根据 UUID 获取消息
func (m *MessageService) GetMessageByUuid(ctx context.Context, messageId string) (*Message, error) {
	return m.messageStore.FindByUuid(ctx, messageId)
}

func (m *MessageService) checkSendPermission(ctx context.Context, sendId, receiveId string) error {
	if len(receiveId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "接收者ID不能为空")
	}

	senderStatus, err := grpcinfra.GetUserStatus(ctx, sendId)
	if err == nil && senderStatus == user_status.DISABLE {
		zap.L().Warn("被禁用的用户尝试发送消息", zap.String("sendId", sendId))
		return errorx.New(errorx.CodeForbidden, "您的账号已被禁用，无法发送消息")
	}

	if receiveId[0] == 'U' {
		fsStatus, err := grpcinfra.CheckFriendshipStatus(ctx, sendId, receiveId)
		if err == nil {
			switch fsStatus {
			case 1, 2:
				// 1=正常好友; 2=我拉黑了对方(微信同款特权放行)
			case 3:
				return errorx.New(errorx.CodeForbidden, "消息已发出，但被对方拒收了")
			case 4:
				return errorx.New(errorx.CodeForbidden, "开启了朋友验证，你还不是他（她）朋友")
			default:
				return errorx.New(errorx.CodeForbidden, "你们不是好友，请先添加好友")
			}
		}
	} else if receiveId[0] == 'G' {
		isMember, err := grpcinfra.CheckGroupMember(ctx, receiveId, sendId)
		if err == nil && !isMember {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员，无法发送消息")
		}
	}

	return nil
}

func (m *MessageService) buildMessageFromRequest(req ChatMessageRequest) Message {
	sessionId := req.SessionId
	if sessionId == "" || (!strings.HasPrefix(sessionId, "P_") && !strings.HasPrefix(sessionId, "G")) {
		sessionId = ResolveSessionId(req.SendId, req.ReceiveId)
	}
	return Message{
		Uuid:        "M" + snowflakeinfra.GenerateIDString(),
		ClientMsgId: req.ClientMsgId,
		SessionId:   sessionId,
		Type:        req.Type,
		Content:     req.Content,
		Url:         req.Url,
		SendId:      req.SendId,
		ReceiveId:   req.ReceiveId,
		FileSize:    req.FileSize,
		FileType:    req.FileType,
		FileName:    req.FileName,
		AVdata:      req.AVdata,
	}
}

func (m *MessageService) nextSessionSeq(ctx context.Context, sessionId string) (int64, error) {
	if m.cache != nil {
		key := constants.CacheKeySessionSeq + sessionId
		seq, err := m.cache.Incr(ctx, key)
		if err == nil {
			if seq == 1 {
				maxSeq, dbErr := m.messageStore.GetMaxSeqBySessionId(ctx, sessionId)
				if dbErr == nil && maxSeq >= 1 {
					calibratedSeq := maxSeq + 1
					script := `
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local target = tonumber(ARGV[1])
if current <= target then
    redis.call('SET', KEYS[1], target)
    return target
else
    return redis.call('INCR', KEYS[1])
end`
					if res, evalErr := m.cache.Eval(ctx, script, []string{key}, calibratedSeq); evalErr == nil {
						if num, ok := res.(int64); ok {
							return num, nil
						}
					}
					return calibratedSeq, nil
				}
			}
			return seq, nil
		}
		zap.L().Warn("Redis 生成会话 Seq 失败，降级查 DB 基准", zap.String("sessionId", sessionId), zap.Error(err))
	}

	maxSeq, err := m.messageStore.GetMaxSeqBySessionId(ctx, sessionId)
	if err != nil {
		return 0, err
	}
	return maxSeq + 1, nil
}

func (m *MessageService) trySendBack(targetUserId string, messageUuid string, payload []byte) {
	if m.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	gatewayAddr, err := m.cache.Get(ctx, constants.CacheKeyUserGateway+targetUserId)
	if err != nil || gatewayAddr == "" {
		metricsinfra.MessagesDropped.Inc()
		return
	}

	if err := grpcinfra.PushToGateway(ctx, gatewayAddr, targetUserId, messageUuid, payload); err != nil {
		zap.L().Warn("push to gateway failed",
			zap.String("userId", targetUserId),
			zap.String("gatewayAddr", gatewayAddr),
			zap.Error(err),
		)
		metricsinfra.MessagesDropped.Inc()
		m.cache.SubmitTask(func() {
			_ = m.cache.Delete(context.Background(), constants.CacheKeyUserGateway+targetUserId)
		})
		return
	}

	metricsinfra.MessagesDispatched.Inc()
}

func (m *MessageService) PushRecallNotify(messageUuid, receiveId string) {
	recallMsg := map[string]interface{}{
		"type":         msgtype.Recall,
		"message_uuid": messageUuid,
	}
	jsonMsg, err := json.Marshal(recallMsg)
	if err != nil {
		zap.L().Error("序列化撤回通知失败", zap.Error(err))
		return
	}

	if len(receiveId) > 0 && receiveId[0] == 'G' {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		memberIds, err := grpcinfra.ListGroupMemberIds(ctx, receiveId)
		if err != nil {
			zap.L().Error("get group members via grpc for recall error", zap.Error(err))
		}
		for _, uid := range memberIds {
			m.trySendBack(uid, "", jsonMsg)
		}
		return
	}

	m.trySendBack(receiveId, "", jsonMsg)
}

func (m *MessageService) updateSessionLastMessage(message *Message, content string) {
	if m.sessionStore == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.sessionStore.UpdateLastMessage(ctx,
			message.SendId,
			message.ReceiveId,
			content,
			message.Type,
			message.CreatedAt,
			message.SeqId,
		); err != nil {
			zap.L().Error("更新发送者会话最后消息失败", zap.Error(err))
		}

		if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
			if err := m.sessionStore.UpdateLastMessage(ctx,
				message.ReceiveId,
				message.SendId,
				content,
				message.Type,
				message.CreatedAt,
				message.SeqId,
			); err != nil {
				zap.L().Warn("更新接收者会话最后消息失败", zap.Error(err))
			}
		}
	}()
}

func (m *MessageService) hydrateMessages(ctx context.Context, messages []Message) []GetMessageListRespond {
	sendIds := make([]string, 0, len(messages))
	for _, msg := range messages {
		sendIds = append(sendIds, msg.SendId)
	}

	userMap := make(map[string]*userpb.PublicUserInfo)
	if len(sendIds) > 0 {
		if users, err := grpcinfra.BatchGetPublicUserInfo(ctx, sendIds); err == nil {
			for _, u := range users {
				userMap[u.Uuid] = u
			}
		} else {
			zap.L().Warn("hydrateMessages: BatchGetPublicUserInfo failed", zap.Error(err))
		}
	}

	rspList := make([]GetMessageListRespond, 0, len(messages))
	for _, msg := range messages {
		var sendName, sendAvatar string
		if u, ok := userMap[msg.SendId]; ok {
			sendName = u.Nickname
			sendAvatar = u.Avatar
		}

		rspList = append(rspList, GetMessageListRespond{
			SendId:     msg.SendId,
			SendName:   sendName,
			SendAvatar: sendAvatar,
			ReceiveId:  msg.ReceiveId,
			Content:    msg.Content,
			Url:        msg.Url,
			Type:       msg.Type,
			FileType:   msg.FileType,
			FileName:   msg.FileName,
			FileSize:   msg.FileSize,
			SeqId:      msg.SeqId,
			CreatedAt:  msg.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return rspList
}

func (m *MessageService) dispatchToUser(message Message, sendName, originalAvatar string) {
	messageRsp := GetMessageListRespond{
		SendId:     message.SendId,
		SendName:   sendName,
		SendAvatar: originalAvatar,
		ReceiveId:  message.ReceiveId,
		Type:       message.Type,
		Content:    message.Content,
		Url:        message.Url,
		FileSize:   message.FileSize,
		FileName:   message.FileName,
		FileType:   message.FileType,
		SeqId:      message.SeqId,
		CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	jsonMessage, _ := json.Marshal(messageRsp)

	m.trySendBack(message.ReceiveId, message.Uuid, jsonMessage)
	m.trySendBack(message.SendId, message.Uuid, jsonMessage)
}

func (m *MessageService) dispatchToGroup(ctx context.Context, message Message, sendName, originalAvatar string) {
	messageRsp := GetMessageListRespond{
		SendId:     message.SendId,
		SendName:   sendName,
		SendAvatar: originalAvatar,
		ReceiveId:  message.ReceiveId,
		Type:       message.Type,
		Content:    message.Content,
		Url:        message.Url,
		FileSize:   message.FileSize,
		FileName:   message.FileName,
		FileType:   message.FileType,
		SeqId:      message.SeqId,
		CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	jsonMessage, _ := json.Marshal(messageRsp)

	memberIds, err := grpcinfra.ListGroupMemberIds(ctx, message.ReceiveId)
	if err != nil {
		zap.L().Error("get group members via grpc error", zap.Error(err))
	}
	if len(memberIds) == 0 {
		return
	}

	go func(members []string, payload []byte, msgUuid string) {
		const maxWorkers = 10
		workers := maxWorkers
		if len(members) < workers {
			workers = len(members)
		}
		ch := make(chan string, len(members))
		for _, uid := range members {
			ch <- uid
		}
		close(ch)

		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for uid := range ch {
					m.trySendBack(uid, msgUuid, payload)
				}
			}()
		}
		wg.Wait()
	}(memberIds, jsonMessage, message.Uuid)
}

func (m *MessageService) dispatchAVToUser(message Message, sendName, sendAvatar string) {
	if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
		messageRsp := AVMessageRespond{
			SendId:     message.SendId,
			SendName:   sendName,
			SendAvatar: sendAvatar,
			ReceiveId:  message.ReceiveId,
			Type:       message.Type,
			Content:    message.Content,
			Url:        message.Url,
			FileSize:   message.FileSize,
			FileName:   message.FileName,
			FileType:   message.FileType,
			SeqId:      message.SeqId,
			CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
			AVdata:     message.AVdata,
		}
		jsonMessage, _ := json.Marshal(messageRsp)
		m.trySendBack(message.ReceiveId, message.Uuid, jsonMessage)
	}
}

// SendMessage 上行消息发送
func (m *MessageService) SendMessage(ctx context.Context, req ChatMessageRequest) (*SendMessageRespond, error) {
	execFn := func(execCtx context.Context) (*SendMessageRespond, error) {
		if err := m.checkSendPermission(execCtx, req.SendId, req.ReceiveId); err != nil {
			return nil, err
		}

		message := m.buildMessageFromRequest(req)

		seqId, err := m.nextSessionSeq(execCtx, message.SessionId)
		if err != nil {
			zap.L().Error("生成会话 SeqID 失败", zap.String("sessionId", message.SessionId), zap.Error(err))
			return nil, errorx.ErrServerBusy
		}
		message.SeqId = seqId

		switch req.Type {
		case int8(msgtype.Text):
			metricsinfra.MessagesConsumed.WithLabelValues("text").Inc()
			message.Url = ""
			message.FileSize = "0B"
			message.FileType = ""
			message.FileName = ""
			message.AVdata = ""
			if err := m.messageStore.Create(execCtx, &message); err != nil {
				zap.L().Error("创建文本消息失败", zap.Error(err))
				return nil, errorx.ErrServerBusy
			}
			m.cacheMessageToZSet(&message)
			m.updateSessionLastMessage(&message, message.Content)
			if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
				m.dispatchToUser(message, req.SendName, req.SendAvatar)
			} else if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'G' {
				m.dispatchToGroup(execCtx, message, req.SendName, req.SendAvatar)
			}

		case int8(msgtype.File):
			metricsinfra.MessagesConsumed.WithLabelValues("file").Inc()
			message.Content = ""
			message.AVdata = ""
			if err := m.messageStore.Create(execCtx, &message); err != nil {
				zap.L().Error("创建文件消息失败", zap.Error(err))
				return nil, errorx.ErrServerBusy
			}
			m.cacheMessageToZSet(&message)
			content := "[文件] " + req.FileName
			m.updateSessionLastMessage(&message, content)
			if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
				m.dispatchToUser(message, req.SendName, req.SendAvatar)
			} else if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'G' {
				m.dispatchToGroup(execCtx, message, req.SendName, req.SendAvatar)
			}

		case int8(msgtype.AudioOrVideo):
			metricsinfra.MessagesConsumed.WithLabelValues("audio_video").Inc()
			var avData AVSignalData
			_ = json.Unmarshal([]byte(req.AVdata), &avData)
			message.Content = ""

			if avData.MessageId == "PROXY" && (avData.Type == "start_call" || avData.Type == "receive_call" || avData.Type == "reject_call") {
				if err := m.messageStore.Create(execCtx, &message); err == nil {
					m.cacheMessageToZSet(&message)
				}
				var summary string
				switch avData.Type {
				case "start_call":
					summary = "[通话] 发起了通话"
				case "receive_call":
					summary = "[通话] 已接听"
				case "reject_call":
					summary = "[通话] 已拒绝"
				}
				m.updateSessionLastMessage(&message, summary)
			}
			m.dispatchAVToUser(message, req.SendName, req.SendAvatar)
		default:
			return nil, errorx.New(errorx.CodeInvalidParam, "不支持的消息类型")
		}

		return &SendMessageRespond{
			MessageUuid: message.Uuid,
			SeqId:       message.SeqId,
			CreatedAt:   message.CreatedAt.Format("2006-01-02 15:04:05"),
		}, nil
	}

	if m.guard != nil && req.ClientMsgId != "" {
		key := "msg:dedup:" + req.ClientMsgId
		rsp, isReplay, err := idempotent.ExecuteJSON(ctx, m.guard, key, 5*time.Second, 24*time.Hour, execFn)
		if err != nil {
			return nil, err
		}
		if isReplay {
			metricsinfra.MessagesDuplicated.Inc()
			zap.L().Info("命中消息发送幂等回放，直接返回已发送消息记录",
				zap.String("client_msg_id", req.ClientMsgId),
				zap.String("message_uuid", rsp.MessageUuid),
			)
		}
		return rsp, nil
	}

	return execFn(ctx)
}

// SyncMessages 基于会话 SeqID 增量同步拉取消息
func (m *MessageService) SyncMessages(ctx context.Context, requesterId, sessionId string, afterSeq int64, limit int) ([]GetMessageListRespond, error) {
	if strings.HasPrefix(sessionId, "G") {
		if _, err := m.sessionStore.FindBySendIdAndReceiveId(ctx, requesterId, sessionId); err != nil {
			if errorx.IsNotFound(err) {
				return nil, errorx.New(errorx.CodeForbidden, "您没有该群的会话记录")
			}
			return nil, errorx.ErrServerBusy
		}
	} else if strings.HasPrefix(sessionId, "P_") {
		parts := strings.Split(sessionId, "_")
		if len(parts) != 3 || (parts[1] != requesterId && parts[2] != requesterId) {
			return nil, errorx.New(errorx.CodeForbidden, "您无权访问该单聊会话")
		}
	}

	messages, err := m.messageStore.FindBySessionIdSync(ctx, sessionId, afterSeq, limit)
	if err != nil {
		zap.L().Error("SyncMessages 增量同步失败", zap.String("sessionId", sessionId), zap.Int64("afterSeq", afterSeq), zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	return m.hydrateMessages(ctx, messages), nil
}
