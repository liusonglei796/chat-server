# Kama Chat Server

A distributed IM (Instant Messaging) chat system based on Go microservices and event-driven architecture.

## Language

**DistributedLock**:
A mutual-exclusion primitive backed by Redis `SETNX` and released via atomic Lua script to prevent multiple concurrent requests from modifying the same shared resource simultaneously.
_Avoid_: RedisLock, MutexLock, GlobalLock

**RequestDebounce**:
A short-window throttling mechanism backed by Redis `SETNX` on user, operation, and payload hash to reject rapid duplicate clicks (within ~1s) before business execution.
_Avoid_: RateLimiter, AntiClick, ThrottleLock

**IdempotencyKey**:
A unique client-supplied token (e.g. `X-Idempotency-Key`, `ClientMsgId`, `ClientToken`) identifying a specific operation attempt across retries.
_Avoid_: RequestDedupKey, NonceToken, MsgSeq

**IdempotencyTable**:
A persistent relational database table recording execution state, error codes, and responses within a database transaction to guarantee durable replay and prevent duplicate side-effects.
_Avoid_: DedupTable, HistoryTable, LogTable

**TokenIdempotency**:
A mechanism where clients supply an immutable key enabling the server to acquire an atomic lock and replay the cached successful response on retry without executing side-effects twice.
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
