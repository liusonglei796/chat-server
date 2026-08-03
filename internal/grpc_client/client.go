package grpc_client

import (
	"context"
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
	"kama_chat_server/pkg/errorx"
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

// GetUserStatus 跨服务获取用户账号状态（0=正常 1=禁用），用户不存在时返回 codes.NotFound
func GetUserStatus(ctx context.Context, userId string) (int8, error) {
	if UserClient == nil {
		return 0, errorx.New(errorx.CodeServerBusy, "user grpc client not initialized")
	}
	rsp, err := UserClient.GetUserStatus(ctx, &userpb.GetUserStatusRequest{UserId: userId})
	if err != nil {
		return 0, err
	}
	return int8(rsp.Status), nil
}

// GetUserNicknameAvatar 跨服务取用户昵称头像（GetUserInfo 仅允许本人查询，故 requester=target=本人）
func GetUserNicknameAvatar(ctx context.Context, targetId string) (nickname, avatar string, err error) {
	if UserClient == nil {
		return "", "", errorx.New(errorx.CodeServerBusy, "user grpc client not initialized")
	}
	rsp, err := UserClient.GetUserInfo(ctx, &userpb.GetUserInfoRequest{RequesterId: targetId, TargetId: targetId})
	if err != nil {
		return "", "", err
	}
	return rsp.Nickname, rsp.Avatar, nil
}

// CheckFriendshipStatus 跨服务查好友关系状态（0=非好友 1=正常 2=已拉黑对方 3=被对方拉黑）
func CheckFriendshipStatus(ctx context.Context, userId, friendId string) (int8, error) {
	if RelationClient == nil {
		return 0, errorx.New(errorx.CodeServerBusy, "relation grpc client not initialized")
	}
	rsp, err := RelationClient.CheckFriendship(ctx, &relationpb.CheckFriendshipRequest{UserId: userId, FriendId: friendId})
	if err != nil {
		return 0, err
	}
	return int8(rsp.Status), nil
}

// CheckGroupMember 跨服务查用户是否为群成员
func CheckGroupMember(ctx context.Context, groupId, userId string) (bool, error) {
	if RelationClient == nil {
		return false, errorx.New(errorx.CodeServerBusy, "relation grpc client not initialized")
	}
	rsp, err := RelationClient.CheckGroupMember(ctx, &relationpb.CheckGroupMemberRequest{GroupId: groupId, UserId: userId})
	if err != nil {
		return false, err
	}
	return rsp.IsMember, nil
}

// GetGroupDetail 跨服务获取群详情（已含成员校验/禁用校验/存在性校验）
func GetGroupDetail(ctx context.Context, userId, groupId string) (*relationpb.GetGroupDetailResponse, error) {
	if RelationClient == nil {
		return nil, errorx.New(errorx.CodeServerBusy, "relation grpc client not initialized")
	}
	rsp, err := RelationClient.GetGroupDetail(ctx, &relationpb.GetGroupDetailRequest{UserId: userId, GroupId: groupId})
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

// ListGroupMemberIds 跨服务获取群全部成员ID（不要求调用方是成员）
func ListGroupMemberIds(ctx context.Context, groupId string) ([]string, error) {
	if RelationClient == nil {
		return nil, errorx.New(errorx.CodeServerBusy, "relation grpc client not initialized")
	}
	rsp, err := RelationClient.ListGroupMemberIds(ctx, &relationpb.ListGroupMemberIdsRequest{GroupId: groupId})
	if err != nil {
		return nil, err
	}
	return rsp.UserIds, nil
}
