import re

with open("internal/service/message/kafka_processor.go", "r") as f:
    text = f.read()

text = text.replace("package chat", "package message")
text = text.replace("MsgConsumer", "KafkaProcessor")
text = text.replace('"kama_chat_server/internal/model"', '"kama_chat_server/internal/model"\n\t"kama_chat_server/internal/dto/event"\n\t"kama_chat_server/internal/config"\n')

# Replace the struct
struct_pattern = re.compile(r'type KafkaProcessor struct \{.*?\n\}', re.DOTALL)
new_struct = """type KafkaProcessor struct {
	UpstreamReader   *kafka.Reader
	DownstreamWriter *kafka.Writer
	MessageRepo     repository.MessageRepository
	friendshipRepo  repository.FriendshipRepository
	groupMemberRepo repository.GroupMemberRepository
	sessionRepo     repository.SessionRepository
	cacheService    repository.AsyncCacheService
	cacheHelper     *cacheutil.Helper
	userRepo        repository.UserRepository
	quit            chan os.Signal
}"""
text = struct_pattern.sub(new_struct, text)

# Replace NewKafkaProcessor
new_new = """func NewKafkaProcessor(
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
}"""
text = re.sub(r'func NewKafkaProcessor.*?return &KafkaProcessor\{.*?\}\n\}', new_new, text, flags=re.DOTALL)

# Remove Publish
text = re.sub(r'// Publish.*?func \(k \*KafkaProcessor\) Publish.*?return k\.kafkaClient\.Producer\.WriteMessages\(ctx, kafka\.Message\{.*?\n\}\n\}', '', text, flags=re.DOTALL)

# Replace Start
new_start = """func (k *KafkaProcessor) Start() {
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
	<-k.quit
}"""
text = re.sub(r'func \(k \*KafkaProcessor\) Start\(\) \{.*?\}\n\}\n', new_start + "\n\n", text, flags=re.DOTALL)

# Replace Close
new_close = """func (k *KafkaProcessor) Close() {
	k.UpstreamReader.Close()
	k.DownstreamWriter.Close()
	close(k.quit)
}"""
text = re.sub(r'func \(k \*KafkaProcessor\) Close\(\) \{.*?\}\n\}', new_close, text, flags=re.DOTALL)

# Remove GetClient, RegisterClient, UnregisterClient
text = re.sub(r'func \(k \*KafkaProcessor\) GetClient.*?\}\n\}', '', text, flags=re.DOTALL)
text = re.sub(r'func \(k \*KafkaProcessor\) RegisterClient.*?\}\n\}', '', text, flags=re.DOTALL)
text = re.sub(r'func \(k \*KafkaProcessor\) UnregisterClient.*?\}\n\}', '', text, flags=re.DOTALL)

# Rewrite trySendBack
new_trySendBack = """func trySendBack(k *KafkaProcessor, targetUserId string, messageBack *MessageBack) {
	pe := event.PushEvent{
		TargetUserId: targetUserId,
		Payload:      messageBack.Message,
		MessageUuid:  messageBack.Uuid,
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
}"""
text = re.sub(r'func trySendBack\(client \*UserConn, msg \*MessageBack\) \{.*?\n\}', new_trySendBack, text, flags=re.DOTALL)

# Replace trySendBack usages
text = text.replace('if value, ok := k.Clients.Load(receiveId); ok {\n\t\tclient := value.(*UserConn)\n\t\ttrySendBack(client, &MessageBack{Message: jsonMsg, Uuid: ""})\n\t}', 'trySendBack(k, receiveId, &MessageBack{Message: jsonMsg, Uuid: ""})')
text = text.replace('if value, ok := k.Clients.Load(sendId); ok {\n\t\tclient := value.(*UserConn)\n\t\ttrySendBack(client, &MessageBack{Message: jsonMsg, Uuid: ""})\n\t}', 'trySendBack(k, sendId, &MessageBack{Message: jsonMsg, Uuid: ""})')
text = text.replace('if value, ok := k.Clients.Load(message.ReceiveId); ok {\n\t\treceiveClient := value.(*UserConn)\n\t\ttrySendBack(receiveClient, messageBack)\n\t}', 'trySendBack(k, message.ReceiveId, messageBack)')
text = text.replace('if value, ok := k.Clients.Load(message.SendId); ok {\n\t\tsendClient := value.(*UserConn)\n\t\ttrySendBack(sendClient, messageBack)\n\t}', 'trySendBack(k, message.SendId, messageBack)')
text = text.replace('if value, ok := k.Clients.Load(gm.UserUuid); ok {\n\t\t\t\treceiveClient := value.(*UserConn)\n\t\t\t\ttrySendBack(receiveClient, messageBack)\n\t\t\t}', 'trySendBack(k, gm.UserUuid, messageBack)')
text = text.replace('if value, ok := k.Clients.Load(message.SendId); ok {\n\t\t\tsendClient := value.(*UserConn)\n\t\t\ttrySendBack(sendClient, messageBack)\n\t\t}', 'trySendBack(k, message.SendId, messageBack)')

text = text.replace('client *UserConn', 'client interface{}')
text = text.replace('[]*UserConn', '[]interface{}')
text = text.replace('"github.com/gorilla/websocket"\n', '')

with open("internal/service/message/kafka_processor.go", "w") as f:
    f.write(text)

