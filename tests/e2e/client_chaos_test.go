package e2e_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// Test_Client_Ingress_Chaos 客户端入口弱网与故障注入矩阵
//
// 覆盖场景：
//   C1 - 客户端入口网络延迟与抖动 (Latency & Jitter)
//   C2 - 客户端长连接突然切断与无缝增量续传 (Abrupt Disconnect & Catch-Up Sync)
//   C3 - 客户端网络黑洞/完全断网超时转失败 (Blackhole & Timeout)
//   C4 - 重复消息与 ACK 丢失幂等性验证 (Deduplication)
func Test_Client_Ingress_Chaos(t *testing.T) {
	e := loadEnv(t)
	fsDB := e.openDB(t, "chat_friendship")
	msgDB := e.openDB(t, "chat_message")

	// 清理客户端代理上的旧 toxic，确保基线纯净
	e.toxiClear(t, "envoy_client")
	e.toxiClear(t, "ws_client")
	e.toxiSetProxyEnabled(t, "ws_client", true)
	e.toxiSetProxyEnabled(t, "envoy_client", true)
	defer func() {
		e.toxiClear(t, "envoy_client")
		e.toxiClear(t, "ws_client")
		e.toxiSetProxyEnabled(t, "ws_client", true)
		e.toxiSetProxyEnabled(t, "envoy_client", true)
	}()

	t.Log("[前置] 准备测试好友对...")
	pair := e.mustCreateFriendPair(t, fsDB)
	userA, tokenA := pair.AUserID, pair.AToken
	userB, tokenB := pair.BUserID, pair.BToken

	// -------------------------------------------------------------
	// C1: 客户端入口延迟与抖动 (Latency & Jitter)
	// -------------------------------------------------------------
	t.Run("C1_Client_Ingress_Latency_Jitter", func(t *testing.T) {
		// 向 envoy_client (19080) 注入 350ms 延迟 + 50ms 抖动
		e.toxiAddToxic(t, "envoy_client", "c1_http_latency", "latency", "downstream", map[string]any{
			"latency": 350,
			"jitter":  50,
		})
		defer e.toxiRemoveToxic(t, "envoy_client", "c1_http_latency")

		// 客户端走代理调用 HTTP 获取好友列表（幂等 GET）
		start := time.Now()
		reqURL := fmt.Sprintf("%s/friends?user_id=%s", e.EnvoyClientProxyURL, userA)
		status, raw, err := httpDo("GET", reqURL, tokenA, nil, map[string]string{
			"x-user-id": userA,
		})
		elapsed := time.Since(start)
		if err != nil || status != 200 {
			t.Fatalf("C1 走弱网代理 GET /friends 失败 (status=%d, err=%v): %s", status, err, string(raw))
		}
		if elapsed < 250*time.Millisecond {
			t.Errorf("期望延迟生效 (>=250ms)，实际耗时 %v", elapsed)
		}
		t.Logf("C1: 客户端经 350ms 延迟代理请求成功，耗时 %v", elapsed)

		// 向 ws_client (18000) 注入延迟，验证 WS 连接在延迟下依然能成功建立握手
		e.toxiAddToxic(t, "ws_client", "c1_ws_latency", "latency", "downstream", map[string]any{
			"latency": 250,
			"jitter":  30,
		})
		defer e.toxiRemoveToxic(t, "ws_client", "c1_ws_latency")

		wsClientA, err := dialWSErr(e.ConnectorClientProxyURL, tokenA, 10*time.Second)
		if err != nil {
			t.Fatalf("C1 走弱网代理 dialWS 失败: %v", err)
		}
		defer wsClientA.close()

		// 客户端发送心跳 ping
		if err := wsClientA.writeJSON(map[string]any{"action": "ping"}); err != nil {
			t.Fatalf("C1 发送 ping 失败: %v", err)
		}
		pong, ok := wsClientA.waitMessage(t, 5*time.Second, func(b []byte) bool {
			return string(b) == `{"action":"pong"}`
		})
		if !ok {
			t.Fatalf("C1 在延迟网络下未收到 pong")
		}
		t.Logf("C1: 客户端 WS 握手并成功在延迟网络下完成心跳交互: %s", string(pong))
	})

	// -------------------------------------------------------------
	// C2: 客户端长连接突然切断与无缝增量续传 (Abrupt Disconnect & Catch-Up Sync)
	// -------------------------------------------------------------
	t.Run("C2_Abrupt_Disconnect_And_CatchUp_Sync", func(t *testing.T) {
		// 1. 用户 A 和用户 B 通过客户端代理接入 WS
		wsA := dialWS(t, e.ConnectorClientProxyURL, tokenA)
		defer wsA.close()
		e.waitConnectorRoute(t, userA, 10*time.Second)

		wsB := dialWS(t, e.ConnectorClientProxyURL, tokenB)
		e.waitConnectorRoute(t, userB, 10*time.Second)

		// 2. A 发送第 1 条消息，B 正常在线接收
		tag := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
		msg1ID := fmt.Sprintf("c2-msg-1-%s", tag)
		err := wsA.writeJSON(map[string]any{
			"receive_id":    userB,
			"content":       "c2 online msg 1",
			"type":          0,
			"client_msg_id": msg1ID,
		})
		if err != nil {
			t.Fatalf("A 发送消息 1 失败: %v", err)
		}
		// A 等待 ACK
		_, okA := wsA.waitMessage(t, 5*time.Second, func(b []byte) bool {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err == nil {
				return m["client_msg_id"] == msg1ID
			}
			return false
		})
		if !okA {
			t.Fatalf("A 未收到消息 1 的 ACK")
		}

		// B 等待收到下发的消息 1
		var firstSeq int64
		msg1Bytes, ok := wsB.waitMessage(t, 5*time.Second, func(b []byte) bool {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err == nil {
				if content, ok := m["content"].(string); ok && content == "c2 online msg 1" {
					if s, ok := m["seq_id"].(float64); ok {
						firstSeq = int64(s)
					}
					return true
				}
			}
			return false
		})
		if !ok {
			t.Fatalf("B 未在预期时间内收到在线消息 1")
		}
		t.Logf("C2: B 收到在线消息 1 (seq=%d): %s", firstSeq, string(msg1Bytes))

		// 3. 模拟用户 B 所在网络突然闪断 (Toxiproxy 注入 reset_peer 切断 ws_client)
		e.toxiAddToxic(t, "ws_client", "c2_reset", "reset_peer", "downstream", map[string]any{
			"timeout": 0,
		})
		time.Sleep(500 * time.Millisecond)
		// 移除 reset_peer 模拟网络恢复
		e.toxiRemoveToxic(t, "ws_client", "c2_reset")
		wsB.close() // B 本地连接切断

		// 4. 在 B 掉线期间，A 连续发送消息 2 和消息 3
		msg2ID := fmt.Sprintf("c2-msg-2-%s", tag)
		msg3ID := fmt.Sprintf("c2-msg-3-%s", tag)
		if err := wsA.writeJSON(map[string]any{
			"receive_id": userB, "content": "c2 offline msg 2", "type": 0, "client_msg_id": msg2ID,
		}); err != nil {
			t.Fatalf("A 发送消息 2 失败: %v", err)
		}
		_, _ = wsA.waitMessage(t, 5*time.Second, func(b []byte) bool {
			var m map[string]any
			return json.Unmarshal(b, &m) == nil && m["client_msg_id"] == msg2ID
		})

		if err := wsA.writeJSON(map[string]any{
			"receive_id": userB, "content": "c2 offline msg 3", "type": 0, "client_msg_id": msg3ID,
		}); err != nil {
			t.Fatalf("A 发送消息 3 失败: %v", err)
		}
		_, _ = wsA.waitMessage(t, 5*time.Second, func(b []byte) bool {
			var m map[string]any
			return json.Unmarshal(b, &m) == nil && m["client_msg_id"] == msg3ID
		})

		// 5. 用户 B 触发客户端重连（建立新 WS 连接并带上历史已确认 firstSeq 发送 sync 拉取未读）
		wsBReconnected := dialWS(t, e.ConnectorClientProxyURL, tokenB)
		defer wsBReconnected.close()
		e.waitConnectorRoute(t, userB, 10*time.Second)

		syncCmd := map[string]any{
			"action":    "sync",
			"target_id": userA,
			"after_seq": firstSeq,
			"limit":     10,
		}
		if err := wsBReconnected.writeJSON(syncCmd); err != nil {
			t.Fatalf("B 重连后发送 sync 指令失败: %v", err)
		}

		// 等待 sync_resp 响应并断言消息 2 和 3 全部到位
		respMsg, ok := wsBReconnected.waitMessage(t, 10*time.Second, func(b []byte) bool {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err == nil {
				return m["type"] == "sync_resp"
			}
			return false
		})
		if !ok {
			t.Fatalf("B 重连后未在超时时间内收到 WS sync_resp 响应")
		}

		var wsResp struct {
			Type     string            `json:"type"`
			TargetId string            `json:"target_id"`
			Messages []syncMessageItem `json:"messages"`
			HasMore  bool              `json:"has_more"`
		}
		if err := json.Unmarshal(respMsg, &wsResp); err != nil {
			t.Fatalf("解析 WS sync_resp 失败: %v", err)
		}
		if len(wsResp.Messages) < 2 {
			t.Fatalf("期望增量拉取至少 2 条断网消息，实际只有 %d 条: %s", len(wsResp.Messages), string(respMsg))
		}
		t.Logf("C2: B 断网重连增量补齐成功，拉取到断网期间消息: %s", string(respMsg))
	})

	// -------------------------------------------------------------
	// C3: 客户端网络黑洞/完全断网超时转失败 (Blackhole & Timeout)
	// -------------------------------------------------------------
	t.Run("C3_Network_Blackhole_Timeout", func(t *testing.T) {
		wsA := dialWS(t, e.ConnectorClientProxyURL, tokenA)
		defer wsA.close()
		e.waitConnectorRoute(t, userA, 10*time.Second)

		// 注入完全阻断（禁用 ws_client 代理通道）
		e.toxiSetProxyEnabled(t, "ws_client", false)
		defer e.toxiSetProxyEnabled(t, "ws_client", true)

		tag := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
		blackholeMsgID := fmt.Sprintf("c3-blackhole-%s", tag)

		// 尝试写出消息，此时连接在底层因黑洞无法送达服务端
		_ = wsA.writeJSON(map[string]any{
			"receive_id":    userB,
			"content":       "c3 blackhole msg",
			"type":          0,
			"client_msg_id": blackholeMsgID,
		})

		// 客户端等待 ACK：断言在超时时间内绝不会收到 ACK
		_, ackReceived := wsA.waitMessage(t, 2*time.Second, func(b []byte) bool {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err == nil {
				if cid, ok := m["client_msg_id"].(string); ok && cid == blackholeMsgID {
					return true
				}
			}
			return false
		})
		if ackReceived {
			t.Fatalf("C3 网络黑洞下不应收到任何 ACK")
		}
		t.Log("C3: 客户端在黑洞下未收到 ACK，断言超时失败逻辑正确")

		// 恢复代理
		e.toxiSetProxyEnabled(t, "ws_client", true)
	})

	// -------------------------------------------------------------
	// C4: 重复消息与 ACK 丢失幂等性验证 (Deduplication)
	// -------------------------------------------------------------
	t.Run("C4_Message_Deduplication", func(t *testing.T) {
		wsA := dialWS(t, e.ConnectorClientProxyURL, tokenA)
		defer wsA.close()
		e.waitConnectorRoute(t, userA, 10*time.Second)

		tag := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
		dupClientMsgID := fmt.Sprintf("c4-dup-%s", tag)

		payload := map[string]any{
			"receive_id":    userB,
			"content":       "c4 unique message content",
			"type":          0,
			"client_msg_id": dupClientMsgID,
		}

		// 第 1 次发送
		if err := wsA.writeJSON(payload); err != nil {
			t.Fatalf("第一次发送消息失败: %v", err)
		}
		// 等待第 1 次 ACK
		_, ok1 := wsA.waitMessage(t, 5*time.Second, func(b []byte) bool {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err == nil {
				if cid, ok := m["client_msg_id"].(string); ok && cid == dupClientMsgID {
					return true
				}
			}
			return false
		})
		if !ok1 {
			t.Fatalf("第一次发送未收到 ACK")
		}

		// 模拟客户端网络卡顿误判或 ACK 丢失，使用同一 client_msg_id 重发完全相同的消息
		if err := wsA.writeJSON(payload); err != nil {
			t.Fatalf("重发消息失败: %v", err)
		}

		// 等待一段时间让消息流转写入完成
		time.Sleep(2 * time.Second)

		// 数据库断言：通过 client_msg_id 查询数据库，必须严格只有 1 条记录！
		var count int64
		err := msgDB.Raw("SELECT COUNT(*) FROM message WHERE client_msg_id = ?", dupClientMsgID).Scan(&count).Error
		if err != nil {
			t.Fatalf("查询数据库 message 表失败: %v", err)
		}
		if count != 1 {
			t.Fatalf("C4 幂等防重失败: client_msg_id=%s 在数据库中存在 %d 条记录 (预期严格为 1 条)", dupClientMsgID, count)
		}
		t.Logf("C4: 重复发送验证通过，client_msg_id=%s 在持久层严格去重保持 1 条", dupClientMsgID)
	})
}
