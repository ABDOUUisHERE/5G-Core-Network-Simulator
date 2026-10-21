# CDC traceability

Status is updated as code and evidence are delivered. A planned check is not a passing test.

| Requirement | Implementation scope | Evidence / status |
| --- | --- | --- |
| F01, F02, T01-T03 | UDM, AUSF, AMF, UE state and registration | Unit/race and real Compose/Kind acceptance passed |
| F03, T04-T05, T08-T09 | SMF, IPAM, session lifecycle and compensation | Concurrent unit tests and real-stack failure/release tests passed |
| F04, T06-T07 | Virtual UPF traffic and seeded loss | Seeded loss, counters and released-session rejection passed |
| F05 | REST control, metrics, logs and health | Real API and dashboard tested; observability stack provisioned |
| NF01, NF02, T10 | 1000 synthetic UE workload, p50/p95/p99 and environment | 1000/1000 successful at concurrency 50; p95 153.04 ms; <100 ms target not met; 1000 simultaneous not established |
| NF03 | Dependency failure and cleanup scenarios | UPF outage/restore and AUSF deadline tests passed in Compose and Kind |
| NF04 | At least 80% coverage for critical business packages | All five critical business packages above 80%; CI gate enabled |
| NF05 | Go race detector | Local Linux and GitHub checks passed |
| NF06 | Docker Compose startup | Real GitHub runner startup and acceptance passed |
| NF07 | Per-service metrics, structured logs and OTLP traces | Implemented with provisioned Grafana/Prometheus/Jaeger; explicit backend assertions in final CI pass |
| NF08 | Synthetic credentials only, ignored environment files | Source review: demonstration values only; GitHub credentials remain in process memory |

Kubernetes, ADRs, a runnable demo and real execution captures are delivered. Detailed commands, limitations and run links are in [validation status](validation.md). The reference topology supports one active instance per stateful service; active-active deployment is not claimed.
