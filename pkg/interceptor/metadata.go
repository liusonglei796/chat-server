package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UserContextKey defines a type for context keys
type UserContextKey string

const (
	UserIDKey UserContextKey = "x-user-id"
)

// ServerAuthInterceptor is a gRPC server interceptor that extracts user-id from metadata
// and puts it into the context.
func ServerAuthInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			userIDs := md.Get(string(UserIDKey))
			if len(userIDs) > 0 {
				ctx = context.WithValue(ctx, UserIDKey, userIDs[0])
			}
		}
		return handler(ctx, req)
	}
}

// ClientAuthInterceptor is a gRPC client interceptor that takes user-id from the context
// and puts it into metadata to be sent to the server.
func ClientAuthInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		userID, ok := ctx.Value(UserIDKey).(string)
		if ok && userID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, string(UserIDKey), userID)
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
