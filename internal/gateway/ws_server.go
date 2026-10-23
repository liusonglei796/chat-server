package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/pkg/errorx"
)

// WsServer 专职负责 WebSocket 长连接与服务自省的原生 HTTP 服务
type WsServer struct {
	hub    *ConnHub
	server *http.Server
}

// NewWsServer 创建并配置长连接 HTTP 服务
func NewWsServer(addr string, hub *ConnHub) *WsServer {
	mux := http.NewServeMux()
	s := &WsServer{
		hub: hub,
	}

	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/ws/logout", s.handleLogout)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ping", s.handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Start 启动 HTTP 服务
func (s *WsServer) Start() error {
	zap.L().Info("WebSocket ConnServer listening", zap.String("addr", s.server.Addr))
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown 优雅关闭 HTTP 服务
func (s *WsServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// extractWsUserID 从请求头或 Query 中提取经过认证的用户 ID
func extractWsUserID(r *http.Request) string {
	// 1. 优先从网关（如 Envoy）透传的 Header 中读取
	if uid := r.Header.Get("X-User-Id"); uid != "" {
		return uid
	}

	// 2. 从 URL Query 参数 "token" 读取（客户端 WebSocket 握手常用）
	if tokenStr := r.URL.Query().Get("token"); tokenStr != "" {
		if claims, err := jwt.ParseToken(tokenStr); err == nil && claims != nil {
			return claims.UserID
		}
	}

	// 3. 从 Authorization: Bearer <token> 请求头读取
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if claims, err := jwt.ParseToken(parts[1]); err == nil && claims != nil {
				return claims.UserID
			}
		}
	}

	return ""
}

// handleWebSocket 处理 /ws 升级请求
func (s *WsServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	userId := extractWsUserID(r)
	if userId == "" {
		zap.L().Warn("unauthorized websocket connection attempt", zap.String("remoteAddr", r.RemoteAddr))
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": errorx.CodeUnauthorized,
			"msg":  "请先登录",
		})
		return
	}

	NewClientInit(w, r, userId, s.hub)
}

// handleLogout 处理 /ws/logout 登出请求
func (s *WsServer) handleLogout(w http.ResponseWriter, r *http.Request) {
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
func (s *WsServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
