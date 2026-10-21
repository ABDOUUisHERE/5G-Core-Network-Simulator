# Core5G

**An educational, cloud-native 5G core network simulator in Go.**

Core5G models synthetic UE registration, fictional authentication, PDU sessions and virtual packets across independent gRPC services. It is a learning and engineering demonstration, not a 3GPP-conformant implementation or a production 5G core.

![Core5G dashboard from a real PostgreSQL-backed acceptance run](docs/evidence/dashboard-desktop.png)

## Stack and scope

Go · gRPC / Protocol Buffers · PostgreSQL · Docker Compose · Prometheus · Grafana · Kubernetes / Kind.

The [CDC V2.0](docs/cahier-des-charges.pdf) is the functional specification. The previous full-stack challenge is being replaced on dedicated branches; its committed history remains available.

## Engineering process

- [Roadmap and assumptions](docs/ROADMAP.md)
- [Contribution and review workflow](CONTRIBUTING.md)
- Small feature branches, Conventional Commits, linked issues and stacked pull requests.
- Tests and benchmark evidence are reported as measured; unfinished work is identified explicitly.

## Development

Go 1.25 or Docker is required. Run `go test ./...` and `go vet ./...`.

Generate protobuf bindings with `make tools proto`; on Windows, use the equivalent commands in the Makefile through PowerShell, replacing `$(CURDIR)` with `${PWD}`.

## Run the lab

Prerequisites: Docker Desktop/Engine with Compose, several GB of free disk, and Python 3 for the demo scripts. The dashboard runs inside the Go executable; Node is only required for optional browser tests.

```sh
docker compose up --build -d --wait
python scripts/demo.py
python tests/e2e/acceptance.py
```

| Tool | Compose URL | Purpose |
| --- | --- | --- |
| Network Lab | http://127.0.0.1:8085 | UE/session lifecycle, scenarios, traffic and faults |
| Grafana | http://127.0.0.1:3005 | Provisioned Core5G dashboard, anonymous viewer |
| Prometheus | http://127.0.0.1:9095 | Service metrics and PromQL |
| Jaeger | http://127.0.0.1:16686 | OpenTelemetry traces across the request path |

One thousand synthetic subscriber profiles (`imsi-001010000000001` through `imsi-001010000001000`) are seeded with the fictional credential `demo-key`. An unknown or disabled subscriber is refused. Database and Grafana passwords in the deployment are explicitly local demonstration values.

```sh
docker compose run --rm ue-simulator
python tests/performance/load.py --count 1000 --concurrency 50
docker compose down
```

Database records survive normal shutdown in a named volume. `docker compose down -v` is a deliberate destructive reset for this lab only. PostgreSQL init migrations run on a new data volume, not on every restart.

## What is implemented

- Independent AMF, AUSF, UDM, SMF, virtual UPF, gNB, UE and control executables.
- Canonical UUIDs, synthetic SUPI validation and a fail-closed registration state machine.
- Idempotent session creation/release, unique active IPs and durable compensation.
- Packet/byte counters and reproducible latency, jitter and loss.
- Background scenarios, REST controls, a responsive dashboard and fault injection.
- Per-service metrics, correlated JSON logs, OTLP tracing and readiness/liveness.
- Service-specific PostgreSQL schemas and logins; nonroot runtime containers.
- Unit, PostgreSQL integration, real-stack E2E, browser, load and Kind validation workflows.

## Verification and evidence

The five critical business packages are `subscriber`, `authentication`, `registration`, `session` and `routing`. Local Linux Docker runs measured coverage above 80% for each and passed the race detector and `go vet`. Runtime acceptance, browser checks, deployment evidence and performance results are retained by GitHub Actions; see [validation status](docs/validation.md) for the actual completed checks and any limitations.

Compose and Kind acceptance passed. A real CI workload completed 1000 registrations at concurrency 50 with 0 errors and p95 153.04 ms. The target below 100 ms was not met; 1000 simultaneous registrations remain a separate experiment. The [raw report](docs/evidence/performance-compose.json) retains the environment and measured samples.

```sh
go test ./... -cover
go test -race ./...
go vet ./...
npm ci
npx playwright install chromium
npm run test:browser
```

On Windows without Go, `powershell -ExecutionPolicy Bypass -File scripts/go.ps1 -Task test` runs Go in Docker. `-Task race`, `vet`, `build`, `tools` and `proto` are also supported.

A Windows binary is retained as the `core5g-windows` CI artifact. For an optional **volatile in-memory preview** without Docker, extract it and run `powershell -ExecutionPolicy Bypass -File scripts/local-lab.ps1 -Binary <path-to-core5g.exe>`. Stop only those preview processes with `scripts/local-lab.ps1 -Stop`. This mode does not start PostgreSQL, Grafana or Jaeger; Compose remains the reference persistent deployment.

## Read the engineering decisions

- [Architecture and service boundaries](docs/architecture.md)
- [API reference](docs/api.md)
- [Demonstration and failure scenarios](docs/demo.md)
- [Kind/Kubernetes deployment](docs/kubernetes.md)
- [Requirements and acceptance matrix](docs/requirements.md)
- [ADRs](docs/adr/)
- [Performance methodology](docs/performance.md)

The supported topology has one active instance per stateful service. Distributed active-active orchestration is a future design task; replicas are not increased blindly. The administrative API is for a trusted local lab. No 3GPP compliance or production subscriber security is claimed.
