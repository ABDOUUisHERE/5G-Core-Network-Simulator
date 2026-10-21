# ADR 0001: Explicit simulation boundaries and local deployment

Status: accepted for the MVP.

## Decision

Implement Go microservices with protobuf/gRPC, PostgreSQL ownership per service and virtual packet processing. Use Docker Compose as the reference environment; provide Kind manifests afterward. Keep the administrative interface local and trusted.

## Rationale

The exercise demonstrates distributed state, idempotency, compensation, cancellation and observability. Implementing real radio protocols or subscriber authentication would change the purpose and scope.

## Consequences

No production 3GPP compliance or subscriber security is claimed. Performance goals require measurements on a documented environment. One active stateful service instance is supported initially; cross-replica orchestration must be designed before scaling replicas.
