"""Verify actual provisioned metrics, dashboard and distributed traces."""
import argparse
import json
import time
import urllib.request


def get(url):
    with urllib.request.urlopen(url, timeout=5) as response:
        return json.load(response)


def eventually(name, check):
    deadline = time.monotonic() + 45
    last_error = None
    while time.monotonic() < deadline:
        try:
            result = check()
            if result:
                print(json.dumps({"check": name, "result": result}))
                return
        except Exception as error:
            last_error = str(error)
        time.sleep(1)
    raise SystemExit(f"{name} did not become ready: {last_error}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profile", choices=["compose", "kind"], default="compose")
    args = parser.parse_args()
    grafana_port, prometheus_port, jaeger_port = (3005, 9095, 16686) if args.profile == "compose" else (3006, 9096, 16687)
    def metrics():
        up = get(f"http://127.0.0.1:{prometheus_port}/api/v1/query?query=up")
        targets = [result for result in up["data"]["result"] if result["metric"].get("job") == "core5g"]
        if len(targets) != 7 or any(result["value"][1] != "1" for result in targets):
            return None
        registered = get(f"http://127.0.0.1:{prometheus_port}/api/v1/query?query=core5g_registered_ues")
        values = registered["data"]["result"]
        return {"targets": len(targets), "registered_ues": float(values[0]["value"][1])} if values and float(values[0]["value"][1]) > 0 else None
    def dashboard():
        result = get(f"http://127.0.0.1:{grafana_port}/api/dashboards/uid/core5g-overview")
        panels = result["dashboard"]["panels"]
        return {"uid": result["dashboard"]["uid"], "panels": len(panels)} if len(panels) == 8 else None
    def traces():
        result = get(f"http://127.0.0.1:{jaeger_port}/api/traces?service=control-api&limit=10")
        for trace in result.get("data", []):
            services = sorted({process["serviceName"] for process in trace["processes"].values()})
            if len(services) >= 3:
                return {"trace_id": trace["traceID"], "services": services, "spans": len(trace["spans"])}
        return None
    eventually("prometheus", metrics)
    eventually("grafana", dashboard)
    eventually("distributed_trace", traces)


if __name__ == "__main__":
    main()
