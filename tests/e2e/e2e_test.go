package e2e_test

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// 结论收集
// ---------------------------------------------------------------------------

type verdict struct {
	ID       string
	Level    string // P0 / P1 / P2
	Title    string
	Pass     bool
	Expected string
	Actual   string
	Note     string
}

var (
	resultMu sync.Mutex
	results  []verdict
)

func record(t *testing.T, v verdict) {
	resultMu.Lock()
	defer resultMu.Unlock()
	results = append(results, v)
	status := "FAIL"
	if v.Pass {
		status = "PASS"
	}
	t.Logf("[%s] %s (%s) %s | 期望: %s | 实际: %s %s",
		status, v.ID, v.Level, v.Title, v.Expected, v.Actual, v.Note)
}

func check(t *testing.T, cond bool, format string, args ...any) bool {
	t.Helper()
	if !cond {
		t.Errorf(format, args...)
	}
	return cond
}

func printSummary(t *testing.T) {
	t.Helper()
	resultMu.Lock()
	defer resultMu.Unlock()
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	var p0Fail, p1Fail, p2Fail int
	t.Log("\n================ 场景矩阵结论汇总 ================")
	t.Logf("%-6s %-4s %-6s %s", "ID", "级别", "结果", "场景")
	for _, v := range results {
		res := "PASS"
		switch {
		case v.Pass:
		case v.Level == "P0":
			res = "FAIL"
			p0Fail++
		case v.Level == "P1":
			res = "FAIL"
			p1Fail++
		default:
			res = "FAIL"
			p2Fail++
		}
		t.Logf("%-6s %-4s %-6s %s", v.ID, v.Level, res, v.Title)
		t.Logf("       期望: %s", v.Expected)
		t.Logf("       实际: %s", v.Actual)
		if v.Note != "" {
			t.Logf("       备注: %s", v.Note)
		}
	}
	t.Log("--------------------------------------------------")
	t.Logf("P0 失败 %d 项 / P1 失败 %d 项 / P2 失败 %d 项", p0Fail, p1Fail, p2Fail)
	if p0Fail > 0 {
		t.Logf("结论：存在 P0 级缺陷（正确性/安全/数据一致性）")
	} else {
		t.Logf("结论：P0 全部通过 —— 弱网下拉黑接口的原子性与幂等性正确")
	}
	t.Log("==================================================")
}

// ---------------------------------------------------------------------------
// 夹具：真实链路造好友
// ---------------------------------------------------------------------------

type friendPair struct {
	AUserID string
	BUserID string
	AToken  string
	BToken  string
	AName   string
	BName   string
}

// registerAndLogin 走 Gin 网关注册并登录，返回 uuid 与 access_token
func (e *testEnv) registerAndLogin(t *testing.T, username, password string) (string, string) {
	t.Helper()
	reg := e.call(t, "POST", e.GatewayURL+"/auth/register", "", map[string]any{
		"username": username, "password": password, "client_token": "e2e-" + username,
	}, nil)
	if reg.Code != 1000 {
		t.Fatalf("注册 %s 失败: %s", username, reg.Summary())
	}
	login := e.call(t, "POST", e.GatewayURL+"/auth/login", "", map[string]any{
		"username": username, "password": password,
	}, nil)
	if login.Code != 1000 {
		t.Fatalf("登录 %s 失败: %s", username, login.Summary())
	}
	var data struct {
		Uuid        string `json:"uuid"`
		AccessToken string `json:"access_token"`
	}
	if err := unmarshalData(login.Data, &data); err != nil {
		t.Fatalf("解析登录响应失败: %v (raw=%s)", err, string(login.Raw))
	}
	if data.Uuid == "" || data.AccessToken == "" {
		t.Fatalf("登录响应缺少 uuid/access_token: %s", string(login.Raw))
	}
	e.putToken(data.Uuid, data.AccessToken)
	return data.Uuid, data.AccessToken
}

// createFriendPair 通过「注册 → 申请 → 审批」真实链路造一对互为好友的用户。
// 好友关系是异步建立的（apply_service 写 outbox → CDC → Kafka → friendship_service 消费），
// 因此必须轮询等待。
//
// 返回 error 而不是 t.Fatalf：前置链路本身可能坏（例如 apply_service 缺 grpc_client.Init），
// 这时不应该让整个套件崩掉，而应该记为「前置链路缺陷」并继续跑其它场景。
func (e *testEnv) createFriendPair(t *testing.T, db *gorm.DB) (*friendPair, error) {
	t.Helper()
	tag := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	aname := "e2ea_" + tag
	bname := "e2eb_" + tag
	aID, aTok := e.registerAndLogin(t, aname, "Password123!")
	bID, bTok := e.registerAndLogin(t, bname, "Password123!")

	apply := e.call(t, "POST", e.GatewayURL+"/friends/apply", aTok, map[string]any{
		"friend_id": bID, "message": "e2e", "client_token": "e2e-apply-" + tag,
	}, nil)
	if apply.Code != 1000 {
		return nil, fmt.Errorf("发起好友申请失败: %s", apply.Summary())
	}
	approve := e.call(t, "POST", e.GatewayURL+"/friends/applies/approve", bTok, map[string]any{
		"applicant_id": aID,
	}, nil)
	if approve.Code != 1000 {
		return nil, fmt.Errorf("审批好友申请失败: %s", approve.Summary())
	}

	p := &friendPair{AUserID: aID, BUserID: bID, AToken: aTok, BToken: bTok, AName: aname, BName: bname}
	if err := e.waitFriendshipReady(db, aID, bID); err != nil {
		return nil, err
	}
	return p, nil
}

// mustCreateFriendPair 构造好友对，失败则终止当前子测试
func (e *testEnv) mustCreateFriendPair(t *testing.T, db *gorm.DB) *friendPair {
	t.Helper()
	p, err := e.createFriendPair(t, db)
	if err != nil {
		t.Fatalf("构造好友对失败: %v", err)
	}
	return p
}

// waitFriendshipReady 轮询等待双向 NORMAL 好友关系落库
func (e *testEnv) waitFriendshipReady(db *gorm.DB, a, b string) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		ab, ok1 := friendshipRowNoFatal(db, a, b)
		ba, ok2 := friendshipRowNoFatal(db, b, a)
		if ok1 && ok2 && ab.Status == 0 && ba.Status == 0 {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("等待好友关系建立超时（A=%s B=%s）—— 检查 CDC 是否在跑、outbox 表是否被订阅", a, b)
}

// ---------------------------------------------------------------------------
// 断言辅助
// ---------------------------------------------------------------------------

// assertPairState 断言双向状态与版本号
func assertPairState(t *testing.T, db *gorm.DB, a, b string, wantAStatus, wantBStatus int8, wantVersion int64) (string, bool) {
	t.Helper()
	ab, ok1 := friendshipRow(t, db, a, b)
	ba, ok2 := friendshipRow(t, db, b, a)
	actual := fmt.Sprintf("A(status=%d,ver=%d,存在=%v) B(status=%d,ver=%d,存在=%v)",
		ab.Status, ab.Version, ok1, ba.Status, ba.Version, ok2)
	ok := true
	ok = check(t, ok1, "A→B 好友关系不存在") && ok
	ok = check(t, ok2, "B→A 好友关系不存在") && ok
	if ok1 && ok2 {
		ok = check(t, ab.Status == wantAStatus, "A→B 状态期望 %d，实际 %d", wantAStatus, ab.Status) && ok
		ok = check(t, ba.Status == wantBStatus, "B→A 状态期望 %d，实际 %d", wantBStatus, ba.Status) && ok
		ok = check(t, ab.Version == wantVersion, "A→B 版本期望 %d，实际 %d", wantVersion, ab.Version) && ok
		ok = check(t, ba.Version == wantVersion, "B→A 版本期望 %d，实际 %d", wantVersion, ba.Version) && ok
	}
	return actual, ok
}

// waitSessionGone 等待异步半程把会话删掉
func waitSessionGone(t *testing.T, db *gorm.DB, a, b string, timeout time.Duration) (int64, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var n int64 = -1
	for time.Now().Before(deadline) {
		n = sessionCount(t, db, a, b)
		if n == 0 {
			return 0, true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return n, false
}

// waitKafkaEvents 等待 Kafka 中出现期望数量的事件
func waitKafkaEvents(t *testing.T, r *eventReader, eventType, needle string, want int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	n := 0
	for time.Now().Before(deadline) {
		n = r.countEvents(eventType, needle)
		if n >= want {
			return n
		}
		time.Sleep(500 * time.Millisecond)
	}
	return n
}

// blockViaEnvoy 通过 Envoy 生产边缘路径拉黑
func (e *testEnv) blockViaEnvoy(t *testing.T, userID, friendID string, extra map[string]string) *apiResp {
	t.Helper()
	h := map[string]string{}
	for k, v := range extra {
		h[k] = v
	}
	if _, ok := h["x-user-id"]; !ok {
		h["x-user-id"] = userID
	}
	return e.call(t, "PUT", e.EnvoyURL+"/friends/block", "", map[string]any{
		"user_id": userID, "friend_id": friendID,
	}, h)
}

// blockViaGateway 通过 Gin 网关路径拉黑（JWT 认证）
func (e *testEnv) blockViaGateway(t *testing.T, token, friendID string) *apiResp {
	t.Helper()
	return e.call(t, "PUT", e.GatewayURL+"/friends/block", token, map[string]any{
		"friend_id": friendID,
	}, nil)
}

func summarize(r *apiResp) string {
	if r == nil {
		return "<nil>"
	}
	s := fmt.Sprintf("HTTP=%d", r.Status)
	if r.Code != 0 {
		s += fmt.Sprintf(" code=%d", r.Code)
	}
	msg := r.Msg
	if msg == "" {
		msg = r.Message
	}
	if msg != "" {
		s += fmt.Sprintf(" msg=%q", msg)
	}
	return s
}
