# CDC traceability

Status is updated as code and evidence are delivered. A planned check is not a passing test.

| Requirement | Implementation scope | Evidence / status |
| --- | --- | --- |
| F01, F02, T01-T03 | UDM, AUSF, AMF, UE state and registration | Registration issue |
| F03, T04-T05, T08-T09 | SMF, IPAM, session lifecycle and compensation | Sessions issue |
| F04, T06-T07 | Virtual UPF traffic and seeded loss | Sessions issue |
| F05 | REST control, metrics, logs and health | Observability issue |
| NF01, NF02, T10 | 1000 synthetic UE workload, p50/p95/p99 and environment | Validation issue; targets unmeasured |
| NF03 | Dependency failure and cleanup scenarios | Resilience tests planned |
| NF04 | At least 80% coverage for critical business packages | Coverage report pending |
| NF05 | Go race detector | Race check pending |
| NF06 | Docker Compose startup | Deployment check pending |
| NF07 | Per-service metrics and correlated structured logs | Observability issue |
| NF08 | Synthetic credentials only, ignored environment files | CI and review |

The source document also asks for Kubernetes, ADRs, a demo and real execution captures. Their validation is tracked separately from the functional MVP.
