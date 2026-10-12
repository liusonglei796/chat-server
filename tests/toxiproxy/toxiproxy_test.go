package toxiproxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	mysqldriver "gorm.io/driver/mysql"
	"gorm.io/gorm"

	userpb "kama_chat_server/api/gen/user"
	applyapp "kama_chat_server/internal/apps/apply"
	friendshipapp "kama_chat_server/internal/apps/friendship"
	groupapp "kama_chat_server/internal/apps/group"
	messageapp "kama_chat_server/internal/apps/message/message"
	sessionapp "kama_chat_server/internal/apps/message/session"
	userapp "kama_chat_server/internal/apps/user/user"
	"kama_chat_server/internal/common/config"
	"kama_chat_server/internal/common/dao/mysql"
	myredis "kama_chat_server/internal/common/dao/redis"
	applyreq "kama_chat_server/internal/common/dto/request/apply"
	authreq "kama_chat_server/internal/common/dto/request/auth"
	groupreq "kama_chat_server/internal/common/dto/request/group"
	userreq "kama_chat_server/internal/common/dto/request/user"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

const (
	toxiproxyAPI   = "http://127.0.0.1:8474"
	proxyMySQLPort = "33066"
	proxyRedisPort = "63855"
	realMySQLPort  = "3306"
	realRedisPort  = "6385"
)

// dummyUserServiceClient 模拟用户服务 RPC 响应
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

// ToxiproxyTestContext 聚合受 Toxiproxy 代理托管的弱网测试上下文
type ToxiproxyTestContext struct {
	DB            *gorm.DB
	RDB           *redis.Client
	Stores        *mysql.Stores
	CachePort     *myredis.RedisCache
	UserService   *userapp.UserService
	FriendshipSvc *friendshipapp.FriendshipService
	ApplyService  *applyapp.ApplyService
	GroupService  *groupapp.GroupService
	MessageSvc    *messageapp.MessageService
	SessionSvc    *sessionapp.SessionService
}

// Test_AllWriteInterfaces_With_Toxiproxy 全量写接口在 Toxiproxy 真实弱网环境下的混沌压测总入口
func Test_AllWriteInterfaces_With_Toxiproxy(t *testing.T) {
	// Step 1.1: 搭建并配置 Toxiproxy 代理通道
	// 来源: Toxiproxy REST API 8474
	// 用于: Step 1.2 将流量注入 300ms 延迟与 50ms 抖动
	setupToxiproxyRoutes(t)

	// Step 1.2: 向 MySQL 与 Redis 代理信道注入高延迟与弱网抖动
	// 来源: setupToxiproxyRoutes 创建的 mysql_proxy 和 redis_proxy
	// 用于: Step 1.3 让全量写操作经过真实弱网通道
	injectToxics(t)

	// Step 1.3: 初始化经由 Toxiproxy 代理的数据库与业务服务连接
	// 来源: 代理端口 33066 与 63855
	// 用于: 执行后续 6 大微服务写接口测试
	tc := setupToxiproxyContext(t)

	// Step 2.1: 压测用户微服务写接口 (注册与资料更新)
	t.Run("1_User_Write_Interfaces", func(t *testing.T) {
		testUserWrites(t, tc)
	})

	// Step 2.2: 压测好友微服务写接口 (拉黑/解拉黑/更新备注)
	t.Run("2_Friendship_Write_Interfaces", func(t *testing.T) {
		testFriendshipWrites(t, tc)
	})

	// Step 2.3: 压测联系人申请微服务写接口 (发起申请/审批通过/终态自愈)
	t.Run("3_Apply_Write_Interfaces", func(t *testing.T) {
		testApplyWrites(t, tc)
	})

	// Step 2.4: 压测群组微服务写接口 (建群/协同改资料/退群)
	t.Run("4_Group_Write_Interfaces", func(t *testing.T) {
		testGroupWrites(t, tc)
	})

	// Step 2.5: 压测消息微服务写接口 (发消息唯一键落库/消息撤回单向 CAS)
	t.Run("5_Message_Write_Interfaces", func(t *testing.T) {
		testMessageWrites(t, tc)
	})

	// Step 2.6: 压测会话微服务写接口 (会话置顶/更新)
	t.Run("6_Session_Write_Interfaces", func(t *testing.T) {
		testSessionWrites(t, tc)
	})
}

// setupToxiproxyRoutes 通过 REST API 初始化 Toxiproxy 路由
func setupToxiproxyRoutes(t *testing.T) {
	createProxy(t, "mysql_proxy", "127.0.0.1:"+proxyMySQLPort, "127.0.0.1:"+realMySQLPort)
	createProxy(t, "redis_proxy", "127.0.0.1:"+proxyRedisPort, "127.0.0.1:"+realRedisPort)
}

// createProxy 向 Toxiproxy 注册单一 TCP 代理
func createProxy(t *testing.T, name, listen, upstream string) {
	delReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/proxies/%s", toxiproxyAPI, name), nil)
	delReq.Header.Set("User-Agent", "Toxiproxy-Client")
	_, _ = http.DefaultClient.Do(delReq)

	body := map[string]interface{}{
		"name":     name,
		"listen":   listen,
		"upstream": upstream,
		"enabled":  true,
	}
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, toxiproxyAPI+"/proxies", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Toxiproxy-Client")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("连接 Toxiproxy 失败: %v (请确保 toxiproxy-server 已启动并在 8474 端口监听)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("创建 Toxiproxy 代理 %s 失败 (code=%d): %s", name, resp.StatusCode, string(respBody))
	}
	t.Logf("[Toxiproxy 成功就绪] 代理 %s: 监听 %s -> 上游目标 %s", name, listen, upstream)
}

// injectToxics 注入下游延迟与弱网抖动
func injectToxics(t *testing.T) {
	// 给 MySQL 注入 300ms 延迟 + 50ms 抖动
	addToxic(t, "mysql_proxy", "mysql_latency", "latency", "downstream", map[string]interface{}{
		"latency": 300,
		"jitter":  50,
	})

	// 给 Redis 注入 50ms 延迟 + 10ms 抖动
	addToxic(t, "redis_proxy", "redis_latency", "latency", "downstream", map[string]interface{}{
		"latency": 50,
		"jitter":  10,
	})
}

// addToxic 向指定代理添加 Toxic
func addToxic(t *testing.T, proxyName, toxicName, toxicType, stream string, attrs map[string]interface{}) {
	body := map[string]interface{}{
		"name":       toxicName,
		"type":       toxicType,
		"stream":     stream,
		"toxicity":   1.0,
		"attributes": attrs,
	}
	buf, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/proxies/%s/toxics", toxiproxyAPI, proxyName)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Toxiproxy-Client")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("添加 Toxic 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("添加 Toxic %s 失败 (code=%d): %s", toxicName, resp.StatusCode, string(respBody))
	}
	t.Logf("[Toxiproxy Toxic 注入] 成功向 %s 注入 %s (%s, attrs=%v)", proxyName, toxicName, toxicType, attrs)
}

// setupToxiproxyContext 初始化直连 Toxiproxy 代理端口的业务测试上下文
func setupToxiproxyContext(t *testing.T) *ToxiproxyTestContext {
	os.Setenv("MYSQL_HOST", "127.0.0.1")
	os.Setenv("MYSQL_PORT", proxyMySQLPort)
	os.Setenv("MYSQL_USER", "root")
	os.Setenv("MYSQL_PASSWORD", "root123456")
	os.Setenv("DB_NAME", "chat")
	os.Setenv("REDIS_HOST", "127.0.0.1")
	os.Setenv("REDIS_PORT", proxyRedisPort)
	os.Setenv("REDIS_PASSWORD", "")

	_ = config.GetConfig()

	dsn := fmt.Sprintf("root:root123456@tcp(127.0.0.1:%s)/chat?charset=utf8mb4&parseTime=True&loc=Local", proxyMySQLPort)
	db, err := gorm.Open(mysqldriver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("穿越 Toxiproxy 连接 MySQL 失败: %v", err)
	}

	err = db.AutoMigrate(
		&model.UserInfo{},
		&model.Friendship{},
		&model.Apply{},
		&model.GroupInfo{},
		&model.GroupMember{},
		&model.Session{},
		&model.Message{},
		&model.Outbox{},
	)
	if err != nil {
		t.Fatalf("自动迁移表结构失败: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:" + proxyRedisPort,
		DB:   0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("穿越 Toxiproxy 连接 Redis 失败: %v", err)
	}

	stores := mysql.NewStores(db)
	cachePort := myredis.NewRedisCache(rdb, 15)
	guard := idempotent.NewRedisGuard(rdb)

	grpc_client.UserClient = &dummyUserServiceClient{}

	return &ToxiproxyTestContext{
		DB:            db,
		RDB:           rdb,
		Stores:        stores,
		CachePort:     cachePort,
		UserService:   userapp.NewUserService(stores, cachePort, guard),
		FriendshipSvc: friendshipapp.NewFriendshipService(stores, cachePort),
		ApplyService:  applyapp.NewApplyService(stores, cachePort, guard),
		GroupService:  groupapp.NewGroupService(stores, cachePort, guard),
		MessageSvc:    messageapp.NewMessageService(stores.Message, stores.Session, cachePort, guard),
		SessionSvc:    sessionapp.NewSessionService(stores.Session, stores.Message, cachePort, guard),
	}
}

// testUserWrites 测试用户服务写接口 (Register, UpdateUserInfo)
func testUserWrites(t *testing.T, tc *ToxiproxyTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	username := "u_" + tag
	clientToken := "tok_" + tag

	start := time.Now()
	// Step 1: 执行注册写接口 (穿过 Toxiproxy 300ms 延迟)
	regRsp, err := tc.UserService.Register(ctx, authreq.RegisterRequest{
		Username:    username,
		Password:    "Password123!",
		ClientToken: clientToken,
	}, "127.0.0.1")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("[User 注册写失败]: %v", err)
	}
	t.Logf("[User 注册成功] 用户UUID=%s, 穿越 Toxiproxy 耗时: %v", regRsp.Uuid, elapsed)

	// Step 2: 执行资料更新写接口
	newNick := "N_" + tag
	start = time.Now()
	err = tc.UserService.UpdateUserInfo(ctx, regRsp.Uuid, userreq.UpdateUserInfoRequest{
		Nickname: &newNick,
	})
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("[User 资料更新写失败]: %v", err)
	}
	t.Logf("[User 资料更新成功] 穿越 Toxiproxy 耗时: %v", elapsed)
}

// testFriendshipWrites 测试好友服务写接口 (BlackFriend, UnblackFriend, UpdateRemark)
func testFriendshipWrites(t *testing.T, tc *ToxiproxyTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	userA := "UA_" + tag
	userB := "UB_" + tag

	// 初始化双向正常好友记录
	fsAB := &model.Friendship{UserId: userA, FriendId: userB, Status: friendship_status.NORMAL, Version: 1}
	fsBA := &model.Friendship{UserId: userB, FriendId: userA, Status: friendship_status.NORMAL, Version: 1}
	tc.DB.Create(fsAB)
	tc.DB.Create(fsBA)

	// Step 1: 执行修改备注写接口
	remark := "Rem_" + tag
	err := tc.FriendshipSvc.UpdateRemark(ctx, userA, userB, remark)
	if err != nil {
		t.Fatalf("[Friendship 修改备注写失败]: %v", err)
	}
	t.Logf("[Friendship 修改备注写成功]")

	// Step 2: 执行拉黑写接口 (双向更新 + Version CAS + Outbox)
	start := time.Now()
	err = tc.FriendshipSvc.BlackFriend(ctx, userA, userB, 1)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("[Friendship 拉黑写失败]: %v", err)
	}
	t.Logf("[Friendship 拉黑写成功] 穿越 Toxiproxy 耗时: %v (事务含 2 次 Update + 1 次 Insert)", elapsed)

	// Step 3: 执行解拉黑写接口 (状态机复位 + Version CAS)
	start = time.Now()
	err = tc.FriendshipSvc.UnblackFriend(ctx, userA, userB, 2)
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("[Friendship 解黑写失败]: %v", err)
	}
	t.Logf("[Friendship 解黑写成功] 穿越 Toxiproxy 耗时: %v (版本已自增到 3)", elapsed)

	// Step 4: 弱网下模拟持旧版本 1 的迟到拉黑包，验证 Version CAS 拦截
	lateErr := tc.FriendshipSvc.BlackFriend(ctx, userA, userB, 1)
	if lateErr == nil {
		t.Fatalf("[Friendship 严重漏洞] Toxiproxy 弱网下持旧版本 1 的迟到包居然未被拦截！")
	}
	t.Logf("[Friendship 拦截成功] Toxiproxy 弱网下迟到包被 Version CAS 拦截: %v", lateErr)
}

// testApplyWrites 测试联系人申请写接口 (ApplyFriend, PassFriendApply)
func testApplyWrites(t *testing.T, tc *ToxiproxyTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	applicant := "AppA_" + tag
	target := "AppB_" + tag

	// Step 1: 发起好友申请写接口
	start := time.Now()
	err := tc.ApplyService.ApplyFriend(ctx, applicant, applyreq.ApplyFriendRequest{
		FriendId: target,
		Message:  "Hi Toxi",
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("[Apply 发起好友申请写失败]: %v", err)
	}
	t.Logf("[Apply 发起好友申请写成功] 穿越 Toxiproxy 耗时: %v", elapsed)

	// Step 2: 审批通过好友申请写接口 (带版本校验与 Outbox 事件投递)
	start = time.Now()
	err = tc.ApplyService.PassFriendApply(ctx, target, applicant)
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("[Apply 审批通过好友申请写失败]: %v", err)
	}
	t.Logf("[Apply 审批通过写成功] 穿越 Toxiproxy 耗时: %v", elapsed)

	// Step 3: 幂等自愈复测：再次审批通过同一申请，终态自愈
	errAgain := tc.ApplyService.PassFriendApply(ctx, target, applicant)
	if errAgain != nil {
		t.Fatalf("[Apply 终态自愈幂等失败]: %v", errAgain)
	}
	t.Logf("[Apply 审批幂等自愈成功] 重复审批直接放行且零副作用")
}

// testGroupWrites 测试群组写接口 (CreateGroup, UpdateGroupInfo, LeaveGroup)
func testGroupWrites(t *testing.T, tc *ToxiproxyTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	ownerId := "Own_" + tag
	memberA := "MemA_" + tag

	// Step 1: 创建群组写接口
	start := time.Now()
	groupId, err := tc.GroupService.CreateGroup(ctx, ownerId, groupreq.CreateGroupRequest{
		Name:   "G_" + tag,
		Notice: "Announce",
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("[Group 建群写失败]: %v", err)
	}
	t.Logf("[Group 建群写成功] 群ID=%s, 穿越 Toxiproxy 耗时: %v", groupId, elapsed)

	// 添加管理员成员用于协同
	tc.DB.Create(&model.GroupMember{
		GroupUuid: groupId,
		UserUuid:  memberA,
		Role:      2,
	})

	// 获取初始版本号
	grp, _ := tc.Stores.GroupStore().FindByUuid(ctx, groupId)
	baseVersion := grp.Version

	// Step 2: 基于 Version 乐观锁协同修改群公告写接口
	newNotice := "NewNotice_" + tag
	start = time.Now()
	err = tc.GroupService.UpdateGroupInfo(ctx, ownerId, groupreq.UpdateGroupInfoRequest{
		Uuid:    groupId,
		Notice:  &newNotice,
		Version: &baseVersion,
	})
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("[Group 乐观锁修改群公告写失败]: %v", err)
	}
	t.Logf("[Group 乐观锁修改资料成功] 穿越 Toxiproxy 耗时: %v (版本已自增为 %d)", elapsed, baseVersion+1)

	// Step 3: 并发冲突拦截测试：携带过期版本再次修改，应返回 409 Conflict
	conflictNotice := "Stale"
	conflictErr := tc.GroupService.UpdateGroupInfo(ctx, memberA, groupreq.UpdateGroupInfoRequest{
		Uuid:    groupId,
		Notice:  &conflictNotice,
		Version: &baseVersion, // 仍然使用旧版本 baseVersion
	})
	if conflictErr == nil {
		t.Fatalf("[Group 严重漏洞] Toxiproxy 弱网下持过期版本的协同编辑未被 409 拦截！")
	}
	if !errorx.IsCode(conflictErr, errorx.CodeConflict) {
		t.Fatalf("[Group 错误码不匹配] 期望 409 Conflict，实际: %v", conflictErr)
	}
	t.Logf("[Group 冲突拦截成功] 协同编辑过期版本被精准拦截为 409 Conflict: %v", conflictErr)

	// Step 4: 成员退群写接口
	err = tc.GroupService.LeaveGroup(ctx, memberA, groupId)
	if err != nil {
		t.Fatalf("[Group 成员退群写失败]: %v", err)
	}
	t.Logf("[Group 成员退群写成功]")
}

// testMessageWrites 测试消息写接口 (Create, UpdateContent 撤回单向 CAS)
func testMessageWrites(t *testing.T, tc *ToxiproxyTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	sendId := "US_" + tag
	recvId := "UR_" + tag
	clientMsgId := fmt.Sprintf("cmsg_%d", time.Now().UnixNano()%1000000)
	sessionId := "P_" + sendId + "_" + recvId

	// Step 1: 消息落库写接口 (带唯一键约束)
	msgUuid := "M" + snowflake.GenerateIDString()
	msg := model.Message{
		Uuid:        msgUuid,
		ClientMsgId: clientMsgId,
		SessionId:   sessionId,
		SendId:      sendId,
		ReceiveId:   recvId,
		Type:        int8(message_type.Text),
		Content:     "Hello Toxiproxy",
	}
	start := time.Now()
	err := tc.Stores.MessageStore().Create(ctx, &msg)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("[Message 消息落库写失败]: %v", err)
	}
	t.Logf("[Message 消息落库写成功] MsgId=%s, 穿越 Toxiproxy 耗时: %v", msgUuid, elapsed)

	// Step 2: 撤回消息写接口 (UpdateContent 单向 CAS 状态机流转)
	start = time.Now()
	ok, err := tc.Stores.MessageStore().UpdateContent(ctx, msgUuid, "", int8(message_type.Recall))
	elapsed = time.Since(start)
	if err != nil || !ok {
		t.Fatalf("[Message 撤回消息写失败]: ok=%v, err=%v", ok, err)
	}
	t.Logf("[Message 撤回消息写成功] 穿越 Toxiproxy 耗时: %v", elapsed)

	// Step 3: 再次撤回同一消息，验证单向 CAS 状态机防御 (应当返回 ok=false)
	okAgain, errAgain := tc.Stores.MessageStore().UpdateContent(ctx, msgUuid, "", int8(message_type.Recall))
	if errAgain != nil {
		t.Fatalf("[Message 重复撤回底层报错]: %v", errAgain)
	}
	if okAgain {
		t.Fatalf("[Message 严重漏洞] 已撤回的消息重复撤回状态机竟然再次放行成功！")
	}
	t.Logf("[Message 撤回防重拦截成功] 重复撤回 RowsAffected==0 (ok=false)")
}

// testSessionWrites 测试会话写接口 (PinSession)
func testSessionWrites(t *testing.T, tc *ToxiproxyTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("%d", time.Now().UnixNano()%10000)
	userId := "UP_" + tag
	sessionUuid := "SES_" + tag

	// 初始化会话记录
	sess := &model.Session{
		Uuid:      sessionUuid,
		SendId:    userId,
		ReceiveId: "UT_" + tag,
		IsPinned:  false,
	}
	tc.DB.Create(sess)

	// Step 1: 会话置顶写接口 (原子翻转 is_pinned)
	start := time.Now()
	err := tc.SessionSvc.PinSession(ctx, userId, sessionUuid, true)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("[Session 置顶会话写失败]: %v", err)
	}
	t.Logf("[Session 置顶会话写成功] 穿越 Toxiproxy 耗时: %v", elapsed)

	// 验证数据库状态更新已生效
	var cur model.Session
	tc.DB.Where("uuid = ?", sessionUuid).First(&cur)
	if !cur.IsPinned {
		t.Fatalf("[Session 置顶未生效] 期望 IsPinned=true")
	}
	t.Logf("[Session 状态校验通过] 会话置顶原子持久化生效")
}
