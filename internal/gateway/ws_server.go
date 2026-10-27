package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/pkg/errorx"
)

// httpServerShim 维护测试与外部对于原生 http.Handler 的访问兼容
type httpServerShim struct {
	Addr    string
	Handler http.Handler
}

// WsServer 专职负责 WebSocket 长连接与服务自省的 Multi-Reactor 网络服务
type WsServer struct {
	hub      *ConnHub
	engine   *nbhttp.Engine
	upgrader *websocket.Upgrader
	mux      *http.ServeMux
	server   *httpServerShim
}

// NewWsServer 创建并配置基于 nbio Multi-Reactor 的长连接服务
func NewWsServer(addr string, hub *ConnHub) *WsServer {
	mux := http.NewServeMux()
	upgrader := websocket.NewUpgrader()
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	s := &WsServer{
		hub:      hub,
		upgrader: upgrader,
		mux:      mux,
		server: &httpServerShim{
			Addr:    addr,
			Handler: mux,
		},
	}

	// 注册 Reactor 事件生命周期回调
	s.registerReactorCallbacks()

	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/ws/logout", s.handleLogout)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ping", s.handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	engine := nbhttp.NewEngine(nbhttp.Config{
		Network:            "tcp",
		Addrs:              []string{addr},
		Handler:            mux,
		KeepaliveTime:      120 * time.Second,
		MaxWriteBufferSize: 1024 * 1024, // 1MB 慢连接写缓冲内核水位线保护
	})
	s.engine = engine

	return s
}

// registerReactorCallbacks 绑定 Reactor 底层事件驱动回调
func (s *WsServer) registerReactorCallbacks() {
	s.upgrader.OnOpen(func(c *websocket.Conn) {
		zap.L().Debug("Reactor OnOpen connected", zap.String("remoteAddr", c.RemoteAddr().String()))
	})

	s.upgrader.OnMessage(func(c *websocket.Conn, messageType websocket.MessageType, data []byte) {
		session := c.Session()
		if session == nil {
			return
		}
		userConn, ok := session.(*UserConn)
		if !ok || userConn == nil {
			return
		}

		userConn.lastActive.Store(time.Now().Unix())
		if userConn.hub != nil {
			userConn.hub.RenewUserLocation(userConn.Uuid)
		}

		// 1. 快速检查是否为心跳 Ping 指令（毫秒级非阻塞就地回包）
		var cmdHeader struct {
			Type     string `json:"type"`
			Action   string `json:"action"`
			TargetId string `json:"target_id"`
			AfterSeq int64  `json:"after_seq"`
			Limit    int32  `json:"limit"`
		}
		if err := json.Unmarshal(data, &cmdHeader); err == nil {
			if cmdHeader.Type == "ping" || cmdHeader.Action == "ping" {
				pongMsg, _ := json.Marshal(map[string]interface{}{"type": "pong"})
				userConn.SafeSend(pongMsg, "")
				return
			}
		}

		// 2. 耗时业务（增量同步 / 聊天消息发送）解耦至协程池，严禁阻塞 Sub-Reactor 事件循环
		task := func() {
			HandleUserMessageTask(userConn, data, &cmdHeader)
		}

		if userConn.hub != nil {
			userConn.hub.SubmitWriteTask(task)
		} else {
			go task()
		}
	})

	s.upgrader.OnClose(func(c *websocket.Conn, err error) {
		if session := c.Session(); session != nil {
			if userConn, ok := session.(*UserConn); ok {
				userConn.cleanup()
			}
		}
	})
}

// Start 启动 Multi-Reactor 网络引擎
func (s *WsServer) Start() error {
	zap.L().Info("WebSocket ConnServer (nbio Multi-Reactor) listening", zap.String("addr", s.server.Addr))
	return s.engine.Start()
}

// Shutdown 优雅关闭 Multi-Reactor 服务
func (s *WsServer) Shutdown(ctx context.Context) error {
	return s.engine.Shutdown(ctx)
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

	var responseHeader http.Header
	if secProto := r.Header.Get("Sec-WebSocket-Protocol"); secProto != "" {
		for _, p := range strings.Split(secProto, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				if responseHeader == nil {
					responseHeader = make(http.Header)
				}
				responseHeader.Set("Sec-WebSocket-Protocol", trimmed)
				break
			}
		}
	}

	conn, err := s.upgrader.Upgrade(w, r, responseHeader)
	if err != nil {
		zap.L().Error("websocket upgrade error", zap.Error(err))
		return
	}

	InitReactorClient(conn, userId, s.hub)
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
