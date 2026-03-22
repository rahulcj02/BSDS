import json
import os
import time
from datetime import datetime, timezone
from urllib import request

API_BASE = os.getenv("API_BASE", "http://localhost:8080").rstrip("/")
OUT_FILE = os.getenv("OUT_FILE", "")
ITERATIONS = int(os.getenv("ITERATIONS", "10"))


def now_iso():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def http_request(method, path, body=None):
    url = f"{API_BASE}{path}"
    data = None
    headers = {"Content-Type": "application/json"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
    req = request.Request(url, data=data, headers=headers, method=method)
    start = time.perf_counter()
    try:
        with request.urlopen(req, timeout=10) as resp:
            status = resp.getcode()
            payload = resp.read().decode("utf-8")
            elapsed_ms = (time.perf_counter() - start) * 1000.0
            return status, payload, elapsed_ms, True
    except Exception as exc:
        elapsed_ms = (time.perf_counter() - start) * 1000.0
        status = getattr(exc, "code", 0)
        return status, "", elapsed_ms, False


def main():
    results = []
    create_miss = 0
    update_miss = 0

    for i in range(ITERATIONS):
        status, payload, ms, ok = http_request("POST", "/shopping-carts", {"customer_id": 1000 + i})
        cart_id = None
        if ok and status == 201:
            try:
                cart_id = json.loads(payload).get("shopping_cart_id")
            except Exception:
                cart_id = None
        results.append({
            "phase": "create",
            "status_code": status,
            "success": ok and status == 201 and cart_id is not None,
            "response_time": round(ms, 3),
            "timestamp": now_iso(),
        })
        if cart_id is None:
            create_miss += 1
            continue

        # immediate get
        status, _, ms, ok = http_request("GET", f"/shopping-carts/{cart_id}")
        ok_get = ok and status == 200
        results.append({
            "phase": "get_after_create",
            "status_code": status,
            "success": ok_get,
            "response_time": round(ms, 3),
            "timestamp": now_iso(),
        })
        if not ok_get:
            create_miss += 1

        # immediate update
        status, _, ms, ok = http_request("POST", f"/shopping-carts/{cart_id}/items",
                                         {"product_id": 9000 + i, "quantity": 1})
        ok_update = ok and status == 204
        results.append({
            "phase": "add_item",
            "status_code": status,
            "success": ok_update,
            "response_time": round(ms, 3),
            "timestamp": now_iso(),
        })
        if not ok_update:
            update_miss += 1
            continue

        # immediate get after update
        status, _, ms, ok = http_request("GET", f"/shopping-carts/{cart_id}")
        ok_get2 = ok and status == 200
        results.append({
            "phase": "get_after_add",
            "status_code": status,
            "success": ok_get2,
            "response_time": round(ms, 3),
            "timestamp": now_iso(),
        })
        if not ok_get2:
            update_miss += 1

    summary = {
        "iterations": ITERATIONS,
        "create_or_get_after_create_failures": create_miss,
        "add_or_get_after_add_failures": update_miss,
    }
    print("Consistency test summary:", json.dumps(summary))

    if OUT_FILE:
        with open(OUT_FILE, "w", encoding="utf-8") as f:
            json.dump({"summary": summary, "events": results}, f, indent=2)
        print(f"Wrote {len(results)} events to {OUT_FILE}")


if __name__ == "__main__":
    main()
