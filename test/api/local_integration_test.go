//go:build integration
// +build integration

package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"kama_chat_server/internal/config"
	dao "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/https_server"
	"kama_chat_server/internal/infrastructure/sms"
	"kama_chat_server/internal/service"
	chat "kama_chat_server/internal/service/chat"
	"kama_chat_server/pkg/util/jwt"
)

type apiEnvelope struct {
	Code int             `json:"code"`
	Msg  any             `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type loginData struct {
	UUID         string `json:"uuid"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func mustDo(t *testing.T, client *http.Client, method, url string, body []byte, contentType string, authHeader string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	return resp
}

func readEnvelope(t *testing.T, resp *http.Response) apiEnvelope {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode response: %v; status=%d; body=%q", err, resp.StatusCode, string(body))
	}
	return env
}

func requireNot404Or5xx(t *testing.T, method, path string, resp *http.Response) {
	t.Helper()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode >= 500 {
		t.Fatalf("%s %s: status=%d", method, path, resp.StatusCode)
	}
}

func ensureMySQLDatabaseExists(t *testing.T, conf *config.Config) {
	t.Helper()
	dsnNoDB := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=True&loc=Local",
		conf.MysqlConfig.User,
		conf.MysqlConfig.Password,
		conf.MysqlConfig.Host,
		conf.MysqlConfig.Port,
	)
	db, err := sql.Open("mysql", dsnNoDB)
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("mysql ping: %v", err)
	}
	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS " + conf.MysqlConfig.DatabaseName + " DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	if err != nil {
		t.Fatalf("create database %s: %v", conf.MysqlConfig.DatabaseName, err)
	}
}

func makePhoneSuffix() string {
	// 生成 8 位数字后缀
	n := time.Now().UnixNano() % 100_000_000
	return fmt.Sprintf("%08d", n)
}

func TestLocalIntegration_AllRoutes_Not404Or5xx(t *testing.T) {
	// 需要本机 MySQL + Redis 可用（按 configs/config.toml）
	gin.SetMode(gin.TestMode)
	_ = os.Setenv("KAMACHAT_SMS_MODE", "mock")

	conf := config.GetConfig()
	ensureMySQLDatabaseExists(t, conf)

	// 初始化基础设施
	repos := dao.Init()
	cache := myredis.Init()
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)

	services := service.NewServices(repos, cache)
	chatServer := chat.NewChatServer(chat.ChatServerConfig{
		Mode:            conf.KafkaConfig.MessageMode,
		MessageRepo:     repos.Message,
		GroupMemberRepo: repos.GroupMember,
		CacheService:    cache,
	})
	if conf.KafkaConfig.MessageMode == "kafka" {
		chatServer.InitKafka()
	}
	go chatServer.Start()
	t.Cleanup(func() {
		chatServer.Close()
	})

	if err := sms.Init(cache); err != nil {
		t.Fatalf("sms init: %v", err)
	}

	handlers := handler.NewHandlers(services, chatServer.GetBroker())
	engine := https_server.Init(handlers)
	routes := engine.Routes()

	srv := httptest.NewServer(engine)
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// ===== 先跑通注册/登录，拿到 access token（用于私有接口和 ws） =====
	phone := "139" + makePhoneSuffix()
	sendResp := mustDo(t, client, http.MethodPost, srv.URL+"/user/sendSmsCode", []byte(fmt.Sprintf(`{"telephone":"%s"}`, phone)), "application/json", "")
	requireNot404Or5xx(t, http.MethodPost, "/user/sendSmsCode", sendResp)
	_ = readEnvelope(t, sendResp)

	code, err := cache.Get(context.Background(), "auth_code_"+phone)
	if err != nil {
		t.Fatalf("read sms code from redis: %v", err)
	}
	if code == "" {
		t.Fatalf("sms code not found in redis for %s", phone)
	}

	regBody := fmt.Sprintf(`{"telephone":"%s","password":"password123","nickname":"itest","sms_code":"%s"}`, phone, code)
	regResp := mustDo(t, client, http.MethodPost, srv.URL+"/register", []byte(regBody), "application/json", "")
	requireNot404Or5xx(t, http.MethodPost, "/register", regResp)
	regEnv := readEnvelope(t, regResp)
	if regEnv.Code != 1000 {
		t.Fatalf("register failed: code=%d msg=%s", regEnv.Code, regEnv.Msg)
	}

	loginBody := fmt.Sprintf(`{"telephone":"%s","password":"password123"}`, phone)
	loginResp := mustDo(t, client, http.MethodPost, srv.URL+"/login", []byte(loginBody), "application/json", "")
	requireNot404Or5xx(t, http.MethodPost, "/login", loginResp)
	loginEnv := readEnvelope(t, loginResp)
	if loginEnv.Code != 1000 {
		t.Fatalf("login failed: code=%d msg=%s", loginEnv.Code, loginEnv.Msg)
	}
	var ld loginData
	if err := json.Unmarshal(loginEnv.Data, &ld); err != nil {
		t.Fatalf("unmarshal login data: %v", err)
	}
	if ld.AccessToken == "" || ld.UUID == "" || ld.RefreshToken == "" {
		t.Fatalf("missing tokens/uuid in login response")
	}
	authHeader := "Bearer " + ld.AccessToken

	// 刷新 token（公开接口）
	refreshBody := fmt.Sprintf(`{"refresh_token":"%s"}`, ld.RefreshToken)
	refreshResp := mustDo(t, client, http.MethodPost, srv.URL+"/auth/refresh", []byte(refreshBody), "application/json", "")
	requireNot404Or5xx(t, http.MethodPost, "/auth/refresh", refreshResp)
	_ = readEnvelope(t, refreshResp)

	// 一个明确的鉴权 GET（确保 token 真能用）
	getMe := mustDo(t, client, http.MethodGet, srv.URL+"/user/getUserInfo?uuid="+ld.UUID, nil, "", authHeader)
	requireNot404Or5xx(t, http.MethodGet, "/user/getUserInfo", getMe)
	_ = readEnvelope(t, getMe)

	// WebSocket 握手（私有路由）
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) + "/wss?client_id=" + ld.UUID
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	wsHeader := http.Header{}
	wsHeader.Set("Authorization", authHeader)
	wsConn, _, err := dialer.Dial(wsURL, wsHeader)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	_ = wsConn.Close()

	// ===== 遍历所有已注册路由：确保不 404/不 5xx（本机真实依赖） =====
	for _, r := range routes {
		method := r.Method
		path := r.Path

		if method != http.MethodGet && method != http.MethodPost {
			continue
		}
		if strings.Contains(path, "*filepath") || strings.HasPrefix(path, "/static/") {
			continue
		}
		if path == "/wss" {
			// 上面已经做了真正的 ws dial
			continue
		}

		url := srv.URL + path
		useAuth := true
		if path == "/login" || path == "/register" || path == "/user/smsLogin" || path == "/user/sendSmsCode" || path == "/auth/refresh" {
			useAuth = false
		}

		var resp *http.Response
		switch {
		case method == http.MethodPost && (path == "/message/uploadAvatar" || path == "/message/uploadFile"):
			var buf bytes.Buffer
			w := multipart.NewWriter(&buf)
			fw, err := w.CreateFormFile("file", "test.png")
			if err != nil {
				t.Fatalf("create form file: %v", err)
			}
			// 一个最小 PNG 头，满足 http.DetectContentType
			_, _ = fw.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R'})
			_ = w.Close()

			ah := ""
			if useAuth {
				ah = authHeader
			}
			resp = mustDo(t, client, method, url, buf.Bytes(), w.FormDataContentType(), ah)

		case method == http.MethodPost:
			ah := ""
			if useAuth {
				ah = authHeader
			}
			resp = mustDo(t, client, method, url, []byte(`{}`), "application/json", ah)

		default: // GET
			ah := ""
			if useAuth {
				ah = authHeader
			}
			resp = mustDo(t, client, method, url, nil, "", ah)
		}

		requireNot404Or5xx(t, method, path, resp)
		_ = readEnvelope(t, resp)
	}
}
