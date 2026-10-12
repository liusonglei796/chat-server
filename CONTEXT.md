# Kama Chat Server

A distributed IM (Instant Messaging) chat system based on Go microservices and event-driven architecture.

## Language

**ApiGateway**:
The perimeter edge gateway handling public HTTP ingress, TLS termination, global rate limiting, and JWT authentication.
_Avoid_: BFF, Gateway, WebServer

**ConnService**:
The dedicated stateful service maintaining client WebSocket connections, heartbeats, and dispatching real-time push messages.
_Avoid_: Gateway, ChatServer, PushService, WsGateway

**TrustedHeader**:
Internal HTTP headers (e.g. `X-User-Id`) injected exclusively by the ApiGateway after authentication to identify the caller to upstream services.
_Avoid_: AuthHeader, ClientHeader, UserToken

**VerticalSlice**:
An incremental migration strategy that delivers a single bounded microservice end-to-end through all tiers before rolling out to other services.
_Avoid_: BigBang, LayeredMigration

**TokenIdempotency**:
A mechanism where clients supply an immutable key (e.g. `X-Idempotency-Key`, `ClientMsgId`) enabling the server to acquire an atomic lock and replay the cached successful response on retry without executing side-effects twice.
_Avoid_: RequestDedup, AntiRepeat

**NaturalIdempotency**:
Operations whose semantics are naturally safe against repetitions because they overwrite absolute values (PUT) or delete designated resources (DELETE).
_Avoid_: SafeWrite, StaticWrite

**StateIdempotency**:
Operations protected by one-way state transitions (e.g. `PENDING` -> `APPROVED`), where repeated executions match zero rows and produce safe no-ops.
_Avoid_: StatusCheck, WorkflowGuard

**AntiIdempotency**:
Security-critical operations (e.g. Login, Token Refresh) that must explicitly generate fresh cryptographic tokens and session states upon every call, strictly forbidding response replay.
_Avoid_: NonIdempotent, UncachedAction

## Service Topology & Ports (Pure gRPC Single Protocol)

| Service | Protocol & Ingress | Internal Port | Responsibility |
| :--- | :--- | :--- | :--- |
| **ApiGateway (Envoy)** | HTTP/JSON & WS (`:9080`) | - | Edge ingress, CORS, grpc_json_transcoder, WebSocket proxy |
| **user_service** | Pure gRPC | `50051` | Authentication, user profile, credentials |
| **apply_service** | Pure gRPC | `50053` | Friend & group membership applications |
| **friendship_service** | Pure gRPC | `50054` | Friend relations, blacklist, remarks |
| **group_service** | Pure gRPC | `50055` | Groups and group member management |
| **message_service** | Pure gRPC | `50056` | Sessions, message storage, cursor history query, recall |
| **ConnService** | gRPC & WS | `8001` (gRPC), `8000` (WS) | Stateful WebSocket connections, client hub, message push |
