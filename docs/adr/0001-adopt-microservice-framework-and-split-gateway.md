# Adopt Go Microservice Framework and Split API Gateway

We chose to adopt an established Go microservice framework (process-level SDK governance) and decouple the monolithic API gateway into an edge ApiGateway and a stateful ConnService, rather than migrating to a Service Mesh. This replaces bespoke service discovery, circuit breaking, and retry code with battle-tested libraries while preserving lightweight local Docker Compose development and separating stateless HTTP routing from stateful WebSocket connection management.
