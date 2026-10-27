package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/pkg/errorx"
)

func init() {
	jwt.Init("test-secret-key-1234567890123456", 60, 24)
}

func TestWsServer_HealthCheck(t *testing.T) {
	wsServer := NewWsServer(":0", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	wsServer.server.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())

	// 测试 /ping
	reqPing := httptest.NewRequest(http.MethodGet, "/ping", nil)
	wPing := httptest.NewRecorder()
	wsServer.server.Handler.ServeHTTP(wPing, reqPing)

	assert.Equal(t, http.StatusOK, wPing.Code)
	assert.Equal(t, "OK", wPing.Body.String())
}

func TestWsServer_ExtractWsUserID(t *testing.T) {
	// 1. 测试 X-User-Id 请求头
	req1 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req1.Header.Set("X-User-Id", "u1001")
	assert.Equal(t, "u1001", extractWsUserID(req1))

	// 2. 测试 Query Token
	tokenStr, err := jwt.GenerateAccessToken("u1002", false)
	assert.NoError(t, err)
	req2 := httptest.NewRequest(http.MethodGet, "/ws?token="+tokenStr, nil)
	assert.Equal(t, "u1002", extractWsUserID(req2))

	// 3. 测试 Authorization: Bearer <token>
	tokenStr3, err := jwt.GenerateAccessToken("u1003", false)
	assert.NoError(t, err)
	req3 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req3.Header.Set("Authorization", "Bearer "+tokenStr3)
	assert.Equal(t, "u1003", extractWsUserID(req3))

	// 4. 测试 Sec-WebSocket-Protocol: Bearer, <token>（浏览器 WebSocket 用法）
	tokenStr4, err := jwt.GenerateAccessToken("u1004", false)
	assert.NoError(t, err)
	req4 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req4.Header.Set("Sec-WebSocket-Protocol", "Bearer, "+tokenStr4)
	assert.Equal(t, "u1004", extractWsUserID(req4))

	// 5. 测试 Sec-WebSocket-Protocol: Bearer <token>（合并写法）
	tokenStr5, err := jwt.GenerateAccessToken("u1005", false)
	assert.NoError(t, err)
	req5 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req5.Header.Set("Sec-WebSocket-Protocol", "Bearer "+tokenStr5)
	assert.Equal(t, "u1005", extractWsUserID(req5))

	// 6. 测试无认证信息
	req6 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	assert.Equal(t, "", extractWsUserID(req6))
}

func TestWsServer_UnauthorizedWs(t *testing.T) {
	wsServer := NewWsServer(":0", nil)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()
	wsServer.server.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, float64(errorx.CodeUnauthorized), resp["code"])
}

func TestWsServer_Logout(t *testing.T) {
	wsServer := NewWsServer(":0", nil)

	// 非 POST 方法拒绝
	reqGet := httptest.NewRequest(http.MethodGet, "/ws/logout", nil)
	wGet := httptest.NewRecorder()
	wsServer.server.Handler.ServeHTTP(wGet, reqGet)
	assert.Equal(t, http.StatusMethodNotAllowed, wGet.Code)

	// 未认证 POST
	reqPostUnauth := httptest.NewRequest(http.MethodPost, "/ws/logout", nil)
	wPostUnauth := httptest.NewRecorder()
	wsServer.server.Handler.ServeHTTP(wPostUnauth, reqPostUnauth)
	assert.Equal(t, http.StatusOK, wPostUnauth.Code)
	var resp map[string]interface{}
	_ = json.Unmarshal(wPostUnauth.Body.Bytes(), &resp)
	assert.Equal(t, float64(errorx.CodeUnauthorized), resp["code"])
}
