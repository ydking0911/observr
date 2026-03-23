#!/usr/bin/env python3
"""
observr v0.1 end-to-end test script.

Verifies the full pipeline:
  1. Start observrd collector
  2. Run a FastAPI app instrumented with the Python SDK
  3. Send HTTP requests to the app
  4. Query the collector and assert events were captured

Prerequisites:
  - observrd binary built: cd server && go build ./cmd/observrd
  - Python SDK installed: pip install -e sdk/python
  - httpx installed: pip install httpx uvicorn fastapi

Usage:
  python scripts/test_e2e.py
"""

import json
import os
import subprocess
import sys
import time
import threading
import urllib.request
import urllib.error

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

COLLECTOR_PORT = 17676  # use non-default port so we don't conflict
APP_PORT = 18000
OBSERVRD_BIN = "./server/bin/observrd"
DB_PATH = "/tmp/observr-e2e-test.db"

GREEN = "\033[92m"
RED   = "\033[91m"
RESET = "\033[0m"
BOLD  = "\033[1m"


def ok(msg):  print(f"  {GREEN}✓{RESET} {msg}")
def fail(msg, detail=""): print(f"  {RED}✗{RESET} {msg}"); detail and print(f"    {detail}")
def header(msg): print(f"\n{BOLD}{msg}{RESET}")


# ── Helpers ───────────────────────────────────────────────────────────────

def wait_for_port(port, timeout=5.0):
    import socket
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.2):
                return True
        except OSError:
            time.sleep(0.1)
    return False


def query_collector(path="", level="", last=50):
    url = f"http://127.0.0.1:{COLLECTOR_PORT}/query?format=json&last={last}"
    if level:
        url += f"&level={level}"
    if path:
        url += f"&path={path}"
    try:
        with urllib.request.urlopen(url, timeout=3) as r:
            data = json.loads(r.read())
            return data if isinstance(data, list) else []
    except Exception as e:
        return []


def http_get(port, path):
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{port}{path}", timeout=3) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code
    except Exception:
        return None


# ── Test app ──────────────────────────────────────────────────────────────

APP_CODE = f"""
import sys
sys.path.insert(0, {repr(os.path.join(REPO_ROOT, "sdk", "python"))})

from fastapi import FastAPI
import uvicorn
import observr
observr.init(service="e2e-app", collector_url="http://127.0.0.1:{COLLECTOR_PORT}")

app = FastAPI()

@app.get("/ping")
async def ping():
    return {{"ok": True}}

@app.get("/slow")
async def slow():
    import asyncio
    await asyncio.sleep(0.1)
    return {{"result": "done"}}

@app.get("/error")
async def error():
    raise RuntimeError("intentional error")

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port={APP_PORT}, log_level="error")
"""


# ── Main ──────────────────────────────────────────────────────────────────

def main():
    import os, signal, atexit

    header("observr v0.1 End-to-End Test")
    passed = 0
    failed = 0

    # ── 1. Start observrd ─────────────────────────────────────────────
    header("1. Starting observrd collector")
    try:
        collector = subprocess.Popen(
            [OBSERVRD_BIN, f"--port={COLLECTOR_PORT}", f"--db={DB_PATH}", "--no-browser"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        atexit.register(collector.terminate)
    except FileNotFoundError:
        print(f"{RED}ERROR: {OBSERVRD_BIN} not found.{RESET}")
        print("  Build it first: cd server && go build -o bin/observrd ./cmd/observrd")
        sys.exit(1)

    if wait_for_port(COLLECTOR_PORT):
        ok(f"Collector running on :{COLLECTOR_PORT}")
        passed += 1
    else:
        fail("Collector did not start in time")
        failed += 1
        sys.exit(1)

    # ── 2. Start test app ─────────────────────────────────────────────
    header("2. Starting instrumented FastAPI app")
    app_file = "/tmp/observr_e2e_app.py"
    with open(app_file, "w") as f:
        f.write(APP_CODE)

    app_proc = subprocess.Popen(
        [sys.executable, app_file],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    atexit.register(app_proc.terminate)

    if wait_for_port(APP_PORT):
        ok(f"App running on :{APP_PORT}")
        passed += 1
    else:
        fail("App did not start in time")
        failed += 1
        sys.exit(1)

    # ── 3. Send requests ──────────────────────────────────────────────
    header("3. Sending requests to app")
    time.sleep(0.3)

    for _ in range(3):
        http_get(APP_PORT, "/ping")
    http_get(APP_PORT, "/slow")
    http_get(APP_PORT, "/error")   # 500
    http_get(APP_PORT, "/missing") # 404

    ok("Sent: 3× GET /ping, 1× GET /slow, 1× GET /error, 1× GET /missing")

    # Give transport time to flush
    time.sleep(1.5)

    # ── 4. Query and assert ───────────────────────────────────────────
    header("4. Querying collector")

    all_events = query_collector(last=100)
    if len(all_events) >= 5:
        ok(f"Total events captured: {len(all_events)}")
        passed += 1
    else:
        fail(f"Expected ≥5 events, got {len(all_events)}")
        failed += 1

    ping_events = query_collector(path="/ping")
    if len(ping_events) >= 3:
        ok(f"/ping traced {len(ping_events)} times")
        passed += 1
    else:
        fail(f"Expected ≥3 /ping events, got {len(ping_events)}")
        failed += 1

    error_events = query_collector(level="error")
    if len(error_events) >= 1:
        ok(f"Error events captured: {len(error_events)}")
        passed += 1
    else:
        fail("No error events captured")
        failed += 1

    slow_events = query_collector(path="/slow")
    if slow_events and slow_events[0].get("duration_ms", 0) >= 80:
        ok(f"/slow duration captured: {slow_events[0]['duration_ms']:.1f}ms")
        passed += 1
    else:
        dur = slow_events[0].get("duration_ms") if slow_events else "N/A"
        fail(f"/slow duration not captured correctly (got {dur}ms)")
        failed += 1

    # ── 5. Service name ───────────────────────────────────────────────
    if all_events and all(e.get("service") == "e2e-app" for e in all_events):
        ok("Service name 'e2e-app' set on all events")
        passed += 1
    else:
        fail("Service name mismatch on some events")
        failed += 1

    # ── Result ────────────────────────────────────────────────────────
    header("Result")
    total = passed + failed
    print(f"  {passed}/{total} checks passed")
    if failed == 0:
        print(f"\n{GREEN}{BOLD}All checks passed ✓{RESET}\n")
        sys.exit(0)
    else:
        print(f"\n{RED}{BOLD}{failed} check(s) failed ✗{RESET}\n")
        sys.exit(1)


if __name__ == "__main__":
    main()
