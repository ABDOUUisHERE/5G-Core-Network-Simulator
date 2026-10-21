# First-run testing guide

This guide starts with a complete local lab, then covers automated checks. Run commands from the repository root, where `docker-compose.yml` and `go.mod` are located. PowerShell commands below work from the IDE terminal.

## 1. Check the prerequisites

Install Git, Python 3 and Docker Desktop with Linux containers. Start Docker Desktop and wait for its engine to be ready.

```powershell
git --version
python --version
docker compose version
docker info
```

`docker info` must return server information. If it hangs or reports that the daemon cannot be reached, fix Docker Desktop before building the lab. The build needs several GB of free disk space. Do not remove unrelated containers or databases to make room.

Go is built inside Docker. You do not need a host Go installation for the Compose demo. Node.js is only needed for browser tests.

If you have not cloned the repository:

```powershell
git clone https://github.com/ABDOUUisHERE/5G-Core-Network-Simulator.git
cd 5G-Core-Network-Simulator
```

If the project is already open in your IDE, use its existing directory.

## 2. Start the complete lab

```powershell
docker compose up --build -d --wait
docker compose ps
```

The first build downloads images and dependencies and can take several minutes. The command should finish successfully and the long-running services should become healthy or running. The UE simulator is an on-demand tool, so it does not run continuously.

Open http://127.0.0.1:8085. On a fresh database, the dashboard has no registered devices. Wait for the service-health indicator to show all core services available.

| Interface | URL |
| --- | --- |
| Network Lab | http://127.0.0.1:8085 |
| Grafana | http://127.0.0.1:3005 |
| Prometheus | http://127.0.0.1:9095 |
| Jaeger | http://127.0.0.1:16686 |

## 3. Complete a manual scenario

1. Click **Register a UE**.
2. Keep SUPI `imsi-001010000000001`, credential `demo-key` and gNB ID `gnb-lab-01`.
3. Click **Register device**. The dialog closes and a success message appears.
4. Open **UE devices**. The new device should be `REGISTERED`.
5. Click **Session** on that device. Open **PDU sessions** and confirm an `ACTIVE` session with a virtual IP address. The device becomes `SESSION_ACTIVE`.
6. Click **Packet** on the session, or open **Virtual traffic**, choose the active session and click **Send traffic**.
7. Check that packet and byte counters change. With nonzero configured loss, a packet may be intentionally lost.
8. In **PDU sessions**, click **Release**. The session becomes `RELEASED`; it can no longer send traffic.
9. In **UE devices**, click **Disconnect** to disconnect the device.

To run registration, session creation and a packet send automatically:

```powershell
python scripts/demo.py
```

The command prints JSON containing an accepted registration, a session and a packet result. Repeated runs create new synthetic UE identities rather than resetting the database.

## 4. Run the acceptance suite

Keep the lab running:

```powershell
python tests/e2e/acceptance.py
```

Expected result: six passing test groups and a final `OK`. The suite covers known/unknown/disabled subscribers, invalid credentials, duplicate requests, input validation, sessions, unique IPs, release, packets, UPF failures, authentication deadlines and background scenarios.

The suite creates synthetic records and changes the virtual traffic profile. Run it against this lab, not against an unrelated service on the same port.

Check the monitoring integrations separately:

```powershell
python tests/e2e/observability.py
```

This verifies actual Prometheus scrape targets, the provisioned Grafana dashboard and traces spanning services. It requires the complete Compose deployment.

## 5. Test a controlled failure

1. Register a device first.
2. In **Observability**, find `upf` and click **Inject fault**.
3. Try creating a new session for that device. The operation should fail in a controlled way, and the session must not become `ACTIVE`.
4. Return to **Observability** and click **Restore** on `upf`.
5. Allow the reconciliation cycle to confirm cleanup. Try a new session request.

The failed request can temporarily reserve an IP while cleanup is uncertain. Reconciliation releases it once route removal is confirmed. Details are in [the architecture decisions](adr/0002-persistence-and-sagas.md).

## 6. Run code and browser checks

With Go 1.25 installed:

```powershell
go test ./... -cover
go vet ./...
```

On a supported Go race-detector setup, also run `go test -race ./...`. On Windows without Go or a suitable native C compiler, use the Linux Docker helper:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/go.ps1 -Task test
powershell -ExecutionPolicy Bypass -File scripts/go.ps1 -Task vet
powershell -ExecutionPolicy Bypass -File scripts/go.ps1 -Task race
```

The PostgreSQL integration test only runs when `CORE5G_TEST_POSTGRES` points to a disposable test database. The local Compose database is isolated inside its network; it is not exposed on a host port. CI provisions a dedicated database, applies migrations and executes this integration test.

For browser checks, install Node.js, keep the lab running and run:

```powershell
npm ci
npx playwright install chromium
npm run test:browser
```

The script checks five views at four viewport sizes, English headings, registration, mobile navigation and keyboard skip-link behavior. It saves screenshots and results under `test-results/browser/`.

## 7. Measure load

Start with a small scenario:

```powershell
python tests/performance/load.py --count 10 --concurrency 2
```

Then increase the workload:

```powershell
python tests/performance/load.py --count 1000 --concurrency 50 --output test-results/performance.json
```

The script reports successes, failures, latency percentiles and environment details. A small-concurrency workload does not establish support for 1,000 simultaneous successful registrations. Current unmet targets and measured results are described in [the performance guide](performance.md).

## 8. Windows preview without Docker

This mode lets you try the application when Docker is unavailable. It uses in-memory data, which is lost on shutdown. It does not include PostgreSQL, Grafana, Prometheus or Jaeger.

1. Open the repository's [GitHub Actions page](https://github.com/ABDOUUisHERE/5G-Core-Network-Simulator/actions/workflows/ci.yml).
2. Select a successful run for the version you want to test.
3. Download the **core5g-windows** artifact. GitHub may require sign-in.
4. Extract `core5g.exe` into this repository's `bin` directory.
5. Start the preview:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/local-lab.ps1 -Binary .\bin\core5g.exe
```

Open http://127.0.0.1:8085 and follow the manual scenario above. You can also run `python scripts/demo.py` and the acceptance suite against the preview. This exercises the service workflow with in-memory storage; it does not validate PostgreSQL persistence or the monitoring integrations.

Stop the preview when finished:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/local-lab.ps1 -Stop
```

Do not run the preview and Compose simultaneously: both use port 8085. Preview service logs are stored in `test-results/local-logs/`.

## 9. Stop the complete lab

```powershell
docker compose down
```

This preserves PostgreSQL data. To deliberately reset this lab's database, use `docker compose down -v`, then start it again. This deletes its named volumes. Initialization migrations only run on a new data volume.

## Troubleshooting

| Symptom | Check or action |
| --- | --- |
| Docker commands hang or cannot reach the server | Start or repair Docker Desktop; confirm Linux containers and a successful `docker info` |
| Build reports no space or a read-only filesystem | Restore sufficient disk space and a healthy Docker engine before retrying |
| Port 8085 is already in use | Stop the Core5G preview if running; inspect the process using the port before changing anything else |
| Browser says the lab is not responding | Run `docker compose ps` and `docker compose logs --tail 100 control-api amf udm` |
| Services keep restarting | Inspect PostgreSQL startup with `docker compose logs --tail 100 postgres`; check whether a reused volume has the required schemas |
| `python` or `npm` is not recognized | Install Python 3 or Node.js and reopen the terminal; Node.js is optional unless running browser tests |
| Browser tests cannot find Chromium | Run `npx playwright install chromium` |
| Grafana, Prometheus or Jaeger is unavailable in the Windows preview | Use Compose for the complete observability stack |

For Kubernetes testing, follow the separate [Kind deployment guide](kubernetes.md). The full CI pipeline runs both Compose and Kind and retains their evidence.
