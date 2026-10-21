# Delivery plan

The user has authorized implementation of CDC V2.0. This plan treats the PDF as requirements, not as independent authorization to publish, merge, or claim results.

| Step | Branch | Deliverable | Exit evidence |
| --- | --- | --- | --- |
| 1 | chore/core5g-foundation | Replace prior application; Go module, protobuf, CI and contribution process | Generation, config tests, build |
| 2 | feature/subscriber-registration | UDM, AUSF, AMF, state management and persistence | Nominal/refused/duplicate registration tests |
| 3 | feature/pdu-traffic | SMF, UPF, IP allocation, compensation and virtual traffic | Session, packet and resilience tests |
| 4 | feature/control-observability | UE/gNB paths, REST API, dashboard, metrics and fault control | Full Docker E2E scenarios |
| 5 | test/deployment-validation | Docker, Kind manifests, load report, architecture and demo guide | Reproducible test evidence and honest limitations |

The PRs are intentionally stacked: each depends on the preceding branch. No automatic merges are part of this delivery. Repository renaming and branch protection changes are outside the current scope.

## Assumptions

- Educational simulator only: no radio, NAS/NGAP, 5G-AKA or GTP-U.
- Single PostgreSQL instance, separate schemas per owning service; no cross-service table access.
- Local trusted network. No production authentication is implied by fictional subscriber checks.
- Minimal browser dashboard served by Go. No JavaScript build toolchain is necessary.
- Two-second RPC timeout and five-second end-to-end timeout are configurable defaults.
- Kubernetes is a local Kind deployment; Compose is the reference demo environment.
