# Validation status

## Completed locally

- Go 1.25 / Linux Docker: `go test ./...`, `go vet ./...` and `go test -race ./...` passed for the implemented core.
- Real PostgreSQL integration verified partial active-IP uniqueness, IP reuse after release, rollback and cross-schema access denial.
- Critical business package coverage: authentication 100%, subscriber 91.2%, registration 83.5%, session 88.1%, routing 86.1%.
- Protobuf bindings were generated with pinned Go plugins.
- Dashboard JavaScript syntax and Python scripts were checked.

## GitHub checks

Foundation, registration and session PR CI runs passed. The complete Compose acceptance/browser/load workflow and Kubernetes workflow produce retained artifacts. Their final outcomes and selected evidence are added here after execution; a configured job is not a passing result.

## Local infrastructure incident

During the first full image build, the host C: drive had approximately 0.57 GiB free and the Docker filesystem became read-only; the daemon then stopped answering normally. Full Compose/Kind validation was moved to isolated GitHub runners. No unrelated container or database was deleted or restarted, and no host-wide Docker prune was performed.

Before running the full local stack again, restore sufficient free disk and a healthy Docker Desktop daemon. This infrastructure incident is separate from the successfully completed core unit/race/PostgreSQL tests.

## Not yet established

- The sub-100-ms registration p95 target.
- 1000 simultaneous successful registrations.
- Multi-writer/active-active service safety.
- Production security or 3GPP conformity (outside scope).
