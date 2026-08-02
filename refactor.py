import re

with open("internal/service/message/kafka_processor.go", "r") as f:
    content = f.read()

# Change package name
content = content.replace("package chat", "package message")

# Fix imports
content = content.replace('"kama_chat_server/internal/model"', '"kama_chat_server/internal/model"\n\t"kama_chat_server/internal/dto/event"\n\t"kama_chat_server/internal/config"\n')

# Rename MsgConsumer to KafkaProcessor
content = content.replace("MsgConsumer", "KafkaProcessor")

# Remove WS dependencies
content = re.sub(r'Clients\s+sync\.Map\n', '', content)
content = re.sub(r'Login\s+chan\s+\*UserConn\n', '', content)
content = re.sub(r'Logout\s+chan\s+\*UserConn\n', '', content)

content = content.replace("kafkaClient     *KafkaClient", "UpstreamReader *kafka.Reader\n\tDownstreamWriter *kafka.Writer")

# Rewrite NewKafkaProcessor
new_constructor = """
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
		UpstreamReader:  reader,
		DownstreamWriter: writer,
		MessageRepo:     messageRepo,
		friendshipRepo:  friendshipRepo,
		groupMemberRepo: groupMemberRepo,
		sessionRepo:     sessionRepo,
		cacheService:    cacheService,
		cacheHelper:     helper,
		userRepo:        userRepo,
		quit:            make(chan os.Signal, 1),
	}
}
"""
content = re.sub(r'func NewKafkaProcessor.*?return &KafkaProcessor\{.*?\}', new_constructor, content, flags=re.DOTALL)

# Delete Publish
content = re.sub(r'// Publish.*?func \(k \*KafkaProcessor\) Publish.*?return k\.kafkaClient\.Producer\.WriteMessages\(ctx, kafka\.Message\{.*?\}\}', '', content, flags=re.DOTALL)

# Rewrite Start
new_start = """
func (k *KafkaProcessor) Start() {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error(fmt.Sprintf("kafka server panic: %v", r))
		}
		k.closeOnce.Do(func() {})
	}()
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
}
"""
content = re.sub(r'func \(k \*KafkaProcessor\) Start\(\) \{.*?\}\s*\}', new_start, content, flags=re.DOTALL)

# Delete Close (and recreate it correctly)
content = re.sub(r'func \(k \*KafkaProcessor\) Close\(\).*?\}', 'func (k *KafkaProcessor) Close() {\n\tk.UpstreamReader.Close()\n\tk.DownstreamWriter.Close()\n}', content, flags=re.DOTALL)

# Delete GetClient, RegisterClient, UnregisterClient
content = re.sub(r'func \(k \*KafkaProcessor\) GetClient.*?\}', '', content, flags=re.DOTALL)
content = re.sub(r'func \(k \*KafkaProcessor\) RegisterClient.*?\}', '', content, flags=re.DOTALL)
content = re.sub(r'func \(k \*KafkaProcessor\) UnregisterClient.*?\}', '', content, flags=re.DOTALL)

# Rewrite trySendBack to pushToDownstream
new_trySendBack = """
func trySendBack(k *KafkaProcessor, targetUserId string, messageBack *MessageBack) {
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
}
"""
content = re.sub(r'func trySendBack\(client \*UserConn, msg \*MessageBack\) \{.*?\}', new_trySendBack, content, flags=re.DOTALL)

# Replace all trySendBack calls
content = content.replace('if value, ok := k.Clients.Load(receiveId); ok {\n\t\tclient := value.(*UserConn)\n\t\ttrySendBack(client, &MessageBack{Message: jsonMsg, Uuid: ""})\n\t}', 'trySendBack(k, receiveId, &MessageBack{Message: jsonMsg, Uuid: ""})')
content = content.replace('if value, ok := k.Clients.Load(sendId); ok {\n\t\tclient := value.(*UserConn)\n\t\ttrySendBack(client, &MessageBack{Message: jsonMsg, Uuid: ""})\n\t}', 'trySendBack(k, sendId, &MessageBack{Message: jsonMsg, Uuid: ""})')

content = content.replace('if value, ok := k.Clients.Load(message.ReceiveId); ok {\n\t\treceiveClient := value.(*UserConn)\n\t\ttrySendBack(receiveClient, messageBack)\n\t}', 'trySendBack(k, message.ReceiveId, messageBack)')
content = content.replace('if value, ok := k.Clients.Load(message.SendId); ok {\n\t\tsendClient := value.(*UserConn)\n\t\ttrySendBack(sendClient, messageBack)\n\t}', 'trySendBack(k, message.SendId, messageBack)')

content = content.replace('if value, ok := k.Clients.Load(gm.UserUuid); ok {\n\t\t\t\treceiveClient := value.(*UserConn)\n\t\t\t\ttrySendBack(receiveClient, messageBack)\n\t\t\t}', 'trySendBack(k, gm.UserUuid, messageBack)')


with open("internal/service/message/kafka_processor.go", "w") as f:
    f.write(content)

