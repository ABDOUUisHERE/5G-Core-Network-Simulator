"""Generate committed Kubernetes manifests from the reference Compose configuration files."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
NS = "core5g"


def object_(kind, name, **fields):
    return {"apiVersion": "apps/v1" if kind in ["Deployment", "StatefulSet"] else "v1", "kind": kind, "metadata": {"name": name, "namespace": NS}, **fields}


def service(name, ports, node_port=None):
    entries = [{"name": key, "port": port, "targetPort": port} for key, port in ports.items()]
    if node_port:
        entries[0]["nodePort"] = node_port
    return object_("Service", name, spec={"selector": {"app": name}, "ports": entries, **({"type": "NodePort"} if node_port else {})})


def deployment(name, container, volumes=None, replicas=1):
    return object_("Deployment", name, spec={"replicas": replicas, "strategy": {"type": "Recreate"}, "selector": {"matchLabels": {"app": name}}, "template": {"metadata": {"labels": {"app": name}}, "spec": {"containers": [container], **({"volumes": volumes} if volumes else {})}}})


def generate():
    items = [{"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": NS}}]
    common = {"GRPC_ADDR": ":50051", "HTTP_ADDR": ":8080", "RPC_TIMEOUT": "2s", "REQUEST_TIMEOUT": "5s", "SEED_SUBSCRIBERS": "1000", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://jaeger:4318", "GRAFANA_URL": "http://127.0.0.1:3006", "PROMETHEUS_URL": "http://127.0.0.1:9096", "JAEGER_URL": "http://127.0.0.1:16687"}
    common.update({name.upper() + "_ADDR": name + ":50051" for name in ["udm", "ausf", "amf", "smf", "upf"]})
    common["GNB_ADDR"] = "gnb-simulator:50051"
    items.append(object_("ConfigMap", "core5g-config", data=common))
    items.append(object_("ConfigMap", "core5g-migrations", data={"001_initial.sql": (ROOT / "migrations/001_initial.sql").read_text(encoding="utf-8")}))
    items.append(object_("Secret", "core5g-local-lab", type="Opaque", stringData={"postgres-password": "local-lab-admin", **{name + "-url": f"postgres://core5g_{name}:local-lab-{name}@postgres:5432/core5g?sslmode=disable" for name in ["udm", "amf", "smf", "upf"]}}))
    items.append(object_("PersistentVolumeClaim", "postgres-data", spec={"accessModes": ["ReadWriteOnce"], "resources": {"requests": {"storage": "2Gi"}}}))
    postgres = {"name": "postgres", "image": "postgres:16-alpine", "env": [{"name": "POSTGRES_USER", "value": "core5g"}, {"name": "POSTGRES_DB", "value": "core5g"}, {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": "core5g-local-lab", "key": "postgres-password"}}}], "ports": [{"containerPort": 5432}], "readinessProbe": {"exec": {"command": ["pg_isready", "-U", "core5g"]}, "periodSeconds": 3}, "volumeMounts": [{"name": "data", "mountPath": "/var/lib/postgresql/data"}, {"name": "migrations", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True}], "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"memory": "512Mi"}}}
    items.extend([deployment("postgres", postgres, [{"name": "data", "persistentVolumeClaim": {"claimName": "postgres-data"}}, {"name": "migrations", "configMap": {"name": "core5g-migrations"}}]), service("postgres", {"postgres": 5432})])
    for role in ["udm", "ausf", "amf", "smf", "upf", "gnb-simulator", "control-api"]:
        env = [{"name": "SERVICE", "value": role}]
        if role in ["udm", "amf", "smf", "upf"]:
            env.append({"name": "DATABASE_URL", "valueFrom": {"secretKeyRef": {"name": "core5g-local-lab", "key": role + "-url"}}})
        container = {"name": role, "image": "core5g:local", "imagePullPolicy": "Never", "envFrom": [{"configMapRef": {"name": "core5g-config"}}], "env": env, "ports": [{"containerPort": 8080}, {"containerPort": 50051}], "securityContext": {"allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True, "runAsNonRoot": True, "capabilities": {"drop": ["ALL"]}}, "startupProbe": {"httpGet": {"path": "/healthz", "port": 8080}, "periodSeconds": 3, "failureThreshold": 60}, "readinessProbe": {"httpGet": {"path": "/readyz", "port": 8080}, "periodSeconds": 5}, "livenessProbe": {"httpGet": {"path": "/healthz", "port": 8080}, "periodSeconds": 10}, "resources": {"requests": {"cpu": "50m", "memory": "48Mi"}, "limits": {"cpu": "1", "memory": "192Mi"}}}
        items.extend([deployment(role, container), service(role, {"http": 8080, "grpc": 50051}, 30085 if role == "control-api" else None)])
    prometheus = (ROOT / "deployments/docker/prometheus.yml").read_text(encoding="utf-8")
    items.append(object_("ConfigMap", "prometheus-config", data={"prometheus.yml": prometheus}))
    items.extend([deployment("prometheus", {"name": "prometheus", "image": "prom/prometheus:v3.2.1", "ports": [{"containerPort": 9090}], "volumeMounts": [{"name": "config", "mountPath": "/etc/prometheus"}]}, [{"name": "config", "configMap": {"name": "prometheus-config"}}]), service("prometheus", {"http": 9090}, 30090)])
    items.append(object_("ConfigMap", "grafana-config", data={"datasource.yml": (ROOT / "deployments/docker/grafana/datasources/prometheus.yml").read_text(encoding="utf-8"), "provider.yml": (ROOT / "deployments/docker/grafana/dashboards/provider.yml").read_text(encoding="utf-8"), "core5g.json": (ROOT / "deployments/docker/grafana/dashboards/core5g.json").read_text(encoding="utf-8")}))
    mounts = [{"name": "config", "mountPath": "/etc/grafana/provisioning/datasources/prometheus.yml", "subPath": "datasource.yml"}, {"name": "config", "mountPath": "/etc/grafana/provisioning/dashboards/provider.yml", "subPath": "provider.yml"}, {"name": "config", "mountPath": "/etc/grafana/provisioning/dashboards/core5g.json", "subPath": "core5g.json"}]
    items.extend([deployment("grafana", {"name": "grafana", "image": "grafana/grafana:11.6.0", "ports": [{"containerPort": 3000}], "env": [{"name": "GF_AUTH_ANONYMOUS_ENABLED", "value": "true"}, {"name": "GF_AUTH_ANONYMOUS_ORG_ROLE", "value": "Viewer"}, {"name": "GF_SECURITY_ADMIN_PASSWORD", "value": "local-lab-grafana"}], "volumeMounts": mounts}, [{"name": "config", "configMap": {"name": "grafana-config"}}]), service("grafana", {"http": 3000}, 30030)])
    items.extend([deployment("jaeger", {"name": "jaeger", "image": "jaegertracing/all-in-one:1.66.0", "env": [{"name": "COLLECTOR_OTLP_ENABLED", "value": "true"}], "ports": [{"containerPort": port} for port in [16686, 4317, 4318]]}), service("jaeger", {"http": 16686, "otlp": 4318, "otlp-grpc": 4317}, 30686)])
    return json.dumps({"apiVersion": "v1", "kind": "List", "items": items}, indent=2) + "\n"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    target = ROOT / "deployments/kubernetes/core5g.json"
    content = generate()
    if args.check:
        if not target.exists() or target.read_text(encoding="utf-8") != content:
            raise SystemExit("Kubernetes manifests are stale: run python scripts/kubernetes.py")
        print("Kubernetes manifests are current")
    else:
        target.write_text(content, encoding="utf-8")
        print(target)
