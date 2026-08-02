package chat

// ChatServer 聊天服务器聚合结构
type ChatServer struct {
	Broker      *MsgConsumer
	KafkaClient *KafkaClient
}

// NewChatServer 创建聊天服务器实例
func NewChatServer() *ChatServer {
	cs := &ChatServer{}
	cs.KafkaClient = NewKafkaClient()
	cs.Broker = NewMsgConsumer(cs.KafkaClient)
	return cs
}

func (cs *ChatServer) InitKafka() {
	cs.KafkaClient.KafkaInit()
}

func (cs *ChatServer) Run() {
	cs.Broker.Start()
}

func (cs *ChatServer) Shutdown() {
	cs.Broker.Close()
	if cs.KafkaClient != nil {
		cs.KafkaClient.KafkaClose()
	}
}

func (cs *ChatServer) GetBroker() *MsgConsumer {
	return cs.Broker
}
