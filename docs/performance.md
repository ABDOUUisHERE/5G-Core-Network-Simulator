# Performance methodology and limits

CDC targets: 1000 concurrent UE as a load objective, and local registration p95 below 100 ms as a target rather than a guarantee. Neither is assumed from unit-test success.

```sh
python tests/performance/load.py --count 1000 --concurrency 50 --iterations 3 --output test-results/performance.json
```

The script executes new registrations through HTTP -> gNB -> AMF -> AUSF -> UDM -> PostgreSQL. It records workload, count, worker concurrency, iterations, successful/all-request p50/p95/p99, error samples, throughput, wall time, CPU/OS/RAM, Docker resources and sampled container CPU/memory. Synthetic subscribers are reused; each UE and request UUID is new. Existing persisted records remain present and are documented as part of the environment.

For the separate synchronized 1000-request experiment, run `go run ./tests/performance/burst --count 1000 --concurrency 1000`. All goroutines are released from one gate. The report records the observed peak of actual HTTP attempts, successes, failures, separate success/all-request percentiles and readiness after the load. It does not count waiting goroutines as in-flight requests. CI executes this immediately after the baseline on the same runner; `burst.json` is retained with the baseline environment/resource report.

Raw reports belong in CI artifacts. Small reviewed summaries and selected raw reports can be retained under `docs/evidence`. Each report states its commit and whether the working tree was dirty. Screenshots are evidence of a real run, not a benchmark.

## Interpretation

- 1000 requests at concurrency 50 is not a proof of 1000 simultaneous registrations.
- Error-inclusive percentiles and success-only percentiles are different; the script reports both.
- Docker CPU sampling is coarse and contains the monitoring overhead; no isolated per-request CPU claim is made.
- CI hardware is shared and varies. Record repeated iterations before drawing performance conclusions.
- The current single-writer lifecycle and service-wide persistence serialization favor correctness over peak parallel throughput. Per-UE locking, indexed request lookup and less broad read transactions are concrete future optimizations.
- Increase worker concurrency progressively, then test 1000 concurrency explicitly. The harness supports it, but the result must be reported with its failures.

The first actual run completed 1000/1000 registrations at concurrency 50, with 0 errors, p50 112.99 ms, p95 153.04 ms and p99 270.24 ms. This does not meet the 100-ms p95 target. See [validation status](validation.md) and the [raw CI report](evidence/performance-compose.json) for hardware, commit, workload, resource samples and limitations. No fabricated result fills an unexecuted benchmark.
