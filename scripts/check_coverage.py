"""Enforce the CDC 80% statement-coverage floor for critical business packages."""
import sys
from pathlib import Path

packages = {name: [0, 0] for name in ["subscriber", "authentication", "registration", "session", "routing"]}
for line in Path(sys.argv[1] if len(sys.argv) > 1 else "coverage.out").read_text().splitlines()[1:]:
    location, statements, count = line.split()
    for name, totals in packages.items():
        if f"core5g/internal/{name}/" in location:
            totals[0] += int(statements)
            if int(count) > 0:
                totals[1] += int(statements)
failed = []
for name, (total, covered) in packages.items():
    percent = covered / total * 100 if total else 0
    print(f"{name}: {percent:.1f}% ({covered}/{total} statements)")
    if percent < 80:
        failed.append(name)
if failed:
    raise SystemExit("Coverage below 80%: " + ", ".join(failed))
