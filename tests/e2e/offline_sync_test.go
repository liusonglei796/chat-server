package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"
)

type syncMessageItem struct {
	SendId    string `json:"send_id"`
	ReceiveId string `json:"receive_id"`
	Content   string `json:"content"`
	SeqId     any    `json:"seq_id"`
	CreatedAt string `json:"created_at"`

	SendIdCamel    string `json:"sendId"`
	ReceiveIdCamel string `json:"receiveId"`
	SeqIdCamel     any    `json:"seqId"`
	CreatedAtCamel string `json:"createdAt"`
}

func parseSeq(v any) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	case json.Number:
		n, _ := val.Int64()
		return n
	default:
		return 0
	}
}

func (s *syncMessageItem) GetSeqId() int64 {
	if s.SeqId != nil {
		if n := parseSeq(s.SeqId); n != 0 {
			return n
		}
	}
	return parseSeq(s.SeqIdCamel)
}

type syncMessagesResponse struct {
	List            []syncMessageItem `json:"list"`
	NextCursor      string            `json:"next_cursor"`
	NextCursorCamel string            `json:"nextCursor"`
	HasMore         bool              `json:"has_more"`
	HasMoreCamel    bool              `json:"hasMore"`
}

func (s *syncMessagesResponse) GetNextCursor() string {
	if s.NextCursor != "" {
		return s.NextCursor
	}
	return s.NextCursorCamel
}

func httpDo(method, urlStr, token string, body any, extra map[string]string) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, urlStr, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

func (e *testEnv) registerAndLoginViaEnvoy(t *testing.T, username, password string) (string, string) {
	t.Helper()
	var statusCode int
	var regRaw []byte
	var err error
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		statusCode, regRaw, err = httpDo("POST", e.EnvoyURL+"/auth/register", "", map[string]any{
			"username": username, "password": password, "client_token": "e2e-" + username,
		}, nil)
		if err == nil && statusCode == 200 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil || statusCode != 200 {
		t.Fatalf("Envoy 注册 %s 失败 (HTTP %d, err=%v): %s", username, statusCode, err, string(regRaw))
	}

	var loginRaw []byte
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		statusCode, loginRaw, err = httpDo("POST", e.EnvoyURL+"/auth/login", "", map[string]any{
			"username": username, "password": password,
		}, nil)
		if err == nil && statusCode == 200 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil || statusCode != 200 {
		t.Fatalf("Envoy 登录 %s 失败 (HTTP %d, err=%v): %s", username, statusCode, err, string(loginRaw))
	}

	var data struct {
		Uuid             string `json:"uuid"`
		AccessToken      string `json:"access_token"`
		AccessTokenCamel string `json:"accessToken"`
	}
	if err := json.Unmarshal(loginRaw, &data); err != nil {
		t.Fatalf("解析登录响应失败: %v (raw=%s)", err, string(loginRaw))
	}
	token := data.AccessToken
	if token == "" {
		token = data.AccessTokenCamel
	}
	if data.Uuid == "" || token == "" {
		t.Fatalf("登录响应缺少 uuid/access_token: %s", string(loginRaw))
	}
	e.putToken(data.Uuid, token)
	return data.Uuid, token
}

func (e *testEnv) makeFriendsDirectly(t *testing.T, fsDB *gorm.DB, aID, bID string) {
	t.Helper()
	now := time.Now()
	err := fsDB.Exec(`
		INSERT INTO user_friendship (user_id, friend_id, status, version, created_at, updated_at)
		VALUES (?, ?, 0, 1, ?, ?), (?, ?, 0, 1, ?, ?)
		ON DUPLICATE KEY UPDATE status = 0, updated_at = VALUES(updated_at)
	`, aID, bID, now, now, bID, aID, now, now).Error
	if err != nil {
		t.Fatalf("插入好友关系夹具失败: %v", err)
	}
}

func Test_OfflineMessageSync_CatchUp(t *testing.T) {
	e := loadEnv(t)
	fsDB := e.openDB(t, "chat_friendship")
	msgDB := e.openDB(t, "chat_message")

	tag := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	userA, tokenA := e.registerAndLoginViaEnvoy(t, "off_a_"+tag, "Password123!")
	userB, tokenB := e.registerAndLoginViaEnvoy(t, "off_b_"+tag, "Password123!")

	e.makeFriendsDirectly(t, fsDB, userA, userB)

	// 1. 用户 A 建立 WebSocket 连接（用户 B 完全离线）
	wsA := dialWS(t, e.GatewayURL, tokenA)
	defer wsA.close()

	// 等待网关注册用户 A 路由
	e.waitGatewayRoute(t, userA, 10*time.Second)

	// 2. 用户 A 连续向离线的用户 B 发送 3 条消息
	for i := 1; i <= 3; i++ {
		clientMsgId := fmt.Sprintf("e2e-off-%s-%d", tag, i)
		sendPayload := map[string]any{
			"receive_id":    userB,
			"content":       fmt.Sprintf("offline message %d", i),
			"type":          0,
			"client_msg_id": clientMsgId,
		}
		if err := wsA.conn.WriteJSON(sendPayload); err != nil {
			t.Fatalf("用户 A 发送消息 %d 失败: %v", i, err)
		}
		// 等待网关返回确认 ACK (精准匹配当前 client_msg_id，忽略欢迎消息及其他消息)
		ack, ok := wsA.waitMessage(t, 10*time.Second, func(b []byte) bool {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err == nil {
				if cid, ok := m["client_msg_id"].(string); ok && cid == clientMsgId {
					return true
				}
			}
			return false
		})
		if !ok {
			t.Fatalf("发送消息 %d 后未在超时时间内收到 ACK", i)
		}
		t.Logf("[WS A ACK] 收到发送应答 %d: %s", i, string(ack))
	}

	// 3. 校验数据库中的会话 seq_id 是否严格连续单调递增 (1, 2, 3)
	sessionId := userA + "_" + userB
	if userA > userB {
		sessionId = userB + "_" + userA
	}
	sessionId = "P_" + sessionId

	var dbSeqs []int64
	err := msgDB.Raw("SELECT seq_id FROM message WHERE session_id = ? ORDER BY seq_id ASC", sessionId).Scan(&dbSeqs).Error
	if err != nil {
		t.Fatalf("查询 message 数据库失败: %v", err)
	}
	t.Logf("[DB 核验] 会话 %s 数据库中的 seq 列表: %v", sessionId, dbSeqs)
	if len(dbSeqs) < 3 {
		t.Fatalf("期望数据库至少有 3 条消息，实际只有 %d 条", len(dbSeqs))
	}
	for i := 0; i < 3; i++ {
		if dbSeqs[i] != int64(i+1) {
			t.Errorf("期望 seq_id[%d] 为 %d, 实际为 %d", i, i+1, dbSeqs[i])
		}
	}

	// 4. 用户 B 上线，测试 Envoy HTTP 离线全量增量拉取 (after_seq:0)
	t.Run("HTTP_Sync_From_0", func(t *testing.T) {
		reqURL := fmt.Sprintf("%s/messages?target_id=%s&cursor=after_seq:0&page_size=10", e.EnvoyURL, userA)
		status, raw, err := httpDo("GET", reqURL, tokenB, nil, map[string]string{
			"x-user-id": userB,
		})
		if err != nil || status != 200 {
			t.Fatalf("HTTP GET /messages 失败 (status=%d, err=%v): %s", status, err, string(raw))
		}
		var syncData syncMessagesResponse
		if err := json.Unmarshal(raw, &syncData); err != nil {
			t.Fatalf("解析响应失败: %v (raw=%s)", err, string(raw))
		}
		if len(syncData.List) != 3 {
			t.Fatalf("期望拉取 3 条离线消息，实际拉到 %d 条: %s", len(syncData.List), string(raw))
		}
		seq1 := syncData.List[0].GetSeqId()
		seq2 := syncData.List[1].GetSeqId()
		seq3 := syncData.List[2].GetSeqId()
		if seq1 != 1 || seq2 != 2 || seq3 != 3 {
			t.Errorf("拉取到的 seq 不匹配: %v, %v, %v", seq1, seq2, seq3)
		}
		nextCursor := syncData.GetNextCursor()
		if nextCursor != "after_seq:3" {
			t.Errorf("期望 next_cursor 为 after_seq:3, 实际为 %s", nextCursor)
		}
		t.Logf("[HTTP Sync] 全量补齐成功，收到 3 条离线消息 (seq: %d, %d, %d)，NextCursor=%s", seq1, seq2, seq3, nextCursor)
	})

	// 5. 用户 B 测试游标增量补偿 (after_seq:2)
	t.Run("HTTP_Sync_From_2", func(t *testing.T) {
		reqURL := fmt.Sprintf("%s/messages?target_id=%s&cursor=after_seq:2&page_size=10", e.EnvoyURL, userA)
		status, raw, err := httpDo("GET", reqURL, tokenB, nil, map[string]string{
			"x-user-id": userB,
		})
		if err != nil || status != 200 {
			t.Fatalf("HTTP GET /messages (after_seq:2) 失败 (status=%d, err=%v): %s", status, err, string(raw))
		}
		var syncData syncMessagesResponse
		if err := json.Unmarshal(raw, &syncData); err != nil {
			t.Fatalf("解析响应失败: %v (raw=%s)", err, string(raw))
		}
		if len(syncData.List) != 1 {
			t.Fatalf("期望拉取 1 条增量消息，实际拉到 %d 条: %s", len(syncData.List), string(raw))
		}
		seq := syncData.List[0].GetSeqId()
		if seq != 3 {
			t.Errorf("期望消息 SeqId 为 3, 实际为 %d", seq)
		}
		nextCursor := syncData.GetNextCursor()
		if nextCursor != "after_seq:3" {
			t.Errorf("期望 next_cursor 为 after_seq:3, 实际为 %s", nextCursor)
		}
		t.Logf("[HTTP Sync 增量] 增量补齐成功，仅拉取到 Seq %d", seq)
	})

	// 6. 用户 B 建立 WebSocket，测试 WebSocket 网关 sync 指令
	t.Run("WebSocket_Sync_Command", func(t *testing.T) {
		wsB := dialWS(t, e.GatewayURL, tokenB)
		defer wsB.close()
		e.waitGatewayRoute(t, userB, 10*time.Second)

		// 发送 sync 指令
		syncCmd := map[string]any{
			"action":    "sync",
			"target_id": userA,
			"after_seq": 1,
			"limit":     10,
		}
		if err := wsB.conn.WriteJSON(syncCmd); err != nil {
			t.Fatalf("发送 WS sync 指令失败: %v", err)
		}

		// 等待 sync_resp
		respMsg, ok := wsB.waitMessage(t, 5*time.Second, func(b []byte) bool {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err == nil {
				return m["type"] == "sync_resp"
			}
			return false
		})
		if !ok {
			t.Fatalf("未在超时时间内收到 WS sync_resp 响应")
		}

		var wsResp struct {
			Type     string            `json:"type"`
			TargetId string            `json:"target_id"`
			Messages []syncMessageItem `json:"messages"`
			HasMore  bool              `json:"has_more"`
		}
		if err := json.Unmarshal(respMsg, &wsResp); err != nil {
			t.Fatalf("解析 WS sync_resp 失败: %v (raw=%s)", err, string(respMsg))
		}
		if wsResp.TargetId != userA {
			t.Errorf("期望 target_id 为 %s, 实际为 %s", userA, wsResp.TargetId)
		}
		if len(wsResp.Messages) != 2 {
			t.Fatalf("期望拉取 2 条消息 (Seq 2, 3)，实际拉到 %d 条: %s", len(wsResp.Messages), string(respMsg))
		}
		s1 := wsResp.Messages[0].GetSeqId()
		s2 := wsResp.Messages[1].GetSeqId()
		if s1 != 2 || s2 != 3 {
			t.Errorf("WS sync 消息 SeqId 不匹配: %v, %v", s1, s2)
		}
		t.Logf("[WS Sync 指令] 补齐成功 (Seq: %d, %d): %s", s1, s2, string(respMsg))

		// 7. 测试心跳 ping -> pong
		if err := wsB.conn.WriteJSON(map[string]any{"type": "ping"}); err != nil {
			t.Fatalf("发送 ping 失败: %v", err)
		}
		pong, ok := wsB.waitMessage(t, 3*time.Second, func(b []byte) bool {
			return string(b) == `{"type":"pong"}`
		})
		if !ok {
			t.Fatalf("未在超时时间内收到 WS pong 响应")
		}
		t.Logf("[WS 心跳] ping -> pong 成功: %s", string(pong))
	})
}
