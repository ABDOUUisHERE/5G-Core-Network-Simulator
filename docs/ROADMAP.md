# Delivery plan

Core5G implements the educational scope of CDC V2.0. The initial delivery is integrated into `main`; measured results and remaining performance work are documented in [validation](validation.md).

| Step | Branch | Deliverable | Exit evidence |
| --- | --- | --- | --- |
| 1 | chore/core5g-foundation | Go module, protobuf, CI and contribution process | Generation, config tests, build |
| 2 | feature/subscriber-registration | UDM, AUSF, AMF, state management and persistence | Nominal/refused/duplicate registration tests |
| 3 | feature/pdu-traffic | SMF, UPF, IP allocation, compensation and virtual traffic | Session, packet and resilience tests |
| 4 | feature/control-observability | UE/gNB paths, REST API, dashboard, metrics and fault control | Full Docker E2E scenarios |
| 5 | test/deployment-validation | Docker, Kind manifests, load report, architecture and demo guide | Reproducible test evidence and honest limitations |

The initial PRs were reviewed as a dependency stack and merged into `main` in order. Future work follows the [contribution workflow](../CONTRIBUTING.md). Registration latency and successful 1000-UE concurrency remain open in [issue #19](https://github.com/ABDOUUisHERE/core5g/issues/19).

## Assumptions

- Educational simulator only: no radio, NAS/NGAP, 5G-AKA or GTP-U.
- Single PostgreSQL instance, separate schemas per owning service; no cross-service table access.
- Local trusted network. No production authentication is implied by fictional subscriber checks.
- Minimal browser dashboard served by Go. No JavaScript build toolchain is necessary.
- Two-second RPC timeout and five-second end-to-end timeout are configurable defaults.
- Kubernetes is a local Kind deployment; Compose is the reference demo environment.
