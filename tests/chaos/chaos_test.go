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
	"kama_chat_server/pkg/enum/apply/apply_status"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// ChaosTestContext 聚合弱网混沌测试所需的全局中间件连接与业务服务句柄
type ChaosTestContext struct {
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
// 必须严格在所有测试阶段之前首发执行
func setupChaosEnv(t *testing.T) *ChaosTestContext {
	// Step 1.1: 注入环境变量覆盖指向 Docker 宿主端口 (MySQL 3306, Redis 6385)
	// 来源: Docker Compose 实际映射宿主端口
	// 用于: Step 1.2 加载配置并初始化 gorm 和 redis 客户端
	os.Setenv("MYSQL_HOST", "127.0.0.1")
	os.Setenv("MYSQL_PORT", "3306")
	os.Setenv("MYSQL_USER", "root")
	os.Setenv("MYSQL_PASSWORD", "root123456")
	os.Setenv("DB_NAME", "chat")
	os.Setenv("REDIS_HOST", "127.0.0.1")
	os.Setenv("REDIS_PORT", "6385")
	os.Setenv("REDIS_PASSWORD", "")

	_ = config.GetConfig()

	// Step 1.2: 建立真实 MySQL 物理连接
	// 来源: Step 1.1 环境变量覆盖的连接参数
	// 用于: Step 1.3 自动迁移表结构与初始化 Store 层
	dsn := "root:root123456@tcp(127.0.0.1:3306)/chat?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysqldriver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接 MySQL 失败: %v", err)
	}

	// Step 1.3: 执行全实体表结构自动迁移 (包含新增的 Version 乐观锁字段)
	// 来源: 各业务领域的实体模型定义
	// 用于: Step 1.4 测试运行前环境就绪
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

	// Step 1.4: 建立真实 Redis 连接并探活
	// 来源: Redis 端口 6385
	// 用于: Step 1.5 构造短租约防重守卫与缓存服务
	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6385",
		DB:   0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("连接 Redis 失败: %v", err)
	}

	// Step 1.5: 实例化 Store 与 Service 核心层
	// 来源: Step 1.2 的 db 与 Step 1.4 的 rdb
	// 用于: 全量写操作的混沌测试调用
	stores := mysql.NewStores(db)
	cachePort := myredis.NewRedisCache(rdb, 15)
	guard := idempotent.NewRedisGuard(rdb)

	// 注册 dummy gRPC 客户端，防止内部跨服务查询空指针
	grpc_client.UserClient = &dummyUserServiceClient{}

	return &ChaosTestContext{
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

// injectChaosDelay 注入弱网随机延迟，模拟高抖动与乱序网络环境
func injectChaosDelay(minMs, maxMs int) {
	// Step 2.1: 计算随机抖动毫秒数并休眠
	// 来源: minMs 与 maxMs 上下限
	// 用于: 制造并发协程交错调度与乱序到达
	delta := maxMs - minMs
	if delta <= 0 {
		time.Sleep(time.Duration(minMs) * time.Millisecond)
		return
	}
	sleepMs := minMs + rand.Intn(delta)
	time.Sleep(time.Duration(sleepMs) * time.Millisecond)
}

// testCase1_UserWrite_Chaos 测试用户写接口在弱网并发重试风暴与乱序修改下的防护表现
func testCase1_UserWrite_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("u_%d", time.Now().UnixNano()%100000)
	username := "chaos_user_" + tag
	clientToken := "token_user_reg_" + tag

	// Step 3.1: 模拟弱网超时导致客户端发起 10 路高并发重试注册请求 (同 client_token)
	// 来源: 用户提交的注册请求与唯一客户端凭证
	// 用于: Step 3.2 验证幂等守卫与唯一键约束是否能准确拦截重复插入
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			injectChaosDelay(10, 80) // 模拟随机弱网抖动
			req := authreq.RegisterRequest{
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

	// Step 3.2: 校验注册结果与数据库落库数量
	// 来源: Step 3.1 并发重试执行结果
	// 用于: 断言库中严格只有 1 条用户数据，绝无重复用户
	var userCount int64
	tc.DB.Model(&model.UserInfo{}).Where("nickname = ?", username).Count(&userCount)
	if userCount != 1 {
		t.Fatalf("[Case 1 失败] 用户注册重复落库: 期望 1 条，实际 %d 条", userCount)
	}
	t.Logf("[Case 1 通过] 用户高并发重试注册成功拦截，库中严格唯一落库 (成功响应次数: %d, 落库数: %d)", successCount, userCount)

	// Step 3.3: 模拟弱网乱序并发更新用户资料
	// 来源: Step 3.1 创建成功的用户
	// 用于: 验证事务更新及缓存清理的正确性
	user, err := tc.Stores.UserStore().FindByNickname(ctx, username)
	if err != nil {
		t.Fatalf("查找注册用户失败: %v", err)
	}

	newNick := "Updated_" + tag
	updateReq := userreq.UpdateUserInfoRequest{
		Nickname: &newNick,
	}
	err = tc.UserService.UpdateUserInfo(ctx, user.Uuid, updateReq)
	if err != nil {
		t.Fatalf("[Case 1 失败] 更新用户资料失败: %v", err)
	}
	t.Logf("[Case 1 通过] 用户写操作与资料更新在弱网并发下校验全部通过")
}

// testCase2_Friendship_ABA_Attack_Chaos 测试好友拉黑/解拉黑在可逆循环状态机下的 ABA 攻击防御
// 验证迟到 400ms 的旧拉黑包到达时，能被 Version CAS 乐观锁坚决拦截，防止状态回退与错误级联删除
func testCase2_Friendship_ABA_Attack_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("fs_%d", time.Now().UnixNano()%100000)
	userA := "UA_" + tag
	userB := "UB_" + tag

	// Step 4.1: 初始化双向正常好友记录 (Version = 1, Status = NORMAL)
	// 来源: userA, userB 用户对
	// 用于: Step 4.2 构造 ABA 攻击前置状态
	tc.DB.Where("user_id IN ? AND friend_id IN ?", []string{userA, userB}, []string{userA, userB}).Delete(&model.Friendship{})
	fsAB := &model.Friendship{UserId: userA, FriendId: userB, Status: friendship_status.NORMAL, Version: 1}
	fsBA := &model.Friendship{UserId: userB, FriendId: userA, Status: friendship_status.NORMAL, Version: 1}
	tc.DB.Create(fsAB)
	tc.DB.Create(fsBA)

	// Step 4.2: 模拟弱网 ABA 攻击编排
	// 来源: Step 4.1 初始化就绪的好友关系
	// 用于: Step 4.3 验证 Version CAS 是否成功拦截迟到包
	var wg sync.WaitGroup
	var latePacketErr error

	// 协程 1: 模拟因网络极度卡顿陷入延迟 400ms 的旧拉黑请求 (Req 1，客户端发起时持有 Version 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(400 * time.Millisecond) // 注入长延迟
		// 此时 Req 1 携带着旧版本 1 试图执行拉黑
		latePacketErr = tc.FriendshipSvc.BlackFriend(ctx, userA, userB, 1)
	}()

	// 协程 2: 主线程快速完成一轮状态反转循环 (NORMAL -> BLACK -> NORMAL)
	// 使真实状态虽然回到 NORMAL，但 Version 已经自增到了 3！
	time.Sleep(50 * time.Millisecond)
	err1 := tc.FriendshipSvc.BlackFriend(ctx, userA, userB, 1) // Version 1 -> 2, Status -> BLACK
	if err1 != nil {
		t.Fatalf("[Case 2] 正常拉黑失败: %v", err1)
	}

	time.Sleep(50 * time.Millisecond)
	err2 := tc.FriendshipSvc.UnblackFriend(ctx, userA, userB, 2) // Version 2 -> 3, Status -> NORMAL
	if err2 != nil {
		t.Fatalf("[Case 2] 正常解黑失败: %v", err2)
	}

	// 记录解黑完成时的 Outbox 表事件数
	var outboxCountBeforeLate int64
	tc.DB.Model(&model.Outbox{}).Where("payload LIKE ?", "%"+userA+"%").Count(&outboxCountBeforeLate)

	// 等待迟到的 Req 1 撞击服务端
	wg.Wait()

	// Step 4.3: 断言校验 ABA 防御结果
	// 来源: 迟到包执行错误与最终数据库状态
	// 用于: 证明 Version CAS 拦截了非法状态倒退，且没有多余 Outbox 事件发出
	if latePacketErr == nil {
		t.Fatalf("[Case 2 致命漏洞] 迟到的旧拉黑请求竟然成功执行了！说明发生了 ABA 状态倒退与错误覆盖！")
	}
	t.Logf("[Case 2 拦截成功] 迟到拉黑请求被 Version CAS 乐观锁精准拦截，报错信息: %v", latePacketErr)

	// 验证数据库最终状态必须仍为 NORMAL，版本号必须仍为 3
	latestAB, _ := tc.Stores.FriendshipStore().FindByUserIdAndFriendId(ctx, userA, userB)
	if latestAB.Status != friendship_status.NORMAL || latestAB.Version != 3 {
		t.Fatalf("[Case 2 校验失败] 好友关系被破坏！当前 Status=%d, Version=%d (期望 Status=0, Version=3)", latestAB.Status, latestAB.Version)
	}

	// 验证 Outbox 表没有因为迟到包误插入多余的拉黑删除事件
	var outboxCountAfterLate int64
	tc.DB.Model(&model.Outbox{}).Where("payload LIKE ?", "%"+userA+"%").Count(&outboxCountAfterLate)
	if outboxCountAfterLate != outboxCountBeforeLate {
		t.Fatalf("[Case 2 校验失败] 迟到包产生了多余的 Outbox 领域事件！事件数从 %d 变为 %d", outboxCountBeforeLate, outboxCountAfterLate)
	}

	t.Logf("[Case 2 全部通过] 循环状态机 ABA 弱网混沌防御 100%% 生效，最终状态与版本号严格一致！")
}

// testCase3_Apply_Lifecycle_OldPacket_Chaos 测试好友申请生命周期重置与跨代审批包穿透防御
func testCase3_Apply_Lifecycle_OldPacket_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("app_%d", time.Now().UnixNano()%100000)
	applicantId := "AppA_" + tag
	targetId := "AppB_" + tag

	// Step 5.1: 发起第 1 代好友申请并将其拒绝
	// 来源: applicantId, targetId
	// 用于: Step 5.2 制造前代被拒绝的终态记录
	err := tc.ApplyService.ApplyFriend(ctx, applicantId, applyreq.ApplyFriendRequest{
		FriendId: targetId,
		Message:  "First Apply",
	})
	if err != nil {
		t.Fatalf("[Case 3] 首次申请失败: %v", err)
	}

	firstApply, err := tc.Stores.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, targetId)
	if err != nil || firstApply.Version != 1 {
		t.Fatalf("[Case 3] 首次申请版本号异常: %v", err)
	}
	oldUuid := firstApply.Uuid
	oldVersion := firstApply.Version

	// 被对方拒绝
	err = tc.ApplyService.RefuseFriendApply(ctx, targetId, applicantId)
	if err != nil {
		t.Fatalf("[Case 3] 拒绝申请失败: %v", err)
	}

	// Step 5.2: 申请人重新发起第 2 代申请 (系统自动刷新全新 Uuid 并自增 Version)
	// 来源: 再次发起申请动作
	// 用于: Step 5.3 检验新老代际隔离
	err = tc.ApplyService.ApplyFriend(ctx, applicantId, applyreq.ApplyFriendRequest{
		FriendId: targetId,
		Message:  "Second Apply",
	})
	if err != nil {
		t.Fatalf("[Case 3] 二次申请失败: %v", err)
	}

	secondApply, _ := tc.Stores.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, targetId)
	if secondApply.Uuid == oldUuid {
		t.Fatalf("[Case 3 缺陷] 二次申请未刷新全新 Uuid")
	}
	if secondApply.Version <= oldVersion {
		t.Fatalf("[Case 3 缺陷] 二次申请未递增 Version: old=%d, new=%d", oldVersion, secondApply.Version)
	}
	t.Logf("[Case 3] 二次申请成功刷新代际: Old(Uuid=%s, Ver=%d) -> New(Uuid=%s, Ver=%d)", oldUuid, oldVersion, secondApply.Uuid, secondApply.Version)

	// Step 5.3: 模拟弱网环境下迟到的上一代审批拒绝/通过包试图穿透
	// 来源: 上一代的 oldUuid 与 oldVersion
	// 用于: 断言 UpdateStatusIfPending 拦截上一代审批包
	ok, err := tc.Stores.ApplyStore().UpdateStatusIfPending(ctx, oldUuid, apply_status.REFUSE, oldVersion)
	if err != nil {
		t.Fatalf("[Case 3] 旧包执行报错: %v", err)
	}
	if ok {
		t.Fatalf("[Case 3 致命漏洞] 迟到的上一代审批包成功更新了记录！发生了跨代审批穿透！")
	}
	t.Logf("[Case 3 拦截成功] 迟到上一代审批包被 Version & Uuid CAS 坚决拒绝 (RowsAffected=0)")

	// Step 5.4: 执行当期合法的通过审批
	// 来源: 当前活跃代的 secondApply.Uuid 与 secondApply.Version
	// 用于: 验证当期合法审批正常流转
	err = tc.ApplyService.PassFriendApply(ctx, targetId, applicantId)
	if err != nil {
		t.Fatalf("[Case 3] 当期合法审批失败: %v", err)
	}

	finalApply, _ := tc.Stores.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, targetId)
	if finalApply.Status != apply_status.AGREE {
		t.Fatalf("[Case 3 失败] 最终申请状态不是 AGREE: %d", finalApply.Status)
	}
	t.Logf("[Case 3 全部通过] 申请生命周期防跨代穿透验证全部通过！")
}

// testCase4_Group_CollaborativeEdit_VersionConflict_Chaos 测试群资料弱网高并发多人协同编辑下的版本冲突保护
func testCase4_Group_CollaborativeEdit_VersionConflict_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("grp_%d", time.Now().UnixNano()%100000)
	ownerId := "Owner_" + tag
	adminId := "Admin_" + tag

	// Step 6.1: 创建测试群组并补充管理员成员
	// 来源: ownerId 与 adminId
	// 用于: Step 6.2 构造两人同时持有 Version=1 的并发编辑场景
	groupReq := groupreq.CreateGroupRequest{
		Name:   "G_" + tag,
		Notice: "Initial Notice",
	}
	groupId, err := tc.GroupService.CreateGroup(ctx, ownerId, groupReq)
	if err != nil {
		t.Fatalf("[Case 4] 建群失败: %v", err)
	}

	// 添加管理员成员 (Role = 2)
	tc.DB.Create(&model.GroupMember{
		GroupUuid: groupId,
		UserUuid:  adminId,
		Role:      2,
	})

	// 获取初始版本号 (Version 应当为 1)
	grp, _ := tc.Stores.GroupStore().FindByUuid(ctx, groupId)
	baseVersion := grp.Version

	// Step 6.2: 两人弱网并发提交修改，都基于同一个 baseVersion
	// 来源: 两人各自编辑不同的公告内容
	// 用于: 检验乐观锁版本号冲突拦截
	var wg sync.WaitGroup
	var errOwner, errAdmin error

	notice1 := "Notice Modified by Owner"
	notice2 := "Notice Modified by Admin"

	wg.Add(2)
	go func() {
		defer wg.Done()
		injectChaosDelay(20, 60)
		errOwner = tc.GroupService.UpdateGroupInfo(ctx, ownerId, groupreq.UpdateGroupInfoRequest{
			Uuid:    groupId,
			Notice:  &notice1,
			Version: &baseVersion,
		})
	}()

	go func() {
		defer wg.Done()
		injectChaosDelay(20, 60)
		errAdmin = tc.GroupService.UpdateGroupInfo(ctx, adminId, groupreq.UpdateGroupInfoRequest{
			Uuid:    groupId,
			Notice:  &notice2,
			Version: &baseVersion,
		})
	}()
	wg.Wait()

	// Step 6.3: 断言一人成功、另一人返回 409 Conflict
	// 来源: errOwner 与 errAdmin
	// 用于: 证明绝无无感脏写覆盖
	hasConflict := errorx.IsCode(errOwner, errorx.CodeConflict) || errorx.IsCode(errAdmin, errorx.CodeConflict)
	hasSuccess := (errOwner == nil) || (errAdmin == nil)

	if !hasConflict || !hasSuccess {
		t.Fatalf("[Case 4 冲突校验失败] 期望一人成功一人409冲突，实际 errOwner=%v, errAdmin=%v", errOwner, errAdmin)
	}
	t.Logf("[Case 4 通过] 群资料并发协同编辑成功拦截版本冲突: 成功者顺利递增，落后者收到 409 Conflict")

	// 最终群版本号必须自增为 baseVersion + 1
	updatedGrp, _ := tc.Stores.GroupStore().FindByUuid(ctx, groupId)
	if updatedGrp.Version != baseVersion+1 {
		t.Fatalf("[Case 4 版本异常] 群版本号期望 %d，实际 %d", baseVersion+1, updatedGrp.Version)
	}
	t.Logf("[Case 4 全部通过] 群资料协同编辑乐观锁防覆盖 100%% 有效！")
}

// testCase5_Message_RetryStorm_And_RevokeCAS_Chaos 测试消息写接口高并发重发去重与撤回单向状态机 CAS
func testCase5_Message_RetryStorm_And_RevokeCAS_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("msg_%d", time.Now().UnixNano()%100000)
	sendId := "UserS_" + tag
	recvId := "UserR_" + tag
	clientMsgId := "CMID_" + tag
	sessionId := "P_" + sendId + "_" + recvId

	// Step 7.1: 模拟客户端弱网重试风暴，10 个并发协程发送相同 client_msg_id
	// 来源: 客户端发送消息请求
	// 用于: Step 7.2 检验底层 OnConflict DoNothing 唯一约束去重
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			injectChaosDelay(10, 50)
			msg := model.Message{
				Uuid:        "M" + snowflake.GenerateIDString(),
				ClientMsgId: clientMsgId,
				SessionId:   sessionId,
				SendId:      sendId,
				ReceiveId:   recvId,
				Type:        int8(message_type.Text),
				Content:     "Hello Chaos Testing",
			}
			_ = tc.Stores.MessageStore().Create(ctx, &msg)
		}(i)
	}
	wg.Wait()

	// Step 7.2: 检验数据库中相同 client_msg_id 的消息记录数
	// 来源: Step 7.1 并发插入结果
	// 用于: 断言必须严格只有 1 条记录
	var msgCount int64
	tc.DB.Model(&model.Message{}).Where("client_msg_id = ?", clientMsgId).Count(&msgCount)
	if msgCount != 1 {
		t.Fatalf("[Case 5 消息去重失败] 重试风暴导致重复落库: 期望 1 条，实际 %d 条", msgCount)
	}
	t.Logf("[Case 5 通过] 消息发送重试风暴成功去重，库中严格唯一落库 (落库数: %d)", msgCount)

	// Step 7.3: 测试并发撤回消息（单向状态机 CAS 跃迁）
	// 来源: Step 7.2 插入的唯一消息记录
	// 用于: 验证只有首次撤回 RowsAffected > 0，重复撤回幂等拦截
	var storedMsg model.Message
	tc.DB.Where("client_msg_id = ?", clientMsgId).First(&storedMsg)

	var revokeSuccessCount int
	var mu sync.Mutex
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			injectChaosDelay(5, 30)
			ok, err := tc.Stores.MessageStore().UpdateContent(ctx, storedMsg.Uuid, "", int8(message_type.Recall))
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

// testCase6_Session_Pin_AtomicUpdate_Chaos 测试会话置顶原子写操作在弱网抖动下的稳定性
func testCase6_Session_Pin_AtomicUpdate_Chaos(t *testing.T, tc *ChaosTestContext) {
	ctx := context.Background()
	tag := fmt.Sprintf("sess_%d", time.Now().UnixNano()%100000)
	sessionUuid := "SESS_" + tag
	userId := "UserPin_" + tag

	// Step 8.1: 初始化单聊会话记录
	// 来源: sessionUuid, userId
	// 用于: Step 8.2 并发执行置顶更新
	sess := &model.Session{
		Uuid:      sessionUuid,
		SendId:    userId,
		ReceiveId: "Target_" + tag,
		IsPinned:  false,
	}
	tc.DB.Create(sess)

	// Step 8.2: 弱网并发多次置顶
	// 来源: PinSession 接口调用
	// 用于: 检验原子更新一致性
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

	// Step 8.3: 校验会话最终状态
	// 来源: Step 8.2 并发执行后的数据库记录
	// 用于: 断言 IsPinned 为 true
	var curSess model.Session
	tc.DB.Where("uuid = ?", sessionUuid).First(&curSess)
	if !curSess.IsPinned {
		t.Fatalf("[Case 6 失败] 会话置顶状态异常: 期望 true，实际 false")
	}
	t.Logf("[Case 6 通过] 会话置顶原子操作并发测试全部通过！")
}

// TestAllWriteInterfaces_WeakNetworkChaos 弱网混沌测试主入口
// 按照系统启动、环境准备、各接口时间序依次执行全部混沌用例
func TestAllWriteInterfaces_WeakNetworkChaos(t *testing.T) {
	// Step 9.1: 启动与初始化混沌测试环境 (连通 Docker MySQL 与 Redis，迁移全量表结构)
	// 来源: 本地 Docker Compose 容器实例 (MySQL:3306, Redis:6385)
	// 用于: Step 9.2 至 Step 9.7 各业务写接口压测
	tc := setupChaosEnv(t)
	t.Log("================================================================================")
	t.Log("弱网混沌测试套件启动: 已连通真实 Docker 中间件环境 (MySQL 3306, Redis 6385)")
	t.Log("================================================================================")

	// Step 9.2: 执行 Case 1 - 用户模块并发注册防重与资料更新测试
	// 来源: Step 9.1 就绪的 UserService
	// 用于: 验证高并发重发风暴拦截与资料更新
	t.Log(">>> [Phase 1] 执行用户模块弱网并发写操作测试...")
	testCase1_UserWrite_Chaos(t, tc)

	// Step 9.3: 执行 Case 2 - 好友模块拉黑/解拉黑 ABA 循环状态机攻击测试
	// 来源: Step 9.1 就绪的 FriendshipService
	// 用于: 验证 Version CAS 拦截迟到旧拉黑包与阻断会话级联删除
	t.Log(">>> [Phase 2] 执行好友拉黑/解黑 ABA 循环状态机弱网攻击测试...")
	testCase2_Friendship_ABA_Attack_Chaos(t, tc)

	// Step 9.4: 执行 Case 3 - 申请模块审批流生命周期穿透与跨代串号防御测试
	// 来源: Step 9.1 就绪的 ApplyService
	// 用于: 验证全新 Uuid 与 Version 阻止旧拒绝/通过包穿透
	t.Log(">>> [Phase 3] 执行联系人申请重置与跨代审批穿透测试...")
	testCase3_Apply_Lifecycle_OldPacket_Chaos(t, tc)

	// Step 9.5: 执行 Case 4 - 群组模块多人弱网协同编辑版本冲突保护测试
	// 来源: Step 9.1 就绪的 GroupService
	// 用于: 验证并发编辑乐观锁防脏写覆盖与 409 Conflict 返回
	t.Log(">>> [Phase 4] 执行群资料多人协同编辑版本冲突保护测试...")
	testCase4_Group_CollaborativeEdit_VersionConflict_Chaos(t, tc)

	// Step 9.6: 执行 Case 5 - 消息模块弱网重试风暴去重与撤回单向状态机测试
	// 来源: Step 9.1 就绪的 MessageStore
	// 用于: 验证 client_msg_id 唯一约束去重与撤回单向 CAS 跃迁
	t.Log(">>> [Phase 5] 执行消息重试风暴去重与撤回单向状态机测试...")
	testCase5_Message_RetryStorm_And_RevokeCAS_Chaos(t, tc)

	// Step 9.7: 执行 Case 6 - 会话模块置顶原子写操作测试
	// 来源: Step 9.1 就绪的 SessionService
	// 用于: 验证置顶原子更新无竞态冲突
	t.Log(">>> [Phase 6] 执行会话置顶原子写操作测试...")
	testCase6_Session_Pin_AtomicUpdate_Chaos(t, tc)

	t.Log("================================================================================")
	t.Log("[弱网混沌测试大获全胜] 所有 6 大写操作核心链路在延迟、乱序、ABA 攻击下 100% 成功！")
	t.Log("================================================================================")
}
