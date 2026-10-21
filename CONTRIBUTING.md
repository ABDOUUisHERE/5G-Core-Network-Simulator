# Contributing to Core5G

Core5G is a portfolio engineering project: small, reviewable changes and reproducible evidence matter as much as features.

1. Link the work to a GitHub issue and its CDC acceptance criteria.
2. Create `feature/<topic>`, `chore/<topic>` or `test/<topic>` from the appropriate base. Never develop directly on `main`.
3. Commit cohesive changes using Conventional Commits (`feat`, `fix`, `test`, `docs`, `chore`). Do not create empty or artificially fragmented commits.
4. Run formatting, `go vet ./...`, `go test ./...`, and `go test -race ./...` when relevant. Report commands actually run.
5. Open a PR with the problem, resulting behavior, validation and limits. Stacked PRs must name their parent; merge parents first.
6. Keep API documentation, architecture decisions and the requirements matrix current.
7. Record real benchmark measurements, including hardware and workload. A target is never a measured result.

Review checks: state transitions, dependency failures, cancellation, idempotency, data ownership, concurrent access, input validation and useful observability.

For the initial stacked delivery, review PRs #14 -> #15 -> #16 -> #17 -> #18. Merge the first into `main`, then retarget the next PR to `main` before merging it; repeat in order. Do not merge a child into a retained feature base and assume it has reached `main`. No PR is merged automatically by the project tooling.

Only synthetic subscriber identities and demonstration credentials belong in this repository. The control interface is intended for a trusted local lab.
