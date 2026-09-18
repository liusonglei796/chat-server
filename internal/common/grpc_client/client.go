package grpc_client

import (
	"context"
	"fmt"
	"log"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"

	applypb "kama_chat_server/api/gen/apply"
	authpb "kama_chat_server/api/gen/auth"
	friendshippb "kama_chat_server/api/gen/friendship"
	gatewaypb "kama_chat_server/api/gen/gateway"
	grouppb "kama_chat_server/api/gen/group"
	messagepb "kama_chat_server/api/gen/message"
	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/pkg/discovery"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
	otelinit "kama_chat_server/pkg/otel"
)

var (
	UserClient     userpb.UserServiceClient
	AuthClient     authpb.AuthServiceClient
	ApplyClient applypb.ApplyServiceClient
	FriendshipClient friendshippb.FriendshipServiceClient
	GroupClient grouppb.GroupServiceClient
	MessageClient  messagepb.MessageServiceClient
	once           sync.Once
)

func Init(etcdEndpoints []string) {
	once.Do(func() {
		// 注册 Etcd Resolver
		r, err := discovery.NewResolver(etcdEndpoints)
		if err != nil {
			log.Fatalf("failed to init etcd resolver: %v", err)
		}
		resolver.Register(r)

		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithResolvers(r),
			// 先注入用户身份，再注入 trace context（顺序：auth → trace）
			grpc.WithChainUnaryInterceptor(
				interceptor.ClientAuthInterceptor(),
				otelinit.ClientTraceInterceptor(),
			),
			grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		}

		// User Service
		// 使用 NewClient（懒加载）而非 Dial：Dial 会同步 exitIdleMode 触发 resolver
		// 建连，冷启动时对端服务尚未注册到 etcd，导致 "bad resolver state" 直接 Fatal。
		userConn, err := grpc.NewClient(discovery.BuildDialTarget("user_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect user_service: %v", err)
		}
		UserClient = userpb.NewUserServiceClient(userConn)

		// Auth Service (hosted by user_service)
		authConn, err := grpc.NewClient(discovery.BuildDialTarget("user_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect auth_service: %v", err)
		}
		AuthClient = authpb.NewAuthServiceClient(authConn)

		// Apply Service
		applyConn, err := grpc.NewClient(discovery.BuildDialTarget("apply_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect apply_service: %v", err)
		}
		ApplyClient = applypb.NewApplyServiceClient(applyConn)

		// Friendship Service
		friendshipConn, err := grpc.NewClient(discovery.BuildDialTarget("friendship_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect friendship_service: %v", err)
		}
		FriendshipClient = friendshippb.NewFriendshipServiceClient(friendshipConn)

		// Group Service
		groupConn, err := grpc.NewClient(discovery.BuildDialTarget("group_service"), opts...)
		if err != nil {
			log.Fatalf("failed to connect group_service: %v", err)
		}
		GroupClient = grouppb.NewGroupServiceClient(groupConn)

		// Message Service
		messageConn, err := grpc.NewClient(discovery.BuildDialTarget("message_service"), opts...)
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
	if FriendshipClient == nil {
		return 0, errorx.New(errorx.CodeServerBusy, "relation grpc client not initialized")
	}
	rsp, err := FriendshipClient.CheckFriendship(ctx, &friendshippb.CheckFriendshipRequest{UserId: userId, FriendId: friendId})
	if err != nil {
		return 0, err
	}
	return int8(rsp.Status), nil
}

// CheckGroupMember 跨服务查用户是否为群成员
func CheckGroupMember(ctx context.Context, groupId, userId string) (bool, error) {
	if GroupClient == nil {
		return false, errorx.New(errorx.CodeServerBusy, "grpc client not initialized")
	}
	rsp, err := GroupClient.CheckGroupMember(ctx, &grouppb.CheckGroupMemberRequest{GroupId: groupId, UserId: userId})
	if err != nil {
		return false, err
	}
	return rsp.IsMember, nil
}

// GetGroupMemberRole 跨服务获取用户在某群的角色（1普通成员 2管理员 3群主）
func GetGroupMemberRole(ctx context.Context, groupId, userId string) (int8, error) {
	if GroupClient == nil {
		return 0, errorx.New(errorx.CodeServerBusy, "grpc client not initialized")
	}
	rsp, err := GroupClient.GetGroupMemberRole(ctx, &grouppb.GetGroupMemberRoleRequest{GroupId: groupId, UserId: userId})
	if err != nil {
		return 0, err
	}
	return int8(rsp.Role), nil
}

// GetGroupDetail 跨服务获取群详情（已含成员校验/禁用校验/存在性校验）
func GetGroupDetail(ctx context.Context, userId, groupId string) (*grouppb.GetGroupDetailResponse, error) {
	if GroupClient == nil {
		return nil, errorx.New(errorx.CodeServerBusy, "grpc client not initialized")
	}
	rsp, err := GroupClient.GetGroupDetail(ctx, &grouppb.GetGroupDetailRequest{UserId: userId, GroupId: groupId})
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

// ListGroupMemberIds 跨服务获取群全部成员ID（不要求调用方是成员）
func ListGroupMemberIds(ctx context.Context, groupId string) ([]string, error) {
	if GroupClient == nil {
		return nil, errorx.New(errorx.CodeServerBusy, "grpc client not initialized")
	}
	rsp, err := GroupClient.ListGroupMemberIds(ctx, &grouppb.ListGroupMemberIdsRequest{GroupId: groupId})
	if err != nil {
		return nil, err
	}
	return rsp.UserIds, nil
}

// BatchGetPublicUserInfo 跨服务批量获取用户公开信息（昵称/头像/性别/生日/签名）
func BatchGetPublicUserInfo(ctx context.Context, userIds []string) ([]*userpb.PublicUserInfo, error) {
	if UserClient == nil {
		return nil, errorx.New(errorx.CodeServerBusy, "user grpc client not initialized")
	}
	if len(userIds) == 0 {
		return []*userpb.PublicUserInfo{}, nil
	}
	rsp, err := UserClient.BatchGetPublicUserInfo(ctx, &userpb.BatchGetPublicUserInfoRequest{UserIds: userIds})
	if err != nil {
		return nil, err
	}
	return rsp.Users, nil
}

// GetPublicUserInfo 跨服务获取单个用户公开信息（昵称/头像/性别/生日/签名）
func GetPublicUserInfo(ctx context.Context, userId string) (*userpb.PublicUserInfo, error) {
	if UserClient == nil {
		return nil, errorx.New(errorx.CodeServerBusy, "user grpc client not initialized")
	}
	rsp, err := UserClient.GetPublicUserInfo(ctx, &userpb.GetPublicUserInfoRequest{TargetId: userId})
	if err != nil {
		return nil, err
	}
	return &userpb.PublicUserInfo{
		Uuid:      rsp.Uuid,
		Nickname:  rsp.Nickname,
		Avatar:    rsp.Avatar,
		Gender:    rsp.Gender,
		Birthday:  rsp.Birthday,
		Signature: rsp.Signature,
	}, nil
}

// SendMessage 跨服务发送消息（上行同步落库 + 下行单管道广播）
func SendMessage(ctx context.Context, req *messagepb.SendMessageRequest) (*messagepb.SendMessageResponse, error) {
	if MessageClient == nil {
		return nil, errorx.New(errorx.CodeServerBusy, "message grpc client not initialized")
	}
	return MessageClient.SendMessage(ctx, req)
}

var (
	gatewayClients sync.Map // map[string]gatewaypb.GatewayServiceClient
	gatewayMu      sync.Mutex
)

// GetGatewayClient 获取指定网关地址的 gRPC 客户端（连接复用池）
func GetGatewayClient(addr string) (gatewaypb.GatewayServiceClient, error) {
	if val, ok := gatewayClients.Load(addr); ok {
		return val.(gatewaypb.GatewayServiceClient), nil
	}
	gatewayMu.Lock()
	defer gatewayMu.Unlock()
	if val, ok := gatewayClients.Load(addr); ok {
		return val.(gatewaypb.GatewayServiceClient), nil
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			interceptor.ClientAuthInterceptor(),
			otelinit.ClientTraceInterceptor(),
		),
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, err
	}
	client := gatewaypb.NewGatewayServiceClient(conn)
	gatewayClients.Store(addr, client)
	return client, nil
}

// PushToGateway 向指定网关实例直推消息
func PushToGateway(ctx context.Context, gatewayAddr string, targetUserId string, messageUuid string, payload []byte) error {
	client, err := GetGatewayClient(gatewayAddr)
	if err != nil {
		return err
	}
	rsp, err := client.PushMessage(ctx, &gatewaypb.PushMessageRequest{
		TargetUserId: targetUserId,
		MessageUuid:  messageUuid,
		Payload:      payload,
	})
	if err != nil {
		return err
	}
	if !rsp.Success {
		return fmt.Errorf("gateway push failed: %s", rsp.ErrorMsg)
	}
	return nil
}

// DeleteGroupMemberSessions 跨服务通知 message_service 批量软删指定群成员的会话
func DeleteGroupMemberSessions(ctx context.Context, groupId string, userIds []string) error {
	if MessageClient == nil {
		return errorx.New(errorx.CodeServerBusy, "message grpc client not initialized")
	}
	_, err := MessageClient.DeleteGroupMemberSessions(ctx, &messagepb.DeleteGroupMemberSessionsRequest{
		GroupId: groupId,
		UserIds: userIds,
	})
	return err
}

// AddGroupMember 跨服务通知 group_service 添加群成员
func AddGroupMember(ctx context.Context, groupId, userId string, role int8) error {
	if GroupClient == nil {
		return errorx.New(errorx.CodeServerBusy, "group grpc client not initialized")
	}
	_, err := GroupClient.AddGroupMember(ctx, &grouppb.AddGroupMemberRequest{
		GroupId: groupId,
		UserId:  userId,
		Role:    int32(role),
	})
	return err
}

// CreateGroupSession 跨服务通知 message_service 创建群会话
func CreateGroupSession(ctx context.Context, groupId, userId, groupName, groupAvatar string) error {
	if MessageClient == nil {
		return errorx.New(errorx.CodeServerBusy, "message grpc client not initialized")
	}
	_, err := MessageClient.CreateGroupSession(ctx, &messagepb.CreateGroupSessionRequest{
		GroupId:     groupId,
		UserId:      userId,
		GroupName:   groupName,
		GroupAvatar: groupAvatar,
	})
	return err
}

// DeleteFriendSessions 跨服务通知 message_service 软删双方私聊会话
func DeleteFriendSessions(ctx context.Context, userOneId, userTwoId string) error {
	if MessageClient == nil {
		return errorx.New(errorx.CodeServerBusy, "message grpc client not initialized")
	}
	_, err := MessageClient.DeleteFriendSessions(ctx, &messagepb.DeleteFriendSessionsRequest{
		UserOneId: userOneId,
		UserTwoId: userTwoId,
	})
	return err
}

// EstablishFriendship 跨服务通知 friendship_service 建立双向好友关系
func EstablishFriendship(ctx context.Context, userId, friendId string) error {
	if FriendshipClient == nil {
		return errorx.New(errorx.CodeServerBusy, "friendship grpc client not initialized")
	}
	_, err := FriendshipClient.EstablishFriendship(ctx, &friendshippb.EstablishFriendshipRequest{
		UserId:   userId,
		FriendId: friendId,
	})
	return err
}


