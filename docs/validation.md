# Validation status

## Completed locally

- Go 1.25 / Linux Docker: `go test ./...`, `go vet ./...` and `go test -race ./...` passed for the implemented core.
- Real PostgreSQL integration verified partial active-IP uniqueness, IP reuse after release, rollback and cross-schema access denial.
- Critical business package coverage: authentication 100%, subscriber 91.2%, registration 83.5%, session 88.1%, routing 86.1%.
- Protobuf bindings were generated with pinned Go plugins.
- Dashboard JavaScript syntax and Python scripts were checked.

## GitHub checks

The following runs passed:

- [Foundation](https://github.com/ABDOUUisHERE/core5g/actions/runs/39599184089)
- [Registration](https://github.com/ABDOUUisHERE/core5g/actions/runs/39599743000)
- [Sessions and routing](https://github.com/ABDOUUisHERE/core5g/actions/runs/39600242167)
- [Complete Compose runtime, four browser widths and measured load](https://github.com/ABDOUUisHERE/core5g/actions/runs/39603433550)
- [Go quality gate, Compose and real Kind cluster](https://github.com/ABDOUUisHERE/core5g/actions/runs/39603443408)

Both Compose and Kind passed the six acceptance groups covering CDC T01-T09, including unknown/disabled/bad credentials, duplicate requests, unique IPs, released-session rejection, UPF compensation, registration timeout and a full background scenario. All eleven Kind pods were ready at capture time; stateful services retried startup while PostgreSQL initialized.

The first browser run found an assertion that checked drawer closure before the asynchronous hashchange. The test now waits for the actual transition; all five views passed at 1440, 768, 390 and 320 pixels, with no page errors or horizontal root overflow. The skip-link test also preserves the current route.

Retained repository evidence: [coverage](evidence/coverage.json), [browser results](evidence/browser-results.json), [Kind pods](evidence/kubernetes-pods.txt), [Kind events](evidence/kubernetes-events.txt), [raw measured workload](evidence/performance-compose.json), and [desktop](evidence/dashboard-desktop.png)/[mobile](evidence/dashboard-mobile.png) captures of the real PostgreSQL-backed run.

Explicit Prometheus, provisioned-Grafana and multi-service-Jaeger assertions passed in both Compose and Kind in [this validation run](https://github.com/ABDOUUisHERE/core5g/actions/runs/39604489154). They verified seven scraped services, eight provisioned Grafana panels and actual traces spanning multiple services. Retained checks: [Compose](evidence/observability-compose.json) and [Kind](evidence/observability-kind.json).

## First measured workload

Environment: Ubuntu 24.04.5 LTS runner, 4 logical x86-64 CPUs, 16,760,336,384 bytes RAM, Docker 28.0.4. The runner reported architecture rather than a CPU model. Monitoring/tracing were enabled. One iteration, 1000 new UE registrations, worker concurrency 50, existing acceptance records retained. The raw report records the clean CI merge SHA and sampled container resources.

| Measurement | Actual result |
| --- | --- |
| Successful registrations | 1000 / 1000 |
| Errors | 0 |
| p50 | 112.99 ms |
| p95 | 153.04 ms |
| p99 | 270.24 ms |
| Wall time | 2.50 s |
| Throughput | 400.38 requests/s |

These are client-observed measurements of the entire HTTP/gRPC/PostgreSQL path on one shared CI runner. The p95 target below 100 ms was not met. A workload of 1000 requests at concurrency 50 does not establish 1000 simultaneous registrations; the harness supports that separate experiment. Do not generalize one iteration into a capacity guarantee.

## Local infrastructure incident

During the first full image build, the host C: drive had approximately 0.57 GiB free and the Docker filesystem became read-only; the daemon then stopped answering normally. Full Compose/Kind validation was moved to isolated GitHub runners. No unrelated container or database was deleted or restarted, and no host-wide Docker prune was performed.

Before running the full local stack again, restore sufficient free disk and a healthy Docker Desktop daemon. This infrastructure incident is separate from the successfully completed core unit/race/PostgreSQL tests.

## Not yet established

- The sub-100-ms registration p95 target.
- 1000 simultaneous successful registrations.
- Multi-writer/active-active service safety.
- Production security or 3GPP conformity (outside scope).
