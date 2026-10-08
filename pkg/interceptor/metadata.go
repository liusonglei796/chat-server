package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UserContextKey defines a type for context keys
type UserContextKey string

const (
	UserIDKey      UserContextKey = "x-user-id"
	IdempotencyKey UserContextKey = "x-idempotency-key"
)

// ServerAuthInterceptor is a gRPC server interceptor that extracts user-id and idempotency-key from metadata
// and puts it into the context.
func ServerAuthInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if userIDs := md.Get(string(UserIDKey)); len(userIDs) > 0 {
				ctx = context.WithValue(ctx, UserIDKey, userIDs[0])
			}
			if idempKeys := md.Get(string(IdempotencyKey)); len(idempKeys) > 0 {
				ctx = context.WithValue(ctx, IdempotencyKey, idempKeys[0])
			}
		}
		return handler(ctx, req)
	}
}

// ClientAuthInterceptor is a gRPC client interceptor that takes user-id and idempotency-key from the context
// and puts it into metadata to be sent to the server (preserved across retries).
func ClientAuthInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if userID := GetUserIDFromContext(ctx); userID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, string(UserIDKey), userID)
		}
		if idempKey := GetIdempotencyKeyFromContext(ctx); idempKey != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, string(IdempotencyKey), idempKey)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// GetUserIDFromContext retrieves the user ID from the context.
func GetUserIDFromContext(ctx context.Context) string {
	userID, ok := ctx.Value(UserIDKey).(string)
	if ok {
		return userID
	}
	return ""
}

// ContextWithIdempotencyKey injects the business idempotency key into the context.
func ContextWithIdempotencyKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, IdempotencyKey, key)
}

// GetIdempotencyKeyFromContext retrieves the business idempotency key from the context.
func GetIdempotencyKeyFromContext(ctx context.Context) string {
	key, ok := ctx.Value(IdempotencyKey).(string)
	if ok {
		return key
	}
	return ""
}
