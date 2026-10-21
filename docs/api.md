# Control API

Base: `http://127.0.0.1:8085/api`. JSON requests are limited to 16 KiB. Unknown fields and multiple JSON objects are rejected. Browser cross-origin mutation requests are rejected. There is no production administrator authentication; this is a trusted local lab.

The HTTP boundary defaults to five seconds; interservice gRPC calls default to two seconds. `RPC_TIMEOUT` and `REQUEST_TIMEOUT` are configurable. No automatic retry is performed for packet sends. `X-Request-ID` can carry a UUID; otherwise a canonical UUID is generated and returned. OTLP context is propagated independently.

| Method | Route | Result |
| --- | --- | --- |
| GET | `/overview` | Real UE/session counts, packet counters, service health and tool links; warnings for partial dependency failure |
| GET | `/ues` | Equipment records |
| POST | `/ues` | Create/register a synthetic UE; returns its UUID and registration result |
| POST | `/ues/{id}/register` | Register the specified UE identity |
| POST | `/ues/{id}/deregister` | Disconnect the UE and release sessions; reports cleanup failures |
| GET | `/subscribers` | Synthetic profiles without auth credentials |
| POST | `/subscribers` | Create/update a fictional subscriber profile |
| GET | `/sessions?ue_id=...` | All sessions, optionally for one UE |
| GET | `/sessions/{id}` | Session plus UPF-owned counters; `counter_error` if counters are unavailable |
| POST | `/sessions` | Create a PDU session; request replay returns the stored session |
| POST | `/sessions/{id}/release` | Idempotent release |
| POST | `/packets` | Send one virtual packet through the gNB and UPF |
| POST | `/traffic/config` | Apply latency, jitter, loss and seed settings |
| POST | `/simulations` | Start an asynchronous scenario, HTTP 202 |
| GET | `/simulations/{id}` | Progress, successes/errors and measured p50/p95/p99 |
| POST | `/faults/{service}` | Set unavailability or injected delay |

## Examples

```json
{"supi":"imsi-001010000000001","credential":"demo-key","gnb_id":"gnb-lab-01"}
```

Send that object to `POST /ues`. Optional `ue_id` and `request_id` must be canonical nonzero UUIDs. A repeated request must refer to the same UE and SUPI. Negative fictional authentication is a valid business response (`accepted` is false/omitted and `reason_code` explains why); dependency errors use HTTP error status codes.

```json
{"ue_id":"<registered-ue-uuid>","request_id":"<unique-request-uuid>"}
```

Send to `POST /sessions`. Only `REGISTERED`/`SESSION_ACTIVE` UE are eligible. A request ID belongs permanently to one creation attempt, including failed/released attempts. Use a new request ID for a new attempt.

```json
{"session_id":"<active-session-uuid>","size":512,"destination":"203.0.113.10"}
```

Send to `POST /packets`. Size is 1..65507 bytes. Source defaults to the session's allocated IP; explicit source must match it. Timestamp defaults to the server time. Latency/loss are deterministic for the seed and packet request UUID; repeating a send increments counters again.

```json
{"latency_ms":2,"jitter_ms":1,"loss_percent":1,"seed":42}
```

Send to `POST /traffic/config`. Latency/jitter are 0..1000 ms; loss is 0..100%. All values must be finite.

```json
{"count":100,"concurrency":10,"full":true}
```

Send to `POST /simulations`. Count is 1..1000 and worker concurrency is 1..100. Full scenarios register, create a session and send one packet. One background job runs at a time; the latest 20 jobs are retained in control-process memory. These progress records are not durable business records. A 1000-UE count does not imply 1000 simultaneous registrations.

```json
{"enabled":true,"delay_ms":0}
```

Send to `POST /faults/upf`; restore with `{"enabled":false,"delay_ms":0}`. Supported services: `udm`, `ausf`, `amf`, `smf`, `upf`, `gnb-simulator`. Fault controls remain reachable while the service's business RPCs fail. Fault state is process-local and resets on restart.

## Error mapping

`INVALID_ARGUMENT` -> 400; `NOT_FOUND` -> 404; `ALREADY_EXISTS`/`FAILED_PRECONDITION` -> 409; `RESOURCE_EXHAUSTED` -> 429; `UNAVAILABLE` -> 503; `DEADLINE_EXCEEDED` -> 504. Error bodies have `code` and `message`. Internal persistence details are not returned.

Every long-running service exposes `/healthz`, `/readyz` and `/metrics` on internal port 8080. Business protobuf definitions and generated clients are in `api/proto/core/v1` and `api/gen/core/v1`.
