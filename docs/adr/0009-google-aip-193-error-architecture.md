# Adopt Google AIP-193 Error Architecture with Kratos and Envoy Transcoding

## Context
The system previously returned legacy 4-digit numeric business error codes (1001-1013) inside a custom `errorx.CodeError` struct. Because this struct did not implement the gRPC `GRPCStatus() *status.Status` interface, all business errors degraded to `codes.Unknown(2)` at the gRPC transport boundary. In turn, Envoy's `grpc_json_transcoder` translated these into generic HTTP 500 responses, causing business rejections to be indistinguishable from backend server crashes and failing E2E error contract verification (B7).

## Decision
We decided to adopt the official **Google AIP-193 Error Specification** natively across all microservices:
1. **Canonical gRPC Codes**: Classify errors into standard gRPC canonical status codes (e.g., `InvalidArgument`, `NotFound`, `PermissionDenied`, `AlreadyExists`, `Unavailable`), which Envoy maps 1:1 to semantic HTTP 4xx/5xx status codes.
2. **Proto-First ErrorReason via Kratos**: Declare service-scoped `enum XxxErrorReason` in each microservice `.proto` file with `(errors.code)` annotations, compiled via `protoc-gen-go-errors` into strongly-typed constructors and predicates.
3. **Structured Error Payload**: Carry domain context and failure reasons inside `google.rpc.ErrorInfo` (`domain: "kama.chat"`, uppercase `reason`, and key-value `metadata`) within the `details` field of `google.rpc.Status`.
4. **Server-Side Observability**: Configure Zap with `zap.AddStacktrace(zapcore.ErrorLevel)` to record multi-frame call stacks and correlate with OpenTelemetry TraceIDs in backend logs, while strictly suppressing internal stack trace leakage to client responses.

## Consequences
- **Eliminated Boundary Degradation**: Envoy and clients receive accurate HTTP status codes and machine-readable `reason` tokens without error code loss.
- **Contract as Single Source of Truth**: Microservice errors are fully declared in Protobuf schema, eliminating magic numbers in code.
- **Full Fleet Migration Completed**: All domain services (`user`, `friendship`, `apply`, `group`, `message`) and Envoy gateway have completed migration following the VerticalSlice pattern, with 100% unit tests passing and global protobuf descriptor regenerated.
