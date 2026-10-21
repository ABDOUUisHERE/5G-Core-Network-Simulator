# Core5G

**An educational, cloud-native 5G core network simulator in Go.**

Core5G models synthetic UE registration, fictional authentication, PDU sessions and virtual packets across independent gRPC services. It is a learning and engineering demonstration, not a 3GPP-conformant implementation or a production 5G core.

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

Implementation status and runnable deployment commands are updated in each feature PR.
