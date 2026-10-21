# A five-minute engineering demo

## 1. Start and inspect

Run `docker compose up --build -d --wait`, then open http://127.0.0.1:8085. The dashboard starts empty on a new database. It reads real API state; no sample graphs or fabricated metrics are injected.

## 2. Walk through a nominal flow

Run `python scripts/demo.py` or `docker compose run --rm ue-simulator`. Follow UE -> gNB -> AMF -> AUSF -> UDM for registration, then AMF -> SMF -> UPF for the session. Send a virtual packet, observe counters, release the session twice, and verify packet rejection afterward.

In the dashboard, use **Register a UE**, then **UE devices -> Session**. Use the **PDU sessions** controls to send a packet or release a connection.

## 3. Demonstrate resilience

Register a UE before injecting the fault. In **Observability**, inject a UPF failure and attempt a new session. The request fails, the session does not become active, and an IP remains reserved if route cleanup cannot be confirmed. Restore UPF. Within a reconciliation cycle, the cleanup is retried; the failed session releases the reservation.

Inject an AUSF delay longer than two seconds, then register a new UE. The request must time out and the UE must return to `DISCONNECTED`. Use the API for the delay:

```json
{"enabled":false,"delay_ms":2500}
```

Post to `/api/faults/ausf`. Always restore with `{"enabled":false,"delay_ms":0}` afterward.

## 4. Inspect evidence

- Grafana: registered UE, active sessions, RPC latency/errors and packet outcomes.
- Jaeger: select `control-api` or a core service and inspect an actual request trace.
- `docker compose logs amf smf upf`: structured JSON with request IDs, trace IDs and status codes.
- `python tests/e2e/acceptance.py`: nominal, negative and injected-failure acceptance scenarios.

## 5. Discuss engineering tradeoffs

Explain durable reservation before external effects, idempotency ownership, bounded compensations, IP quarantine during uncertain cleanup, and revisioned derived UE state. State the supported single-writer topology. Show the measured benchmark report, including errors and unmet targets, rather than claiming telecom-grade scalability.
