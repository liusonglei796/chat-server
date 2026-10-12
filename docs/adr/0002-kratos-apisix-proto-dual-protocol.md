# Adopt Kratos v2, Apache APISIX, and Proto Dual Protocols

We adopted Bilibili Kratos (v2) for microservices, Apache APISIX (backed by etcd) as the edge API gateway, and standard Protobuf `google.api.http` annotations to generate both gRPC and HTTP/REST endpoints from a single `.proto` file. APISIX proxies external HTTP requests directly to microservice HTTP endpoints, eliminating handwritten BFF glue code in Gin while preserving high-performance internal gRPC communication.
