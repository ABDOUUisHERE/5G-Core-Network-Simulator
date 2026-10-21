"""Reproducible, measured REST-to-gRPC registration workload; no synthetic results."""
import argparse
import concurrent.futures
import datetime
import json
import os
import platform
import statistics
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def command(args):
    result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True, timeout=30)
    return result.stdout.strip() if result.returncode == 0 else None


def quantile(values, probability):
    if not values:
        return None
    values = sorted(values)
    return values[min(len(values) - 1, max(0, __import__('math').ceil(len(values) * probability) - 1))]


def memory():
    if os.name == "nt":
        import ctypes
        class Memory(ctypes.Structure):
            _fields_ = [("length", ctypes.c_ulong), ("load", ctypes.c_ulong)] + [(name, ctypes.c_ulonglong) for name in ["total", "available", "page", "free_page", "virtual", "free_virtual", "extended"]]
        status = Memory()
        status.length = ctypes.sizeof(status)
        ctypes.windll.kernel32.GlobalMemoryStatusEx(ctypes.byref(status))
        return status.total
    try:
        return os.sysconf("SC_PAGE_SIZE") * os.sysconf("SC_PHYS_PAGES")
    except (ValueError, AttributeError):
        return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="http://127.0.0.1:8085")
    parser.add_argument("--count", type=int, default=1000)
    parser.add_argument("--concurrency", type=int, default=50)
    parser.add_argument("--iterations", type=int, default=1)
    parser.add_argument("--output", default="test-results/performance.json")
    args = parser.parse_args()
    if not 1 <= args.count <= 1000 or not 1 <= args.concurrency <= 1000 or args.iterations < 1:
        parser.error("count/concurrency must be 1..1000; iterations must be positive")
    container_ids = (command(["docker", "compose", "ps", "-q"]) or "").splitlines()
    samples, stop = [], threading.Event()
    def monitor():
        while not stop.is_set():
            if container_ids:
                raw = command(["docker", "stats", "--no-stream", "--format", "{{json .}}", *container_ids])
                if raw:
                    samples.append({"at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "containers": [json.loads(line) for line in raw.splitlines()]})
            stop.wait(2)
    thread = threading.Thread(target=monitor, daemon=True)
    thread.start()
    def register(index):
        payload = json.dumps({"supi": f"imsi-00101{index:010d}", "credential": "demo-key", "ue_id": str(uuid.uuid4()), "request_id": str(uuid.uuid4())}).encode()
        request = urllib.request.Request(args.base + "/api/ues", data=payload, headers={"Content-Type": "application/json"})
        started = time.perf_counter()
        error = None
        try:
            with urllib.request.urlopen(request, timeout=12) as response:
                data = json.load(response)
                if not data["registration"].get("accepted"):
                    error = data["registration"].get("reason_code", "refused")
        except Exception as failure:
            error = str(failure)
        return {"latency_ms": (time.perf_counter() - started) * 1000, "error": error}
    iterations = []
    try:
        for iteration in range(args.iterations):
            start = time.perf_counter()
            with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as executor:
                results = list(executor.map(register, range(1, args.count + 1)))
            elapsed = time.perf_counter() - start
            successes = [result["latency_ms"] for result in results if result["error"] is None]
            all_latencies = [result["latency_ms"] for result in results]
            iterations.append({"iteration": iteration + 1, "requests": args.count, "concurrency": args.concurrency, "successes": len(successes), "errors": args.count - len(successes), "error_samples": [result["error"] for result in results if result["error"]][:10], "wall_seconds": elapsed, "throughput_per_second": args.count / elapsed, "success_p50_ms": quantile(successes, .5), "success_p95_ms": quantile(successes, .95), "success_p99_ms": quantile(successes, .99), "all_p50_ms": quantile(all_latencies, .5), "all_p95_ms": quantile(all_latencies, .95), "all_p99_ms": quantile(all_latencies, .99)})
    finally:
        stop.set()
        thread.join(timeout=35)
    docker_info = command(["docker", "info", "--format", "{{json .}}"])
    docker = json.loads(docker_info) if docker_info else {}
    report = {"timestamp_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(), "git_commit": command(["git", "rev-parse", "HEAD"]), "working_tree_dirty": bool(command(["git", "status", "--porcelain"])), "target": args.base, "host": {"os": platform.platform(), "cpu": platform.processor(), "logical_cpus": os.cpu_count(), "ram_bytes": memory()}, "docker": {key: docker.get(key) for key in ["ServerVersion", "NCPU", "MemTotal", "OperatingSystem", "KernelVersion"]}, "workload": "HTTP control API -> gNB -> AMF -> AUSF -> UDM -> PostgreSQL, synthetic subscribers; new UE UUID per request; persistent pre-existing dataset retained; OTLP/metrics enabled", "warmup": "No explicit warm-up; previous acceptance/demo requests may have warmed the stack.", "iterations": iterations, "resource_samples": samples, "limits": ["Client-observed successful-request and all-request percentiles are reported separately.", "The CDC sub-100-ms p95 is a target, not a guaranteed result.", "CPU values are Docker sampled percentages, not isolated per-request CPU time.", "This run does not prove active-active service scalability."]}
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"output": str(output), "iterations": iterations, "resource_samples": len(samples)}, indent=2))


if __name__ == "__main__":
    main()
