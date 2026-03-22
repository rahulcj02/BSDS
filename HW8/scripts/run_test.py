import json
import os
import random
import time
from datetime import datetime, timezone
from urllib import request


API_BASE = os.getenv("API_BASE", "http://localhost:8080").rstrip("/")
OUT_FILE = os.getenv("OUT_FILE", "results.json")


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
    cart_ids = []

    # 50 create cart
    for i in range(50):
        status, payload, ms, ok = http_request("POST", "/shopping-carts", {"customer_id": i + 1})
        cart_id = None
        if ok and status == 201:
            try:
                data = json.loads(payload)
                cart_id = data.get("shopping_cart_id")
            except Exception:
                cart_id = None
        if cart_id is not None:
            cart_ids.append(cart_id)
        results.append({
            "operation": "create_cart",
            "response_time": round(ms, 3),
            "success": ok and status == 201 and cart_id is not None,
            "status_code": status,
            "timestamp": now_iso()
        })

    # 50 add items
    for i in range(50):
        if not cart_ids:
            break
        cart_id = cart_ids[i % len(cart_ids)]
        product_id = 1000 + i
        quantity = random.randint(1, 5)
        status, _, ms, ok = http_request("POST", f"/shopping-carts/{cart_id}/items",
                                         {"product_id": product_id, "quantity": quantity})
        results.append({
            "operation": "add_items",
            "response_time": round(ms, 3),
            "success": ok and status == 204,
            "status_code": status,
            "timestamp": now_iso()
        })

    # 50 get cart
    for i in range(50):
        if not cart_ids:
            break
        cart_id = cart_ids[i % len(cart_ids)]
        status, _, ms, ok = http_request("GET", f"/shopping-carts/{cart_id}")
        results.append({
            "operation": "get_cart",
            "response_time": round(ms, 3),
            "success": ok and status == 200,
            "status_code": status,
            "timestamp": now_iso()
        })

    with open(OUT_FILE, "w", encoding="utf-8") as f:
        json.dump(results, f, indent=2)

    print(f"Wrote {len(results)} results to {OUT_FILE}")


if __name__ == "__main__":
    main()
