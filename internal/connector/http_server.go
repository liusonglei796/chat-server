package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/pkg/errorx"
)

// HttpServer 专职负责 WebSocket 长连接握手升级与服务自省探活的 HTTP 服务 (基于 Go 原生 net/http 与 coder/websocket)
// 监听 8000 端口，承担 /ws 长连接接入、/ws/logout、/health 探活与 /metrics 监控
type HttpServer struct {
	hub     *ConnHub
	mux     *http.ServeMux
	httpSrv *http.Server
}

// NewHttpServer 创建并配置基于 coder/websocket 的长连接 HTTP 服务
func NewHttpServer(addr string, hub *ConnHub) *HttpServer {
	mux := http.NewServeMux()

	s := &HttpServer{
		hub: hub,
		mux: mux,
		httpSrv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
	}

	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/ws/logout", s.handleLogout)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ping", s.handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	return s
}

// ServeHTTP 实现 http.Handler 接口，将请求委托给内部 mux，便于测试与中间件挂载
func (s *HttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Start 启动 HTTP/WebSocket 服务
func (s *HttpServer) Start() error {
	zap.L().Info("Connector HTTP/WebSocket Server listening", zap.String("addr", s.httpSrv.Addr))
	err := s.httpSrv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown 优雅关闭 HTTP/WebSocket 服务
func (s *HttpServer) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// extractBearerUserID 解析 Bearer Token 并返回用户 ID
func extractBearerUserID(tokenStr string) string {
	if tokenStr == "" {
		return ""
	}
	if claims, err := jwt.ParseToken(tokenStr); err == nil && claims != nil {
		return claims.UserID
	}
	return ""
}

// extractWsUserID 从 WebSocket 握手请求中提取经过认证的用户 ID
func extractWsUserID(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if uid := extractBearerUserID(parts[1]); uid != "" {
				return uid
			}
		}
	}

	// 2. Sec-WebSocket-Protocol: "Bearer, <token>" 或 "Bearer <token>"
	protocols := strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",")
	for i, protocol := range protocols {
		protocol = strings.TrimSpace(protocol)
		if strings.EqualFold(protocol, "Bearer") && i+1 < len(protocols) {
			if uid := extractBearerUserID(strings.TrimSpace(protocols[i+1])); uid != "" {
				return uid
			}
		}
		if strings.HasPrefix(strings.ToLower(protocol), "bearer ") {
			if uid := extractBearerUserID(strings.TrimSpace(protocol[len("bearer "):])); uid != "" {
				return uid
			}
		}
	}

	// 3. Query 参数 token
	if uid := extractBearerUserID(r.URL.Query().Get("token")); uid != "" {
		return uid
	}

	// 4. 受信网关透传的 Header（兼容旧部署与测试）
	return r.Header.Get("X-User-Id")
}

// handleWebSocket 处理 /ws 升级请求
func (s *HttpServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	userId := extractWsUserID(r)
	if userId == "" {
		zap.L().Warn("unauthorized websocket connection attempt", zap.String("remoteAddr", r.RemoteAddr))
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": errorx.CodeUnauthorized,
			"msg":  "请先登录",
		})
		return
	}

	opts := &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	}
	if secProto := r.Header.Get("Sec-WebSocket-Protocol"); secProto != "" {
		for _, p := range strings.Split(secProto, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				opts.Subprotocols = []string{trimmed}
				break
			}
		}
	}

	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		zap.L().Error("websocket upgrade error", zap.Error(err))
		return
	}

	client := NewUserConn(conn, userId)
	if s.hub != nil {
		s.hub.RegisterClient(client)
	}
	go client.readPump(s.hub)
}

// handleLogout 处理 /ws/logout 登出请求
func (s *HttpServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	userId := extractWsUserID(r)
	if userId == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": errorx.CodeUnauthorized,
			"msg":  "请先登录",
		})
		return
	}

	if err := ClientLogout(userId, s.hub); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": errorx.CodeServerBusy,
			"msg":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": errorx.CodeSuccess,
		"msg":  "success",
	})
}

// handleHealth 健康探活
func (s *HttpServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
