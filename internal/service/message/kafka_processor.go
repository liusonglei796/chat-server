package message

import (
	"context"
	"encoding/json"
	"fmt"
	"kama_chat_server/internal/config"
	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/dto/event"
	messagereq "kama_chat_server/internal/dto/request/message"
	messagersp "kama_chat_server/internal/dto/respond/message"
	cacheutil "kama_chat_server/internal/infrastructure/cache"
	"kama_chat_server/internal/infrastructure/metrics"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status"
	"kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/otel"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type KafkaProcessor struct {
	UpstreamReader   *kafka.Reader
	DownstreamWriter *kafka.Writer
	MessageRepo      repository.MessageRepository
	friendshipRepo   repository.FriendshipRepository
	groupMemberRepo  repository.GroupMemberRepository
	sessionRepo      repository.SessionRepository
	cacheService     repository.AsyncCacheService
	cacheHelper      *cacheutil.Helper
	userRepo         repository.UserRepository
	quit             chan os.Signal
}

func normalizePath(path string) string {
	if strings.HasPrefix(path, "https://cube.elemecdn.com") {
		return path
	}
	idx := strings.Index(path, "/static/")
	if idx == -1 {
		return path
	}
	return path[idx:]
}

func NewKafkaProcessor(
	messageRepo repository.MessageRepository,
	friendshipRepo repository.FriendshipRepository,
	groupMemberRepo repository.GroupMemberRepository,
	sessionRepo repository.SessionRepository,
	cacheService repository.AsyncCacheService,
	userRepo repository.UserRepository,
) *KafkaProcessor {
	var helper *cacheutil.Helper
	if cacheService != nil {
		helper = cacheutil.NewHelper(cacheService)
	}

	kafkaConfig := config.GetConfig().KafkaConfig

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaConfig.HostPort},
		Topic:          "chat_upstream",
		CommitInterval: kafkaConfig.Timeout * time.Second,
		GroupID:        "message_service",
		StartOffset:    kafka.LastOffset,
	})

	writer := &kafka.Writer{
		Addr:                   kafka.TCP(kafkaConfig.HostPort),
		Topic:                  "chat_downstream",
		Balancer:               &kafka.Hash{},
		WriteTimeout:           kafkaConfig.Timeout * time.Second,
		RequiredAcks:           kafka.RequireNone,
		AllowAutoTopicCreation: true,
	}

	return &KafkaProcessor{
		UpstreamReader:   reader,
		DownstreamWriter: writer,
		MessageRepo:      messageRepo,
		friendshipRepo:   friendshipRepo,
		groupMemberRepo:  groupMemberRepo,
		sessionRepo:      sessionRepo,
		cacheService:     cacheService,
		cacheHelper:      helper,
		userRepo:         userRepo,
		quit:             make(chan os.Signal, 1),
	}
}

func (k *KafkaProcessor) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error(fmt.Sprintf("kafka server panic: %v", r))
			}
		}()
		for {
			kafkaMessage, err := k.UpstreamReader.ReadMessage(context.Background())
			if err != nil {
				zap.L().Error("service error", zap.Error(err))
				metrics.KafkaConsumeErrors.Inc()
				continue
			}

			messageData := kafkaMessage.Value
			var chatMessageReq messagereq.ChatMessageRequest
			if err := json.Unmarshal(messageData, &chatMessageReq); err != nil {
				zap.L().Error("service error", zap.Error(err))
				continue
			}

			msgCtx := otel.ExtractTraceContext(context.Background(), kafkaMessage.Headers)

			switch chatMessageReq.Type {
			case message_type.Text:
				k.handleTextMessage(msgCtx, chatMessageReq)
			case message_type.File:
				k.handleFileMessage(msgCtx, chatMessageReq)
			case message_type.AudioOrVideo:
				k.handleAVMessage(msgCtx, chatMessageReq)
			}
		}
	}()
}

func (k *KafkaProcessor) Close() {
	if k.UpstreamReader != nil {
		k.UpstreamReader.Close()
	}
	if k.DownstreamWriter != nil {
		k.DownstreamWriter.Close()
	}
}

func trySendBack(k *KafkaProcessor, targetUserId string, messageUuid string, payload []byte) {
	pe := event.PushEvent{
		TargetUserId: targetUserId,
		Payload:      payload,
		MessageUuid:  messageUuid,
	}
	b, _ := json.Marshal(pe)
	if err := k.DownstreamWriter.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(targetUserId),
		Value: b,
	}); err != nil {
		zap.L().Error("write to downstream error", zap.Error(err))
		metrics.MessagesDropped.Inc()
	} else {
		metrics.MessagesDispatched.Inc()
	}
}

func (k *KafkaProcessor) PushRecallNotify(messageUuid, receiveId string) {
	recallMsg := map[string]interface{}{
		"type":         message_type.Recall,
		"message_uuid": messageUuid,
	}
	jsonMsg, err := json.Marshal(recallMsg)
	if err != nil {
		zap.L().Error("序列化撤回通知失败", zap.Error(err))
		return
	}
	trySendBack(k, receiveId, "", jsonMsg)
}

func (k *KafkaProcessor) isDuplicateMessage(clientMsgId string) (isDuplicate bool, isDegraded bool) {
	if clientMsgId == "" {
		return false, false
	}
	dedupKey := "msg:dedup:" + clientMsgId
	ok, err := k.cacheService.SetNX(context.Background(), dedupKey, "1", 24*time.Hour)
	if err != nil {
		zap.L().Error("幂等检查失败，降级放行", zap.String("client_msg_id", clientMsgId), zap.Error(err))
		metrics.MessagesDegrade.Inc()
		return false, true
	}
	if !ok {
		metrics.MessagesDuplicated.Inc()
	}
	return !ok, false
}

func (k *KafkaProcessor) checkSendPermission(sendId, receiveId string) error {
	ctx := context.Background()
	if len(receiveId) == 0 {
		return fmt.Errorf("接收者ID不能为空")
	}

	if k.userRepo != nil {
		sender, err := k.userRepo.FindByUuid(ctx, sendId)
		if err != nil {
			zap.L().Error("查询发送者信息失败", zap.String("sendId", sendId), zap.Error(err))
			return fmt.Errorf("权限校验失败")
		}
		if sender.Status == user_status.DISABLE {
			zap.L().Warn("被禁用的用户尝试发送消息", zap.String("sendId", sendId))
			return fmt.Errorf("您的账号已被禁用，无法发送消息")
		}
	}

	if receiveId[0] == 'U' {
		if k.friendshipRepo != nil {
			isFriend, err := k.friendshipRepo.IsFriend(ctx, sendId, receiveId)
			if err != nil {
				zap.L().Error("检查好友关系失败", zap.String("sendId", sendId), zap.String("receiveId", receiveId), zap.Error(err))
				return fmt.Errorf("权限校验失败")
			}
			if !isFriend {
				return fmt.Errorf("你们还不是好友，无法发送消息")
			}
		}
	} else if receiveId[0] == 'G' {
		if k.groupMemberRepo != nil {
			_, err := k.groupMemberRepo.FindByGroupAndUser(ctx, receiveId, sendId)
			if err != nil {
				return fmt.Errorf("你不是该群成员，无法发送消息")
			}
		}
	}

	return nil
}

func (k *KafkaProcessor) sendPermissionError(sendId, reason string) {
	errMsg := map[string]interface{}{
		"type":    "error",
		"message": reason,
	}
	jsonMsg, err := json.Marshal(errMsg)
	if err != nil {
		return
	}
	trySendBack(k, sendId, "", jsonMsg)
}

func (k *KafkaProcessor) buildMessageFromRequest(req messagereq.ChatMessageRequest) model.Message {
	return model.Message{
		Uuid:        "M" + snowflake.GenerateIDString(),
		ClientMsgId: req.ClientMsgId,
		SessionId:   req.SessionId,
		Type:        req.Type,
		Content:     req.Content,
		Url:         req.Url,
		SendId:      req.SendId,
		SendName:    req.SendName,
		SendAvatar:  normalizePath(req.SendAvatar),
		ReceiveId:   req.ReceiveId,
		FileSize:    req.FileSize,
		FileType:    req.FileType,
		FileName:    req.FileName,
		Status:      message_status.Unsent,
		AVdata:      req.AVdata,
	}
}

func (k *KafkaProcessor) persistMessage(message *model.Message) {
	if k.MessageRepo != nil {
		if err := k.MessageRepo.Create(context.Background(), message); err != nil {
			zap.L().Error("创建消息失败", zap.Error(err))
		}
	}
}

func (k *KafkaProcessor) updateSessionLastMessage(message *model.Message, content string) {
	if k.sessionRepo == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := k.sessionRepo.UpdateLastMessage(ctx,
			message.SendId,
			message.ReceiveId,
			content,
			message.Type,
			message.CreatedAt,
		); err != nil {
			zap.L().Error("更新发送者会话最后消息失败", zap.Error(err))
		}

		if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
			if err := k.sessionRepo.UpdateLastMessage(ctx,
				message.ReceiveId,
				message.SendId,
				content,
				message.Type,
				message.CreatedAt,
			); err != nil {
				zap.L().Warn("更新接收者会话最后消息失败", zap.Error(err))
			}
		}
	}()
}

func (k *KafkaProcessor) handleTextMessage(ctx context.Context, req messagereq.ChatMessageRequest) {
	isDup, _ := k.isDuplicateMessage(req.ClientMsgId)
	if isDup {
		return
	}
	metrics.MessagesConsumed.WithLabelValues("text").Inc()
	if err := k.checkSendPermission(req.SendId, req.ReceiveId); err != nil {
		k.sendPermissionError(req.SendId, err.Error())
		return
	}
	message := k.buildMessageFromRequest(req)
	message.Url = ""
	message.FileSize = "0B"
	message.FileType = ""
	message.FileName = ""
	message.AVdata = ""
	k.persistMessage(&message)
	k.updateSessionLastMessage(&message, message.Content)

	if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
		k.dispatchToUser(message, req.SendAvatar)
	} else if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'G' {
		k.dispatchToGroup(message, req.SendAvatar)
	}
}

func (k *KafkaProcessor) handleFileMessage(ctx context.Context, req messagereq.ChatMessageRequest) {
	isDup, _ := k.isDuplicateMessage(req.ClientMsgId)
	if isDup {
		return
	}
	metrics.MessagesConsumed.WithLabelValues("file").Inc()
	if err := k.checkSendPermission(req.SendId, req.ReceiveId); err != nil {
		k.sendPermissionError(req.SendId, err.Error())
		return
	}
	message := k.buildMessageFromRequest(req)
	message.Content = ""
	message.AVdata = ""
	k.persistMessage(&message)
	content := "[文件] " + req.FileName
	k.updateSessionLastMessage(&message, content)

	if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
		k.dispatchToUser(message, req.SendAvatar)
	} else if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'G' {
		k.dispatchToGroup(message, req.SendAvatar)
	}
}

func (k *KafkaProcessor) handleAVMessage(ctx context.Context, req messagereq.ChatMessageRequest) {
	isDup, _ := k.isDuplicateMessage(req.ClientMsgId)
	if isDup {
		return
	}
	metrics.MessagesConsumed.WithLabelValues("audio_video").Inc()
	if err := k.checkSendPermission(req.SendId, req.ReceiveId); err != nil {
		k.sendPermissionError(req.SendId, err.Error())
		return
	}
	var avData messagereq.AVSignalData
	if err := json.Unmarshal([]byte(req.AVdata), &avData); err != nil {
		return
	}
	message := k.buildMessageFromRequest(req)
	message.Content = ""

	if avData.MessageId == "PROXY" && (avData.Type == "start_call" || avData.Type == "receive_call" || avData.Type == "reject_call") {
		if k.MessageRepo != nil {
			_ = k.MessageRepo.Create(context.Background(), &message)
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
		k.updateSessionLastMessage(&message, summary)
	}

	if len(req.ReceiveId) > 0 && req.ReceiveId[0] == 'U' {
		messageRsp := messagersp.AVMessageRespond{
			SendId:     message.SendId,
			SendName:   message.SendName,
			SendAvatar: message.SendAvatar,
			ReceiveId:  message.ReceiveId,
			Type:       message.Type,
			Content:    message.Content,
			Url:        message.Url,
			FileSize:   message.FileSize,
			FileName:   message.FileName,
			FileType:   message.FileType,
			CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
			AVdata:     message.AVdata,
		}
		jsonMessage, _ := json.Marshal(messageRsp)
		trySendBack(k, message.ReceiveId, message.Uuid, jsonMessage)
	}
}

func (k *KafkaProcessor) dispatchToUser(message model.Message, originalAvatar string) {
	messageRsp := messagersp.GetMessageListRespond{
		SendId:     message.SendId,
		SendName:   message.SendName,
		SendAvatar: originalAvatar,
		ReceiveId:  message.ReceiveId,
		Type:       message.Type,
		Content:    message.Content,
		Url:        message.Url,
		FileSize:   message.FileSize,
		FileName:   message.FileName,
		FileType:   message.FileType,
		CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	jsonMessage, _ := json.Marshal(messageRsp)
	
	trySendBack(k, message.ReceiveId, message.Uuid, jsonMessage)
	trySendBack(k, message.SendId, message.Uuid, jsonMessage)

	if k.cacheService != nil {
		k.cacheService.SubmitTask(func() {
			id1, id2 := message.SendId, message.ReceiveId
			if id1 > id2 {
				id1, id2 = id2, id1
			}
			key := constants.CacheKeyMessageList + id1 + "_" + id2
			rspString, err := k.cacheService.GetOrError(context.Background(), key)
			if err == nil {
				var list []messagersp.GetMessageListRespond
				if err := json.Unmarshal([]byte(rspString), &list); err == nil {
					list = append(list, messageRsp)
					if rspByte, err := json.Marshal(list); err == nil {
						_ = k.cacheService.Set(context.Background(), key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
					}
				}
			}
		})
	}
}

func (k *KafkaProcessor) dispatchToGroup(message model.Message, originalAvatar string) {
	messageRsp := messagersp.GetMessageListRespond{
		SendId:     message.SendId,
		SendName:   message.SendName,
		SendAvatar: originalAvatar,
		ReceiveId:  message.ReceiveId,
		Type:       message.Type,
		Content:    message.Content,
		Url:        message.Url,
		FileSize:   message.FileSize,
		FileName:   message.FileName,
		FileType:   message.FileType,
		CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	jsonMessage, _ := json.Marshal(messageRsp)

	var groupMembers []model.GroupMember
	if k.groupMemberRepo != nil {
		groupMembers = k.getGroupMembersCached(message.ReceiveId)
	}

	for _, gm := range groupMembers {
		trySendBack(k, gm.UserUuid, message.Uuid, jsonMessage)
	}

	if k.cacheService != nil {
		k.cacheService.SubmitTask(func() {
			key := constants.CacheKeyGroupMessageList + message.ReceiveId
			rspString, err := k.cacheService.GetOrError(context.Background(), key)
			if err == nil {
				var list []messagersp.GetMessageListRespond
				if err := json.Unmarshal([]byte(rspString), &list); err == nil {
					list = append(list, messageRsp)
					if rspByte, err := json.Marshal(list); err == nil {
						_ = k.cacheService.Set(context.Background(), key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
					}
				}
			}
		})
	}
}

func (k *KafkaProcessor) getGroupMembersCached(groupId string) []model.GroupMember {
	if k.cacheHelper == nil {
		if k.groupMemberRepo != nil {
			members, _ := k.groupMemberRepo.FindByGroupUuid(context.Background(), groupId)
			return members
		}
		return nil
	}

	cacheKey := constants.CacheKeyGroupMembers + groupId
	fetchFn := func(ctx context.Context) (interface{}, error) {
		return k.groupMemberRepo.FindByGroupUuid(ctx, groupId)
	}
	var members []model.GroupMember
	err := k.cacheHelper.GetOrLoad(context.Background(), cacheKey, fetchFn, 24*time.Hour, 0, &members)
	if err != nil {
		zap.L().Error("获取群成员失败", zap.Error(err))
		if k.groupMemberRepo != nil {
			members, _ = k.groupMemberRepo.FindByGroupUuid(context.Background(), groupId)
		}
	}
	return members
}
