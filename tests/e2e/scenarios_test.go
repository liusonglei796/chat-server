package e2e_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Test_BlockFriend_WeakNetwork_Chaos 好友拉黑接口在弱网/故障注入下的黑盒验证
//
// 场景矩阵（16 项）：
//
//	B0  基线正常拉黑            P0   双向状态/版本/outbox/会话软删/WS 通知
//	B1  重复拉黑（幂等）        P0   必须拒绝且无副作用
//	B2  拉黑自己                P1
//	B3  非好友拉黑              P0
//	B4  未认证（网关路径）      P0
//	B5  越权（Envoy body 冒名）  P0 安全
//	B6  错误 HTTP 动词          P1
//	B7  两条路径的错误契约一致性  P1
//	B8  熔断器行为              P1
//	F1  MySQL 延迟 30ms         P0
//	F2  MySQL 延迟 300ms        P0   原子性：全成功或全回滚
//	F3  MySQL 断连              P0   原子性
//	F4  gRPC 跳延迟             P1   至多一次
//	F5  Redis 断连              P1
//	A1  ABA 乱序（API 层）      P0
//	A2  异步半程（Kafka 恰好一条）P0
func Test_BlockFriend_WeakNetwork_Chaos(t *testing.T) {
	e := loadEnv(t)

	fsDB := e.openDB(t, "chat_friendship")
	msgDB := e.openDB(t, "chat_message")
	dbPing(t, fsDB)
	dbPing(t, msgDB)

	events := newEventReader(t, e.KafkaBrokers)
	defer events.close()

	t.Logf("[环境] Envoy=%s Gateway=%s Toxiproxy=%s", e.EnvoyURL, e.GatewayURL, e.ToxiAPI)

	// -----------------------------------------------------------------
	// B0 基线正常拉黑
	// -----------------------------------------------------------------
	var base *friendPair
	t.Run("B0_基线正常拉黑", func(t *testing.T) {
		p, err := e.createFriendPair(t, fsDB)
		if err != nil {
			// 前置链路（注册/申请/审批）本身坏了 —— 记为缺陷并让后续依赖场景跳过
			record(t, verdict{
				ID: "B0", Level: "P0", Title: "基线正常拉黑：前置链路（申请/审批）就无法完成",
				Pass: false,
				Expected: "能通过真实链路造出一对互为好友的用户（申请 -> outbox -> CDC -> Kafka -> friendship_service）",
				Actual:  err.Error(),
				Note:    "前置链路不通时，后续所有依赖「一对好友」的场景都无法执行",
			})
			return
		}
		base = p
		// 消费位点预热：申请通过产生的 friend_apply_passed 事件会让
		// 三个消费组都建立并提交一次 offset，之后的 friend_blacked 事件才不会被 AtEnd 跳过
		time.Sleep(3 * time.Second)

		// 对端连上 WS，让网关把 user:gateway:<B> 写进 Redis
		wsB := dialWS(t, e.GatewayURL, base.BToken)
		defer wsB.close()
		e.waitGatewayRoute(t, base.BUserID, 20*time.Second)

		// 造一条会话夹具，验证「拉黑后双方会话被物理删除」
		sessUUID := "S" + fmt.Sprintf("%d", time.Now().UnixNano()%1e13)
		insertSession(t, msgDB, sessUUID, base.AUserID, base.BUserID)

		outboxBefore := outboxCount(t, fsDB, base.AUserID)
		sessionsBefore := sessionCount(t, msgDB, base.AUserID, base.BUserID)
		check(t, sessionsBefore == 1, "会话夹具未就绪，count=%d", sessionsBefore)

		r := e.blockViaEnvoy(t, base.AUserID, base.BUserID, nil)
		t.Logf("[B0] Envoy 拉黑响应: %s", summarize(r))

		state, stateOK := assertPairState(t, fsDB, base.AUserID, base.BUserID, 2, 1, 2)
		outboxAfter := outboxCount(t, fsDB, base.AUserID)

		// 异步半程：会话被物理删除（Unscoped().Delete，行直接消失）
		sessLeft, sessGone := waitSessionGone(t, msgDB, base.AUserID, base.BUserID, 45*time.Second)
		// 异步半程：对端收到 BLACK 系统通知
		blackMsg, gotBlack := wsB.waitMessage(t, 45*time.Second, func(b []byte) bool {
			s := string(b)
			return strings.Contains(s, "BLACK") || strings.Contains(s, "friend_blacked")
		})
		// Kafka 恰好一条 friend_blacked
		kafkaCnt := waitKafkaEvents(t, events, "friend_blacked", base.AUserID, 1, 30*time.Second)

		ok := stateOK &&
			outboxAfter == outboxBefore+1 &&
			sessGone &&
			gotBlack &&
			kafkaCnt == 1
		record(t, verdict{
			ID: "B0", Level: "P0", Title: "基线正常拉黑：双向状态/版本/Outbox/会话软删/WS 通知/Kafka 恰好一条",
			Pass: ok,
			Expected: "A.status=2(BLACK) B.status=1(BE_BLACK) version=2/2；outbox +1；" +
				"会话被物理删除(0 行)；对端 WS 收到 BLACK；Kafka 恰好 1 条 friend_blacked",
			Actual: fmt.Sprintf("%s；outbox %d->%d；会话剩余 %d 行(已删=%v)；WS收到BLACK=%v；Kafka 条数=%d",
				state, outboxBefore, outboxAfter, sessLeft, sessGone, gotBlack, kafkaCnt),
			Note:   noteIf(blackMsg != nil, string(blackMsg)),
		})
	})

	// -----------------------------------------------------------------
	// B1 重复拉黑（幂等）
	// -----------------------------------------------------------------
	if base == nil {
		t.Skip("B0 基线未成功（好友对无法建立），后续依赖场景全部跳过")
	}
	t.Run("B1_重复拉黑应被拒绝且无副作用", func(t *testing.T) {
		outboxBefore := outboxCount(t, fsDB, base.AUserID)
		r := e.blockViaEnvoy(t, base.AUserID, base.BUserID, nil)
		state, _ := assertPairState(t, fsDB, base.AUserID, base.BUserID, 2, 1, 2)
		outboxAfter := outboxCount(t, fsDB, base.AUserID)

		rejected := r.Status >= 400 || r.Code != 0 || r.Code == 1001 || r.Code == 1005
		ok := rejected && outboxAfter == outboxBefore
		record(t, verdict{
			ID: "B1", Level: "P0", Title: "重复拉黑必须被拒绝且不产生副作用",
			Pass: ok,
			Expected: "请求被拒绝；outbox 不再增加；状态/版本保持 A=2,B=1,v=2",
			Actual: fmt.Sprintf("%s；outbox %d->%d；%s", summarize(r), outboxBefore, outboxAfter, state),
			Note:   "注意：业务错误码 1001 在 gRPC 边界会退化为 1005/Unknown，见 B7",
		})
		breakerCooldown(t, "B1 连续业务失败会累计熔断器的 ConsecutiveFailures")
	})

	// -----------------------------------------------------------------
	// B2 拉黑自己
	// -----------------------------------------------------------------
	t.Run("B2_拉黑自己应被拒绝", func(t *testing.T) {
		r := e.blockViaEnvoy(t, base.AUserID, base.AUserID, nil)
		ok := r.Status >= 400 || r.Code == 1001 || r.Code == 1005
		record(t, verdict{
			ID: "B2", Level: "P1", Title: "拉黑自己必须被拒绝",
			Pass: ok,
			Expected: "被拒绝（业务码 1001「不能拉黑自己」）",
			Actual:  summarize(r),
		})
		breakerCooldown(t, "B2")
	})

	// -----------------------------------------------------------------
	// B3 非好友拉黑
	// -----------------------------------------------------------------
	t.Run("B3_非好友拉黑应被拒绝", func(t *testing.T) {
		xID, _ := e.registerAndLogin(t, fmt.Sprintf("e2e_x_%d", time.Now().UnixNano()%1e9), "Password123!")
		yID, _ := e.registerAndLogin(t, fmt.Sprintf("e2e_y_%d", time.Now().UnixNano()%1e9), "Password123!")
		r := e.blockViaEnvoy(t, xID, yID, nil)
		// 不能产生任何关系行
		_, exists := friendshipRow(t, fsDB, xID, yID)
		ok := (r.Status >= 400 || r.Code == 1007 || r.Code == 1005) && !exists
		record(t, verdict{
			ID: "B3", Level: "P0", Title: "非好友之间拉黑必须被拒绝，且不产生关系行",
			Pass: ok,
			Expected: "被拒绝（业务码 1007「你们还不是好友，无法拉黑」）；不产生 user_friendship 行",
			Actual:  fmt.Sprintf("%s；产生了关系行=%v", summarize(r), exists),
		})
		breakerCooldown(t, "B3")
	})

	// -----------------------------------------------------------------
	// B4 未认证
	// -----------------------------------------------------------------
	t.Run("B4_未认证访问网关拉黑应被拒绝", func(t *testing.T) {
		r := e.call(t, "PUT", e.GatewayURL+"/friends/block", "", map[string]any{
			"friend_id": base.BUserID,
		}, nil)
		ok := r.Status == 401 || r.Code == 1006
		record(t, verdict{
			ID: "B4", Level: "P0", Title: "未认证请求必须被网关拒绝",
			Pass: ok,
			Expected: "HTTP 401 或 body.code=1006",
			Actual:  summarize(r),
			Note:    "这是唯一能保留业务码的路径：1006 由网关本地生成，不经过 gRPC 边界",
		})
	})

	// -----------------------------------------------------------------
	// B5 越权
	// -----------------------------------------------------------------
	t.Run("B5_Envoy路径越权：body 里的 user_id 未被校验", func(t *testing.T) {
		victim, err := e.createFriendPair(t, fsDB)
		if err != nil {
			t.Fatalf("构造受害者好友对失败: %v", err)
		}
		attackerID, _ := e.registerAndLogin(t, fmt.Sprintf("e2e_atk_%d", time.Now().UnixNano()%1e9), "Password123!")

		// 攻击者 C 在请求体里写受害者 A 的 user_id
		r := e.blockViaEnvoy(t, victim.AUserID, victim.BUserID, map[string]string{
			"x-user-id": attackerID,
		})
		ab, _ := friendshipRow(t, fsDB, victim.AUserID, victim.BUserID)
		ba, _ := friendshipRow(t, fsDB, victim.BUserID, victim.AUserID)
		escalated := ab.Status == 2 || ba.Status == 1

		record(t, verdict{
			ID: "B5", Level: "P0", Title: "越权：Envoy 路径未校验请求体里的 user_id",
			Pass: !escalated,
			Expected: "攻击者冒充他人 user_id 的拉黑请求必须被拒绝，受害者关系保持 NORMAL(0/0)",
			Actual: fmt.Sprintf("受害者关系 A.status=%d B.status=%d（被拉黑=%v）；响应 %s",
				ab.Status, ba.Status, escalated, summarize(r)),
			Note: "根因：configs/envoy.yaml 无任何认证过滤器；grpc_server.go 优先信任 req.UserId（来自请求体）",
		})
	})

	// -----------------------------------------------------------------
	// B6 错误 HTTP 动词
	// -----------------------------------------------------------------
	t.Run("B6_错误HTTP动词应被拒绝且零副作用", func(t *testing.T) {
		outboxBefore := outboxCount(t, fsDB, base.AUserID)
		rEnvoy := e.call(t, "POST", e.EnvoyURL+"/friends/block", "", map[string]any{
			"user_id": base.AUserID, "friend_id": base.BUserID,
		}, map[string]string{"x-user-id": base.AUserID})
		rGw := e.call(t, "POST", e.GatewayURL+"/friends/block", base.AToken, map[string]any{
			"friend_id": base.BUserID,
		}, nil)
		outboxAfter := outboxCount(t, fsDB, base.AUserID)
		ok := rEnvoy.Status != 200 && rGw.Status != 200 && outboxAfter == outboxBefore
		record(t, verdict{
			ID: "B6", Level: "P1", Title: "错误 HTTP 动词（POST）必须被拒绝且零副作用",
			Pass: ok,
			Expected: "两条路径均非 200；outbox 不增加",
			Actual:  fmt.Sprintf("Envoy: %s；Gateway: %s；outbox %d->%d", summarize(rEnvoy), summarize(rGw), outboxBefore, outboxAfter),
		})
	})

	// -----------------------------------------------------------------
	// B7 两条路径的错误契约
	// -----------------------------------------------------------------
	t.Run("B7_两条路径的业务错误码都不可判别", func(t *testing.T) {
		xID, _ := e.registerAndLogin(t, fmt.Sprintf("e2e_cx_%d", time.Now().UnixNano()%1e9), "Password123!")
		yID, _ := e.registerAndLogin(t, fmt.Sprintf("e2e_cy_%d", time.Now().UnixNano()%1e9), "Password123!")

		rEnvoy := e.blockViaEnvoy(t, xID, yID, nil)
		rGw := e.blockViaGateway(t, e.tokenOf(t, xID), yID)

		envoyHasBizCode := rEnvoy.Code == 1001 || rEnvoy.Code == 1007 || rEnvoy.Code == 1005
		gwHasBizCode := rGw.Code == 1001 || rGw.Code == 1007 || rGw.Code == 1005
		// 「可判别」的门槛：客户端能从响应里区分出业务原因
		envoyDistinguishable := envoyHasBizCode && rEnvoy.Code != 1005
		gwDistinguishable := gwHasBizCode && rGw.Code != 1005

		record(t, verdict{
			ID: "B7", Level: "P1", Title: "业务错误码在 gRPC 边界丢失：两条路径都只能给出「服务繁忙」",
			Pass: envoyDistinguishable && gwDistinguishable,
			Expected: "客户端能拿到可判别的业务码（1001/1007），而不是与系统故障混同的 1005",
			Actual: fmt.Sprintf("Envoy: %s（可判别=%v）；Gateway: %s（可判别=%v）",
				summarize(rEnvoy), envoyDistinguishable, summarize(rGw), gwDistinguishable),
			Note: "根因：errorx.CodeError 未实现 GRPCStatus()，grpc-go 一律映射为 codes.Unknown(2)；" +
				"Gin 网关侧 errors.As(*status.Error) 失败 → 落到 1005「服务繁忙」",
		})
		breakerCooldown(t, "B7")
	})

	// -----------------------------------------------------------------
	// B8 熔断器
	// -----------------------------------------------------------------
	t.Run("B8_连续业务失败会触发客户端熔断器", func(t *testing.T) {
		xID, _ := e.registerAndLogin(t, fmt.Sprintf("e2e_bx_%d", time.Now().UnixNano()%1e9), "Password123!")
		yID, _ := e.registerAndLogin(t, fmt.Sprintf("e2e_by_%d", time.Now().UnixNano()%1e9), "Password123!")

		var last *apiResp
		var opened *apiResp
		for i := 0; i < 8; i++ {
			r := e.blockViaEnvoy(t, xID, yID, nil)
			last = r
			if strings.Contains(strings.ToLower(r.Msg+r.Message), "circuit breaker") {
				opened = r
				break
			}
		}
		ok := opened != nil
		record(t, verdict{
			ID: "B8", Level: "P1", Title: "连续业务失败会打开 gRPC 客户端熔断器，后续调用快速失败",
			Pass: ok,
			Expected: "连续失败触发 gobreaker（5 次连续失败即打开），出现 circuit breaker open / Unavailable(503)",
			Actual:  fmt.Sprintf("最后一次: %s；出现熔断=%v", summarize(last), ok),
			Note: "pkg/interceptor/breaker.go 的 IsSuccessful 把 codes.Unknown 判为失败，"+
				"而所有业务错误都退化成 codes.Unknown → 业务错误会误伤熔断器",
		})
		breakerCooldown(t, "B8")
	})

	// -----------------------------------------------------------------
	// F1 MySQL 延迟 30ms
	// -----------------------------------------------------------------
	t.Run("F1_MySQL延迟30ms应成功", func(t *testing.T) {
		p := e.mustCreateFriendPair(t, fsDB)
		e.toxiAddToxic(t, "mysql_proxy", "f1_latency", "latency", "downstream", map[string]any{
			"latency": 30, "jitter": 5,
		})
		defer e.toxiClear(t, "mysql_proxy")

		start := time.Now()
		r := e.blockViaEnvoy(t, p.AUserID, p.BUserID, nil)
		elapsed := time.Since(start)
		state, stateOK := assertPairState(t, fsDB, p.AUserID, p.BUserID, 2, 1, 2)
		ok := r.Status == 200 && stateOK
		record(t, verdict{
			ID: "F1", Level: "P0", Title: "MySQL 注入 30ms 延迟时拉黑仍应成功",
			Pass: ok,
			Expected: "HTTP 200；A.status=2 B.status=1 version=2/2；耗时远小于 2s 服务端超时",
			Actual:  fmt.Sprintf("%s；耗时=%v；%s", summarize(r), elapsed, state),
		})
	})

	// -----------------------------------------------------------------
	// F2 MySQL 延迟 300ms
	// -----------------------------------------------------------------
	t.Run("F2_MySQL延迟300ms必须原子回滚", func(t *testing.T) {
		p := e.mustCreateFriendPair(t, fsDB)
		outboxBefore := outboxCount(t, fsDB, p.AUserID)
		e.toxiAddToxic(t, "mysql_proxy", "f2_latency", "latency", "downstream", map[string]any{
			"latency": 300, "jitter": 50,
		})
		defer e.toxiClear(t, "mysql_proxy")

		start := time.Now()
		r := e.blockViaEnvoy(t, p.AUserID, p.BUserID, nil)
		elapsed := time.Since(start)

		// 关键断言：要么全成功（A=2,B=1,v=2），要么全回滚（A=0,B=0,v=1），绝不能一半
		ab, _ := friendshipRow(t, fsDB, p.AUserID, p.BUserID)
		ba, _ := friendshipRow(t, fsDB, p.BUserID, p.AUserID)
		outboxAfter := outboxCount(t, fsDB, p.AUserID)

		allDone := ab.Status == 2 && ba.Status == 1 && ab.Version == 2 && ba.Version == 2 && outboxAfter == outboxBefore+1
		allRollback := ab.Status == 0 && ba.Status == 0 && ab.Version == 1 && ba.Version == 1 && outboxAfter == outboxBefore
		half := !allDone && !allRollback

		record(t, verdict{
			ID: "F2", Level: "P0", Title: "MySQL 注入 300ms 延迟时，拉黑必须全成功或全回滚（禁止半拉黑）",
			Pass: !half,
			Expected: "7 个 MySQL 往返 × 300ms ≈ 2.1s > Kratos 2s 服务端超时 → 预期整体回滚；" +
				"无论如何都不允许出现「A 已拉黑但 B 没变」的半拉黑",
			Actual: fmt.Sprintf("%s；耗时=%v；A(status=%d,v=%d) B(status=%d,v=%d)；outbox %d->%d；半拉黑=%v",
				summarize(r), elapsed, ab.Status, ab.Version, ba.Status, ba.Version, outboxBefore, outboxAfter, half),
			Note: "全成功/全回滚二选一都是正确的；只要不是半拉黑，P0 原子性即通过",
		})
		breakerCooldown(t, "F2")
	})

	// -----------------------------------------------------------------
	// F3 MySQL 断连
	// -----------------------------------------------------------------
	t.Run("F3_MySQL断连必须原子回滚", func(t *testing.T) {
		p := e.mustCreateFriendPair(t, fsDB)
		outboxBefore := outboxCount(t, fsDB, p.AUserID)
		e.toxiAddToxic(t, "mysql_proxy", "f3_reset", "reset_peer", "downstream", map[string]any{
			"timeout": 100,
		})
		defer e.toxiClear(t, "mysql_proxy")

		r := e.blockViaEnvoy(t, p.AUserID, p.BUserID, nil)
		ab, _ := friendshipRow(t, fsDB, p.AUserID, p.BUserID)
		ba, _ := friendshipRow(t, fsDB, p.BUserID, p.AUserID)
		outboxAfter := outboxCount(t, fsDB, p.AUserID)

		allDone := ab.Status == 2 && ba.Status == 1 && outboxAfter == outboxBefore+1
		allRollback := ab.Status == 0 && ba.Status == 0 && ab.Version == 1 && ba.Version == 1 && outboxAfter == outboxBefore
		half := !allDone && !allRollback

		record(t, verdict{
			ID: "F3", Level: "P0", Title: "MySQL 连接被重置时，拉黑必须整体回滚",
			Pass: !half,
			Expected: "请求失败（1005/500）且状态与 outbox 完全回到原值",
			Actual: fmt.Sprintf("%s；A(status=%d,v=%d) B(status=%d,v=%d)；outbox %d->%d；半拉黑=%v",
				summarize(r), ab.Status, ab.Version, ba.Status, ba.Version, outboxBefore, outboxAfter, half),
		})
		breakerCooldown(t, "F3")
	})

	// -----------------------------------------------------------------
	// F4 gRPC 跳延迟
	// -----------------------------------------------------------------
	t.Run("F4_gRPC跳延迟下拉黑至多执行一次", func(t *testing.T) {
		p := e.mustCreateFriendPair(t, fsDB)
		outboxBefore := outboxCount(t, fsDB, p.AUserID)
		e.toxiAddToxic(t, "friendship_grpc", "f4_latency", "latency", "downstream", map[string]any{
			"latency": 2500, "jitter": 100,
		})
		defer e.toxiClear(t, "friendship_grpc")

		start := time.Now()
		r := e.blockViaEnvoy(t, p.AUserID, p.BUserID, nil)
		elapsed := time.Since(start)

		outboxAfter := outboxCount(t, fsDB, p.AUserID)
		ab, _ := friendshipRow(t, fsDB, p.AUserID, p.BUserID)
		ba, _ := friendshipRow(t, fsDB, p.BUserID, p.AUserID)

		// 关键断言：outbox 至多增加 1 条（客户端超时重试不能让写操作执行多次）
		atMostOnce := outboxAfter <= outboxBefore+1
		consistent := (ab.Status == 2 && ba.Status == 1) || (ab.Status == 0 && ba.Status == 0)
		ok := atMostOnce && consistent

		record(t, verdict{
			ID: "F4", Level: "P1", Title: "gRPC 跳注入 2.5s 延迟（超过客户端 2s 超时）时，拉黑至多执行一次",
			Pass: ok,
			Expected: "outbox 至多 +1 条；最终状态自洽（要么全拉黑要么全没拉黑）",
			Actual: fmt.Sprintf("%s；耗时=%v；outbox %d->%d；A(status=%d) B(status=%d)",
				summarize(r), elapsed, outboxBefore, outboxAfter, ab.Status, ba.Status),
			Note: "网关侧 gRPC 客户端 WithMax(3)=最多 3 次尝试，会重放同一个请求",
		})
	})

	// -----------------------------------------------------------------
	// F5 Redis 断连
	// -----------------------------------------------------------------
	t.Run("F5_Redis断连时拉黑主流程仍应成功", func(t *testing.T) {
		p := e.mustCreateFriendPair(t, fsDB)
		e.toxiAddToxic(t, "redis_proxy", "f5_reset", "reset_peer", "downstream", map[string]any{
			"timeout": 100,
		})
		defer e.toxiClear(t, "redis_proxy")

		// 用 Envoy 路径（不依赖 Redis 的 JWT 校验）
		r := e.blockViaEnvoy(t, p.AUserID, p.BUserID, nil)
		state, stateOK := assertPairState(t, fsDB, p.AUserID, p.BUserID, 2, 1, 2)

		// 对照：Gin 网关路径依赖 Redis 做 SSO token 校验，Redis 挂掉时应被拒绝
		rGw := e.blockViaGateway(t, e.tokenOf(t, p.AUserID), p.BUserID)

		record(t, verdict{
			ID: "F5", Level: "P1", Title: "Redis 断连时：拉黑主流程不依赖 Redis（Envoy 路径仍成功）",
			Pass: stateOK,
			Expected: "Envoy 路径拉黑成功（Redis 不在拉黑关键路径）；Gin 网关路径因 SSO 校验依赖 Redis 而失败",
			Actual: fmt.Sprintf("Envoy: %s；%s；Gateway: %s", summarize(r), state, summarize(rGw)),
			Note: "附带发现：Redis 不可用会让所有走网关的已认证接口直接 401，" +
				"同时 message_service 的 pushNotice 会静默丢弃 BLACK 通知（无日志无指标）",
		})
	})

	// -----------------------------------------------------------------
	// A1 ABA 乱序
	// -----------------------------------------------------------------
	t.Run("A1_ABA乱序：API层没有版本号，陈旧拉黑包无法被拦截", func(t *testing.T) {
		p := e.mustCreateFriendPair(t, fsDB)

		// 1) 正常拉黑
		r1 := e.blockViaEnvoy(t, p.AUserID, p.BUserID, nil)
		ab1, _ := friendshipRow(t, fsDB, p.AUserID, p.BUserID)

		// 2) 取消拉黑（Envoy 的 UnblackFriend 无 body 规则，参数走 query）
		r2 := e.call(t, "DELETE", e.EnvoyURL+"/friends/block?"+url.Values{
			"user_id":   {p.AUserID},
			"friend_id": {p.BUserID},
		}.Encode(), "", nil, map[string]string{"x-user-id": p.AUserID})
		ab2, _ := friendshipRow(t, fsDB, p.AUserID, p.BUserID)

		// 3) 迟到的旧拉黑包（API 层无法携带版本号，服务端无从区分新旧）
		r3 := e.blockViaEnvoy(t, p.AUserID, p.BUserID, nil)
		ab3, _ := friendshipRow(t, fsDB, p.AUserID, p.BUserID)

		// 期望：迟到包被拦截，最终仍是 NORMAL 且版本停在 3
		blocked := ab3.Status == 2
		ok := !blocked && ab3.Status == 0
		record(t, verdict{
			ID: "A1", Level: "P0", Title: "ABA 乱序：迟到���旧拉黑包应被版本 CAS 拦截",
			Pass: ok,
			Expected: "拉黑→解拉黑后，迟到的旧拉黑包必须被拦截；最终 status=0(NORMAL)、version=3",
			Actual: fmt.Sprintf("拉黑后 status=%d v=%d；解拉黑后 status=%d v=%d；迟到包后 status=%d v=%d（被误拉黑=%v）",
				ab1.Status, ab1.Version, ab2.Status, ab2.Version, ab3.Status, ab3.Version, blocked),
			Note: fmt.Sprintf("根因：BlackFriendRequest 没有 version 字段（api/proto/friendship.proto:85），"+
				"grpc_server.go:153 调 BlackFriend(ctx, userId, friendId) 不传 expectedVersion，"+
				"service.go 里的 ABA 防线（CodeConflict 1013）在真实 API 上不可达。"+
				"对照：tests/chaos/chaos_test.go:275 直接调领域层并传 version=1 才能验到拦截。响应 r1=%s r2=%s r3=%s",
				summarize(r1), summarize(r2), summarize(r3)),
		})
	})

	// -----------------------------------------------------------------
	// A2 异步半程
	// -----------------------------------------------------------------
	t.Run("A2_异步半程：Kafka事件与会话删除的一致性", func(t *testing.T) {
		p := e.mustCreateFriendPair(t, fsDB)
		sessUUID := "S" + fmt.Sprintf("%d", time.Now().UnixNano()%1e13)
		insertSession(t, msgDB, sessUUID, p.AUserID, p.BUserID)

		r := e.blockViaEnvoy(t, p.AUserID, p.BUserID, nil)
		cnt := waitKafkaEvents(t, events, "friend_blacked", p.AUserID, 1, 40*time.Second)
		// 等一会儿看有没有重复事件
		time.Sleep(8 * time.Second)
		cnt2 := events.countEvents("friend_blacked", p.AUserID)
		sessLeft, sessGone := waitSessionGone(t, msgDB, p.AUserID, p.BUserID, 40*time.Second)

		exactlyOnce := cnt2 == 1
		ok := r.Status == 200 && exactlyOnce && sessGone
		record(t, verdict{
			ID: "A2", Level: "P0", Title: "异步半程：friend_blacked 事件恰好一条且双方会话被物理删除",
			Pass: ok,
			Expected: "Kafka domain_events 中 friend_blacked 恰好 1 条（不重不漏）；双方会话被物理删除",
			Actual: fmt.Sprintf("%s；Kafka 条数=%d（观测后复查=%d）；会话剩余=%d 行(已删=%v)",
				summarize(r), cnt, cnt2, sessLeft, sessGone),
			Note: "CDC 遇 Kafka 发布失败会直接死掉且不重启（cmd/cdc_service/main.go:98），" +
				"且 Kafka delivery timeout 只有 1s（[kafkaConfig] timeout=1）",
		})
	})

	printSummary(t)
}

func noteIf(cond bool, s string) string {
	if cond {
		return "WS 报文: " + s
	}
	return ""
}
