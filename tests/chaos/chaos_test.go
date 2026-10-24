package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	mysqldriver "gorm.io/driver/mysql"
	"gorm.io/gorm"

	userpb "kama_chat_server/api/gen/user"
	applyapp "kama_chat_server/internal/apply"
	friendshipapp "kama_chat_server/internal/friendship"
	groupapp "kama_chat_server/internal/group"
	messageapp "kama_chat_server/internal/message"
	userapp "kama_chat_server/internal/user"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/internal/infra/outbox"
	myredis "kama_chat_server/internal/infra/redis"
	"kama_chat_server/internal/infra/snowflake"
	"kama_chat_server/pkg/enum/apply/apply_status"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// ChaosTestContext 聚合弱网混沌测试所需的全局中间件连接与业务服务句柄
type ChaosTestContext struct {
	DB              *gorm.DB
	RDB             *redis.Client
	UserStore       userapp.Store
	FriendshipStore friendshipapp.Store
	ApplyStore      applyapp.Store
	GroupStore      groupapp.GroupStore
	MessageStore    messageapp.MessageStore
	SessionStore    messageapp.SessionStore
	CachePort       *myredis.RedisCache
	UserService     *userapp.UserService
	FriendshipSvc   *friendshipapp.FriendshipService
	ApplyService    *applyapp.ApplyService
	GroupService    *groupapp.GroupService
	MessageSvc      *messageapp.MessageService
	SessionSvc      *messageapp.SessionService
}

// dummyUserServiceClient 桩实现，用于绕过跨服务远程调用以纯净压测业务写链路
type dummyUserServiceClient struct{}

func (d *dummyUserServiceClient) UpdateUserInfo(ctx context.Context, in *userpb.UpdateUserInfoRequest, opts ...grpc.CallOption) (*userpb.UpdateUserInfoResponse, error) {
	return &userpb.UpdateUserInfoResponse{}, nil
}
func (d *dummyUserServiceClient) GetUserInfo(ctx context.Context, in *userpb.GetUserInfoRequest, opts ...grpc.CallOption) (*userpb.GetUserInfoResponse, error) {
	return &userpb.GetUserInfoResponse{}, nil
}
func (d *dummyUserServiceClient) GetPublicUserInfo(ctx context.Context, in *userpb.GetPublicUserInfoRequest, opts ...grpc.CallOption) (*userpb.GetPublicUserInfoResponse, error) {
	return &userpb.GetPublicUserInfoResponse{
		Uuid:     in.TargetId,
		Nickname: "User_" + in.TargetId,
		Avatar:   "http://example.com/avatar.png",
	}, nil
}
func (d *dummyUserServiceClient) BatchGetPublicUserInfo(ctx context.Context, in *userpb.BatchGetPublicUserInfoRequest, opts ...grpc.CallOption) (*userpb.BatchGetPublicUserInfoResponse, error) {
	list := make([]*userpb.PublicUserInfo, 0, len(in.UserIds))
	for _, uid := range in.UserIds {
		list = append(list, &userpb.PublicUserInfo{
			Uuid:     uid,
			Nickname: "User_" + uid,
			Avatar:   "http://example.com/avatar.png",
		})
	}
	return &userpb.BatchGetPublicUserInfoResponse{Users: list}, nil
}
func (d *dummyUserServiceClient) GetUserStatus(ctx context.Context, in *userpb.GetUserStatusRequest, opts ...grpc.CallOption) (*userpb.GetUserStatusResponse, error) {
	return &userpb.GetUserStatusResponse{
		Status: int32(user_status.NORMAL),
	}, nil
}
func (d *dummyUserServiceClient) KickUser(ctx context.Context, in *userpb.KickUserRequest, opts ...grpc.CallOption) (*userpb.KickUserResponse, error) {
	return &userpb.KickUserResponse{}, nil
}

// setupChaosEnv 初始化真实 Docker 中间件环境并建表迁移
func setupChaosEnv(t *testing.T) *ChaosTestContext {
	os.Setenv("MYSQL_HOST", "127.0.0.1")
	os.Setenv("MYSQL_PORT", "3306")
	os.Setenv("MYSQL_USER", "root")
	os.Setenv("MYSQL_PASSWORD", "root123456")
	os.Setenv("DB_NAME", "chat")
	os.Setenv("REDIS_HOST", "127.0.0.1")
	os.Setenv("REDIS_PORT", "6385")
	os.Setenv("REDIS_PASSWORD", "")

	_ = config.GetConfig()

	dsn := "root:root123456@tcp(127.0.0.1:3306)/chat?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysqldriver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接 MySQL 失败: %v", err)
	}

	err = db.AutoMigrate(
		&userapp.UserInfo{},
		&friendshipapp.Friendship{},
		&applyapp.Apply{},
		&groupapp.GroupInfo{},
		&groupapp.GroupMember{},
		&messageapp.Session{},
		&messageapp.Message{},
		&outbox.Outbox{},
	)
	if err != nil {
		t.Fatalf("自动迁移表结构失败: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6385",
		DB:   0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("连接 Redis 失败: %v", err)
	}

	userStore := userapp.NewStore(db)
	friendshipStore := friendshipapp.NewStore(db)
	applyStore := applyapp.NewStore(db)
	groupStore := groupapp.NewGroupStore(db)
	groupMemberStore := groupapp.NewGroupMemberStore(db)
	messageStore := messageapp.NewMessageStore(db)
	sessionStore := messageapp.NewSessionStore(db)

	cachePort := myredis.NewRedisCache(rdb, 15)
	guard := idempotent.NewRedisGuard(rdb)

	grpc_client.UserClient = &dummyUserServiceClient{}

	return &ChaosTestContext{
		DB:              db,
		RDB:             rdb,
		UserStore:       userStore,
		FriendshipStore: friendshipStore,
		ApplyStore:      applyStore,
		GroupStore:      groupStore,
		MessageStore:    messageStore,
		SessionStore:    sessionStore,
		CachePort:       cachePort,
		UserService:     userapp.NewUserService(db, userStore, cachePort, guard),
		FriendshipSvc:   friendshipapp.NewFriendshipService(db, friendshipStore, cachePort),
		ApplyService:    applyapp.NewApplyService(db, applyStore, cachePort, guard),
		GroupService:    groupapp.NewGroupService(db, groupStore, groupMemberStore, cachePort, guard),
		MessageSvc:      messageapp.NewMessageService(messageStore, sessionStore, cachePort, guard),
		SessionSvc:      messageapp.NewSessionService(sessionStore, messageStore, cachePort, guard),
	}
}

func injectChaosDelay(minMs, maxMs int) {
	delta := maxMs - minMs
	if delta <= 0 {
		time.Sleep(time.Duration(minMs) * time.Millisecond)
		return
	}
	sleepMs := minMs + rand.Intn(delta)
	time.Sleep(time.Duration(sleepMs) * time.Millisecond)
}

func testCase1_UserWrite_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("u_%d", time.Now().UnixNano()%100000)
	username := "chaos_user_" + tag
	clientToken := "token_user_reg_" + tag

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			injectChaosDelay(10, 80)
			req := userapp.RegisterRequest{
				Username:    username,
				Password:    "TestPass123!",
				ClientToken: clientToken,
			}
			_, err := tc.UserService.Register(ctx, req, "127.0.0.1")
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	var userCount int64
	tc.DB.Model(&userapp.UserInfo{}).Where("nickname = ?", username).Count(&userCount)
	if userCount != 1 {
		t.Fatalf("[Case 1 失败] 用户注册重复落库: 期望 1 条，实际 %d 条", userCount)
	}
	t.Logf("[Case 1 通过] 用户高并发重试注册成功拦截，库中严格唯一落库 (成功响应次数: %d, 落库数: %d)", successCount, userCount)

	user, err := tc.UserStore.FindByNickname(ctx, username)
	if err != nil {
		t.Fatalf("查找注册用户失败: %v", err)
	}

	newNick := "Updated_" + tag
	updateReq := userapp.UpdateUserInfoRequest{
		Nickname: &newNick,
	}
	err = tc.UserService.UpdateUserInfo(ctx, user.Uuid, updateReq)
	if err != nil {
		t.Fatalf("[Case 1 失败] 更新用户资料失败: %v", err)
	}
	t.Logf("[Case 1 通过] 用户写操作与资料更新在弱网并发下校验全部通过")
}

func testCase2_Friendship_ABA_Attack_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("fs_%d", time.Now().UnixNano()%100000)
	userA := "UA_" + tag
	userB := "UB_" + tag

	tc.DB.Where("user_id IN ? AND friend_id IN ?", []string{userA, userB}, []string{userA, userB}).Delete(&friendshipapp.Friendship{})
	fsAB := &friendshipapp.Friendship{UserId: userA, FriendId: userB, Status: friendship_status.NORMAL, Version: 1}
	fsBA := &friendshipapp.Friendship{UserId: userB, FriendId: userA, Status: friendship_status.NORMAL, Version: 1}
	tc.DB.Create(fsAB)
	tc.DB.Create(fsBA)

	var wg sync.WaitGroup
	var latePacketErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(400 * time.Millisecond)
		latePacketErr = tc.FriendshipSvc.BlackFriend(ctx, userA, userB, 1)
	}()

	time.Sleep(50 * time.Millisecond)
	err1 := tc.FriendshipSvc.BlackFriend(ctx, userA, userB, 1)
	if err1 != nil {
		t.Fatalf("[Case 2] 正常拉黑失败: %v", err1)
	}

	time.Sleep(50 * time.Millisecond)
	err2 := tc.FriendshipSvc.UnblackFriend(ctx, userA, userB, 2)
	if err2 != nil {
		t.Fatalf("[Case 2] 正常解黑失败: %v", err2)
	}

	var outboxCountBeforeLate int64
	tc.DB.Model(&outbox.Outbox{}).Where("payload LIKE ?", "%"+userA+"%").Count(&outboxCountBeforeLate)

	wg.Wait()

	if latePacketErr == nil {
		t.Fatalf("[Case 2 致命漏洞] 迟到的旧拉黑请求竟然成功执行了！说明发生了 ABA 状态倒退与错误覆盖！")
	}
	t.Logf("[Case 2 拦截成功] 迟到拉黑请求被 Version CAS 乐观锁精准拦截，报错信息: %v", latePacketErr)

	latestAB, _ := tc.FriendshipStore.FindByUserIdAndFriendId(ctx, userA, userB)
	if latestAB.Status != friendship_status.NORMAL || latestAB.Version != 3 {
		t.Fatalf("[Case 2 校验失败] 好友关系被破坏！当前 Status=%d, Version=%d (期望 Status=0, Version=3)", latestAB.Status, latestAB.Version)
	}

	var outboxCountAfterLate int64
	tc.DB.Model(&outbox.Outbox{}).Where("payload LIKE ?", "%"+userA+"%").Count(&outboxCountAfterLate)
	if outboxCountAfterLate != outboxCountBeforeLate {
		t.Fatalf("[Case 2 校验失败] 迟到包产生了多余的 Outbox 领域事件！事件数从 %d 变为 %d", outboxCountBeforeLate, outboxCountAfterLate)
	}

	t.Logf("[Case 2 全部通过] 循环状态机 ABA 弱网混沌防御 100%% 生效，最终状态与版本号严格一致！")
}

func testCase3_Apply_Lifecycle_OldPacket_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("app_%d", time.Now().UnixNano()%100000)
	applicantId := "AppA_" + tag
	targetId := "AppB_" + tag

	err := tc.ApplyService.ApplyFriend(ctx, applicantId, applyapp.ApplyFriendRequest{
		FriendId: targetId,
		Message:  "First Apply",
	})
	if err != nil {
		t.Fatalf("[Case 3] 首次申请失败: %v", err)
	}

	firstApply, err := tc.ApplyStore.FindByApplicantIdAndTargetId(ctx, applicantId, targetId)
	if err != nil || firstApply.Version != 1 {
		t.Fatalf("[Case 3] 首次申请版本号异常: %v", err)
	}
	oldUuid := firstApply.Uuid
	oldVersion := firstApply.Version

	err = tc.ApplyService.RefuseFriendApply(ctx, targetId, applicantId)
	if err != nil {
		t.Fatalf("[Case 3] 拒绝申请失败: %v", err)
	}

	err = tc.ApplyService.ApplyFriend(ctx, applicantId, applyapp.ApplyFriendRequest{
		FriendId: targetId,
		Message:  "Second Apply",
	})
	if err != nil {
		t.Fatalf("[Case 3] 二次申请失败: %v", err)
	}

	secondApply, _ := tc.ApplyStore.FindByApplicantIdAndTargetId(ctx, applicantId, targetId)
	if secondApply.Uuid == oldUuid {
		t.Fatalf("[Case 3 缺陷] 二次申请未刷新全新 Uuid")
	}
	if secondApply.Version <= oldVersion {
		t.Fatalf("[Case 3 缺陷] 二次申请未递增 Version: old=%d, new=%d", oldVersion, secondApply.Version)
	}
	t.Logf("[Case 3] 二次申请成功刷新代际: Old(Uuid=%s, Ver=%d) -> New(Uuid=%s, Ver=%d)", oldUuid, oldVersion, secondApply.Uuid, secondApply.Version)

	ok, err := tc.ApplyStore.UpdateStatusIfPending(ctx, oldUuid, apply_status.REFUSE, oldVersion)
	if err != nil {
		t.Fatalf("[Case 3] 旧包执行报错: %v", err)
	}
	if ok {
		t.Fatalf("[Case 3 致命漏洞] 迟到的上一代审批包成功更新了记录！发生了跨代审批穿透！")
	}
	t.Logf("[Case 3 拦截成功] 迟到上一代审批包被 Version & Uuid CAS 坚决拒绝 (RowsAffected=0)")

	err = tc.ApplyService.PassFriendApply(ctx, targetId, applicantId)
	if err != nil {
		t.Fatalf("[Case 3] 当期合法审批失败: %v", err)
	}

	finalApply, _ := tc.ApplyStore.FindByApplicantIdAndTargetId(ctx, applicantId, targetId)
	if finalApply.Status != apply_status.AGREE {
		t.Fatalf("[Case 3 失败] 最终申请状态不是 AGREE: %d", finalApply.Status)
	}
	t.Logf("[Case 3 全部通过] 申请生命周期防跨代穿透验证全部通过！")
}

func testCase4_Group_CollaborativeEdit_VersionConflict_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("grp_%d", time.Now().UnixNano()%100000)
	ownerId := "Owner_" + tag
	adminId := "Admin_" + tag

	groupReq := groupapp.CreateGroupRequest{
		Name:   "G_" + tag,
		Notice: "Initial Notice",
	}
	groupId, err := tc.GroupService.CreateGroup(ctx, ownerId, groupReq)
	if err != nil {
		t.Fatalf("[Case 4] 建群失败: %v", err)
	}

	tc.DB.Create(&groupapp.GroupMember{
		GroupUuid: groupId,
		UserUuid:  adminId,
		Role:      2,
	})

	grp, _ := tc.GroupStore.FindByUuid(ctx, groupId)
	baseVersion := grp.Version

	var wg sync.WaitGroup
	var errOwner, errAdmin error

	notice1 := "Notice Modified by Owner"
	notice2 := "Notice Modified by Admin"

	wg.Add(2)
	go func() {
		defer wg.Done()
		injectChaosDelay(20, 60)
		errOwner = tc.GroupService.UpdateGroupInfo(ctx, ownerId, groupapp.UpdateGroupInfoRequest{
			Uuid:    groupId,
			Notice:  &notice1,
			Version: &baseVersion,
		})
	}()

	go func() {
		defer wg.Done()
		injectChaosDelay(20, 60)
		errAdmin = tc.GroupService.UpdateGroupInfo(ctx, adminId, groupapp.UpdateGroupInfoRequest{
			Uuid:    groupId,
			Notice:  &notice2,
			Version: &baseVersion,
		})
	}()
	wg.Wait()

	hasConflict := errorx.IsCode(errOwner, errorx.CodeConflict) || errorx.IsCode(errAdmin, errorx.CodeConflict)
	hasSuccess := (errOwner == nil) || (errAdmin == nil)

	if !hasConflict || !hasSuccess {
		t.Fatalf("[Case 4 冲突校验失败] 期望一人成功一人409冲突，实际 errOwner=%v, errAdmin=%v", errOwner, errAdmin)
	}
	t.Logf("[Case 4 通过] 群资料并发协同编辑成功拦截版本冲突: 成功者顺利递增，落后者收到 409 Conflict")

	updatedGrp, _ := tc.GroupStore.FindByUuid(ctx, groupId)
	if updatedGrp.Version != baseVersion+1 {
		t.Fatalf("[Case 4 版本异常] 群版本号期望 %d，实际 %d", baseVersion+1, updatedGrp.Version)
	}
	t.Logf("[Case 4 全部通过] 群资料协同编辑乐观锁防覆盖 100%% 有效！")
}

func testCase5_Message_RetryStorm_And_RevokeCAS_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("msg_%d", time.Now().UnixNano()%100000)
	sendId := "UserS_" + tag
	recvId := "UserR_" + tag
	clientMsgId := "CMID_" + tag
	sessionId := "P_" + sendId + "_" + recvId

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			injectChaosDelay(10, 50)
			msg := messageapp.Message{
				Uuid:        "M" + snowflake.GenerateIDString(),
				ClientMsgId: clientMsgId,
				SessionId:   sessionId,
				SendId:      sendId,
				ReceiveId:   recvId,
				Type:        int8(message_type.Text),
				Content:     "Hello Chaos Testing",
			}
			_ = tc.MessageStore.Create(ctx, &msg)
		}(i)
	}
	wg.Wait()

	var msgCount int64
	tc.DB.Model(&messageapp.Message{}).Where("client_msg_id = ?", clientMsgId).Count(&msgCount)
	if msgCount != 1 {
		t.Fatalf("[Case 5 消息去重失败] 重试风暴导致重复落库: 期望 1 条，实际 %d 条", msgCount)
	}
	t.Logf("[Case 5 通过] 消息发送重试风暴成功去重，库中严格唯一落库 (落库数: %d)", msgCount)

	var storedMsg messageapp.Message
	tc.DB.Where("client_msg_id = ?", clientMsgId).First(&storedMsg)

	var revokeSuccessCount int
	var mu sync.Mutex
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			injectChaosDelay(5, 30)
			ok, err := tc.MessageStore.UpdateContent(ctx, storedMsg.Uuid, "", int8(message_type.Recall))
			if err == nil && ok {
				mu.Lock()
				revokeSuccessCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if revokeSuccessCount != 1 {
		t.Fatalf("[Case 5 撤回状态机失败] 单向撤回状态机成功次数期望为 1，实际为 %d", revokeSuccessCount)
	}
	t.Logf("[Case 5 通过] 消息撤回单向状态机 CAS 成功防护并发重试，仅首次生效 (生效次数: %d)", revokeSuccessCount)
}

func testCase6_Session_Pin_AtomicUpdate_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("sess_%d", time.Now().UnixNano()%100000)
	sessionUuid := "SESS_" + tag
	userId := "UserPin_" + tag

	sess := &messageapp.Session{
		Uuid:      sessionUuid,
		SendId:    userId,
		ReceiveId: "Target_" + tag,
		IsPinned:  false,
	}
	tc.DB.Create(sess)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			injectChaosDelay(10, 40)
			_ = tc.SessionSvc.PinSession(ctx, userId, sessionUuid, true)
		}(i)
	}
	wg.Wait()

	var curSess messageapp.Session
	tc.DB.Where("uuid = ?", sessionUuid).First(&curSess)
	if !curSess.IsPinned {
		t.Fatalf("[Case 6 失败] 会话置顶状态异常: 期望 true，实际 false")
	}
	t.Logf("[Case 6 通过] 会话置顶原子操作并发测试全部通过！")
}

func TestAllWriteInterfaces_WeakNetworkChaos(t *testing.T) {
	tc := setupChaosEnv(t)
	t.Log("================================================================================")
	t.Log("弱网混沌测试套件启动: 已连通真实 Docker 中间件环境 (MySQL 3306, Redis 6385)")
	t.Log("================================================================================")

	t.Log(">>> [Phase 1] 执行用户模块弱网并发写操作测试...")
	testCase1_UserWrite_Chaos(t, tc)

	t.Log(">>> [Phase 2] 执行好友拉黑/解黑 ABA 循环状态机弱网攻击测试...")
	testCase2_Friendship_ABA_Attack_Chaos(t, tc)

	t.Log(">>> [Phase 3] 执行联系人申请重置与跨代审批穿透测试...")
	testCase3_Apply_Lifecycle_OldPacket_Chaos(t, tc)

	t.Log(">>> [Phase 4] 执行群资料多人协同编辑版本冲突保护测试...")
	testCase4_Group_CollaborativeEdit_VersionConflict_Chaos(t, tc)

	t.Log(">>> [Phase 5] 执行消息重试风暴去重与撤回单向状态机测试...")
	testCase5_Message_RetryStorm_And_RevokeCAS_Chaos(t, tc)

	t.Log(">>> [Phase 6] 执行会话置顶原子写操作测试...")
	testCase6_Session_Pin_AtomicUpdate_Chaos(t, tc)

	t.Log("================================================================================")
	t.Log("[弱网混沌测试大获全胜] 所有 6 大写操作核心链路在延迟、乱序、ABA 攻击下 100% 成功！")
	t.Log("================================================================================")
}
