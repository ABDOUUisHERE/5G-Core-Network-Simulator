"""Run a real registration -> PDU session -> virtual packet demonstration."""
import json
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "tests/e2e"))
from acceptance import call

equipment = call("/ues", {"supi": "imsi-001010000000001", "credential": "demo-key"})
if not equipment["registration"].get("accepted"):
    raise SystemExit("Registration refused: " + equipment["registration"]["reason_code"])
session = call("/sessions", {"ue_id": equipment["ue_id"]}, expected=201)
packet = call("/packets", {"session_id": session["session_id"], "size": 512})
print(json.dumps({"equipment": equipment, "session": session, "packet": packet}, indent=2))
