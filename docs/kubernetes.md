# Kind / Kubernetes

The manifest is generated from the actual migration and monitoring files to prevent duplicated configuration drift. It uses a dedicated `core5g` namespace, single-replica stateful services, a PostgreSQL PVC, probes, nonroot Go containers and local NodePorts.

Prerequisites: Docker, [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/) and `kubectl`. Use a dedicated local Kind context, not an existing production cluster.

```sh
python scripts/kubernetes.py --check
kind create cluster --name core5g-lab --config deployments/kubernetes/kind-config.yaml --wait 120s
docker build -t core5g:local -f deployments/docker/Dockerfile .
kind load docker-image core5g:local --name core5g-lab
kubectl --context kind-core5g-lab apply -f deployments/kubernetes/core5g.json
kubectl --context kind-core5g-lab rollout status deployment/control-api -n core5g --timeout=240s
```

PostgreSQL may need time to initialize. Services restart until their owning schema is reachable. `kubectl --context kind-core5g-lab get pods -n core5g` shows readiness. Go services deliberately have one replica and `Recreate` updates; active-active operation is not supported.

| Tool | Kind URL |
| --- | --- |
| Network Lab | http://127.0.0.1:8086 |
| Grafana | http://127.0.0.1:3006 |
| Prometheus | http://127.0.0.1:9096 |
| Jaeger | http://127.0.0.1:16687 |

Run acceptance with `CORE5G_URL=http://127.0.0.1:8086 python3 tests/e2e/acceptance.py`. In PowerShell, set `$env:CORE5G_URL='http://127.0.0.1:8086'` before the Python command, then remove that task-specific environment variable afterward if returning to Compose.

The cluster's PVC preserves data across pod restarts. Deleting the Kind cluster deliberately deletes the lab and its database. Use `kind delete cluster --name core5g-lab` only when finished with that lab.

After editing migrations or provisioned dashboards, run `python scripts/kubernetes.py` and commit the regenerated manifest. The CI drift check compares it byte-for-byte with the generator output. See [validation status](validation.md) for the actual Kind test outcome.
