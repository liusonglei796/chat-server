# Vertical Slice Migration and Gateway Authentication Offloading

We chose to migrate existing microservices to Kratos using a vertical slice strategy starting with `user_service`, while completely offloading JWT verification to Apache APISIX to inject trusted headers (`X-User-Id`) into upstream services and `ConnService`. Custom infrastructure libraries (`pkg/discovery`, `pkg/interceptor`, `pkg/otel`, `grpc_client`) are deprecated in favor of official Kratos and APISIX modules, while domain primitives (`pkg/enum`, `pkg/errorx`, `pkg/constants`) are retained.
