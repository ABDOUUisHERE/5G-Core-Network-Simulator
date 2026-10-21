# ADR 0002: Owned schemas, short transactions and durable compensation

Status: accepted for the MVP.

Each stateful service has its own PostgreSQL login and schema. A service cannot read the others' business tables. JSONB snapshots preserve the protobuf model; generated SQL columns enforce UUID, state, request-id and active-IP constraints. This adapts the CDC logical schema without cross-service foreign keys.

Transactions are short and contain no RPC calls. State is reserved before external effects. Canceled workflows use independent, bounded contexts for compensation. Pending session cleanup retains its IP until UPF removal is confirmed. Startup recovery handles interrupted reservations; a reconciler retries pending cleanup and session-state notifications.

One active instance of each owning service serializes lifecycle workflows. PostgreSQL transactions and constraints protect durable records, but automatic active-active failover is not supported. Deployments explicitly use one replica.

AMF session-state events carry monotonic per-session revisions. A late create event cannot reactivate a released session. SMF retries state synchronization during reconciliation. This derived display state is eventually consistent during dependency failure; the SMF session record is authoritative for packet authorization.
