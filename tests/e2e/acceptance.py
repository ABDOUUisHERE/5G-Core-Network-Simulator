"""CDC T01-T09 against the actual REST -> gNB -> gRPC -> PostgreSQL stack."""
import json
import os
import time
import unittest
import urllib.error
import urllib.request
import uuid

BASE = os.environ.get("CORE5G_URL", "http://127.0.0.1:8085")


def call(path, payload=None, expected=200):
    req = urllib.request.Request(BASE + "/api" + path, method="GET" if payload is None else "POST",
        data=None if payload is None else json.dumps(payload).encode(), headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=12) as response:
            code, result = response.status, json.load(response)
    except urllib.error.HTTPError as error:
        code, result = error.code, json.load(error)
    if code != expected:
        raise AssertionError(f"{path}: expected HTTP {expected}, got {code}: {result}")
    return result


class Acceptance(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        for service in ["udm", "ausf", "amf", "smf", "upf", "gnb-simulator"]:
            call(f"/faults/{service}", {"enabled": False, "delay_ms": 0})
        call("/traffic/config", {"latency_ms": 0, "jitter_ms": 0, "loss_percent": 0, "seed": 42})

    def register(self, supi="imsi-001010000000001", credential="demo-key", **extra):
        return call("/ues", dict(supi=supi, credential=credential, **extra))

    def test_01_known_unknown_disabled_and_bad_credentials(self):
        self.assertTrue(self.register()["registration"]["accepted"])
        self.assertEqual(self.register("imsi-001019999999999")["registration"]["reason_code"], "UNKNOWN_SUBSCRIBER")
        self.assertEqual(self.register(credential="incorrect")["registration"]["reason_code"], "INVALID_CREDENTIALS")
        supi = "imsi-001018888888888"
        call("/subscribers", {"supi": supi, "status": "DISABLED", "auth_profile": "demo-key"})
        self.assertEqual(self.register(supi)["registration"]["reason_code"], "DISABLED_SUBSCRIBER")

    def test_02_request_replay_and_input_validation(self):
        request_id, ue_id = str(uuid.uuid4()), str(uuid.uuid4())
        one = self.register(request_id=request_id, ue_id=ue_id)
        two = self.register(request_id=request_id, ue_id=ue_id)
        self.assertEqual(one["registration"], two["registration"])
        call("/ues", {"supi": "invalid", "credential": "demo-key"}, expected=400)

    def test_03_sessions_packets_release_and_unique_ip(self):
        ue = self.register()["ue_id"]
        request = {"ue_id": ue, "request_id": str(uuid.uuid4())}
        one = call("/sessions", request, 201)
        repeat = call("/sessions", request, 201)
        self.assertEqual(one["session_id"], repeat["session_id"])
        two = call("/sessions", {"ue_id": ue}, 201)
        self.assertNotEqual(one["ip_address"], two["ip_address"])
        packet = call("/packets", {"session_id": one["session_id"], "size": 512})
        self.assertTrue(packet.get("received"))
        self.assertGreaterEqual(packet["counters"]["packets_sent"], 1)
        path = f"/sessions/{one['session_id']}/release"
        self.assertEqual(call(path, {})["state"], "RELEASED")
        self.assertEqual(call(path, {})["state"], "RELEASED")
        call("/packets", {"session_id": one["session_id"], "size": 512}, expected=409)
        call(f"/ues/{ue}/deregister", {})
        call("/sessions", {"ue_id": ue}, expected=409)

    def test_04_upf_failure_and_eventual_compensation(self):
        ue = self.register()["ue_id"]
        call("/faults/upf", {"enabled": True})
        try:
            call("/sessions", {"ue_id": ue}, expected=503)
            sessions = call(f"/sessions?ue_id={ue}")["sessions"]
            self.assertTrue(sessions)
            self.assertTrue(all(item["state"] != "ACTIVE" for item in sessions))
        finally:
            call("/faults/upf", {"enabled": False})
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            sessions = call(f"/sessions?ue_id={ue}")["sessions"]
            if all(item["state"] == "FAILED" and not item.get("ip_address") for item in sessions):
                break
            time.sleep(.5)
        else:
            self.fail("unconfirmed cleanup did not converge after UPF restoration")

    def test_05_registration_timeout_fails_closed(self):
        ue = str(uuid.uuid4())
        call("/faults/ausf", {"enabled": False, "delay_ms": 2500})
        try:
            call("/ues", {"supi": "imsi-001010000000001", "credential": "demo-key", "ue_id": ue}, expected=504)
        finally:
            call("/faults/ausf", {"enabled": False, "delay_ms": 0})
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            equipments = call("/ues")["equipments"]
            item = next((item for item in equipments if item["ue_id"] == ue), None)
            if item and item["state"] == "DISCONNECTED":
                break
            time.sleep(.2)
        else:
            self.fail("UE did not return to DISCONNECTED")

    def test_06_seeded_loss_and_background_job(self):
        ue = self.register()["ue_id"]
        session = call("/sessions", {"ue_id": ue}, 201)
        call("/traffic/config", {"latency_ms": 0, "jitter_ms": 0, "loss_percent": 100, "seed": 77})
        try:
            self.assertTrue(call("/packets", {"session_id": session["session_id"], "size": 128})["lost"])
        finally:
            call("/traffic/config", {"latency_ms": 2, "jitter_ms": 1, "loss_percent": 1, "seed": 42})
        job = call("/simulations", {"count": 10, "concurrency": 5, "full": True}, expected=202)
        deadline = time.monotonic() + 40
        while time.monotonic() < deadline:
            current = call(f"/simulations/{job['id']}")
            if current["state"] == "COMPLETED":
                self.assertEqual(current["succeeded"], 10, current)
                break
            time.sleep(.5)
        else:
            self.fail("background simulation did not finish")


if __name__ == "__main__":
    unittest.main(verbosity=2)
