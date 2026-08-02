package grpc_client

import (
	"log"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"

	authpb "kama_chat_server/api/gen/auth"
	messagepb "kama_chat_server/api/gen/message"
	relationpb "kama_chat_server/api/gen/relation"
	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/pkg/discovery"
	"kama_chat_server/pkg/interceptor"
)

var (
	UserClient     userpb.UserServiceClient
	AuthClient     authpb.AuthServiceClient
	RelationClient relationpb.RelationServiceClient
	MessageClient  messagepb.MessageServiceClient
	once           sync.Once
)

func Init(etcdEndpoints []string) {
	once.Do(func() {
		// 注册 Etcd Resolver
		r := discovery.NewResolver(etcdEndpoints)
		resolver.Register(r)

		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(interceptor.ClientAuthInterceptor()),
			grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		}

		// User Service
		userConn, err := grpc.Dial(discovery.BuildDialTarget("user_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect user_service: %v", err)
		}
		UserClient = userpb.NewUserServiceClient(userConn)

		// Auth Service
		authConn, err := grpc.Dial(discovery.BuildDialTarget("auth_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect auth_service: %v", err)
		}
		AuthClient = authpb.NewAuthServiceClient(authConn)

		// Relation Service
		relationConn, err := grpc.Dial(discovery.BuildDialTarget("relation_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect relation_service: %v", err)
		}
		RelationClient = relationpb.NewRelationServiceClient(relationConn)

		// Message Service
		messageConn, err := grpc.Dial(discovery.BuildDialTarget("message_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect message_service: %v", err)
		}
		MessageClient = messagepb.NewMessageServiceClient(messageConn)
	})
}
