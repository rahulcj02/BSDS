import json
import sys
from pathlib import Path


def percentile(sorted_vals, p):
    if not sorted_vals:
        return None
    k = (len(sorted_vals) - 1) * (p / 100.0)
    f = int(k)
    c = min(f + 1, len(sorted_vals) - 1)
    if f == c:
        return sorted_vals[f]
    return sorted_vals[f] + (sorted_vals[c] - sorted_vals[f]) * (k - f)


def compute_stats(items):
    times = [x["response_time"] for x in items]
    times.sort()
    success = [x for x in items if x.get("success")]
    total = len(items)
    ok = len(success)
    return {
        "count": total,
        "avg": round(sum(times) / total, 3) if total else 0,
        "p50": round(percentile(times, 50), 3) if total else 0,
        "p95": round(percentile(times, 95), 3) if total else 0,
        "p99": round(percentile(times, 99), 3) if total else 0,
        "success_rate": round((ok / total) * 100.0, 2) if total else 0,
    }


def main():
    if len(sys.argv) != 3:
        print("Usage: analyze_results.py <combined.json> <analysis_summary.json>")
        sys.exit(1)

    combined_path = Path(sys.argv[1])
    out_path = Path(sys.argv[2])

    data = json.loads(combined_path.read_text())
    mysql = [x for x in data if x.get("backend") == "mysql"]
    dynamo = [x for x in data if x.get("backend") == "dynamodb"]

    summary = {
        "mysql": compute_stats(mysql),
        "dynamodb": compute_stats(dynamo),
        "by_operation": {
            "mysql": {},
            "dynamodb": {},
        },
    }

    for backend, items in [("mysql", mysql), ("dynamodb", dynamo)]:
        for op in ["create_cart", "add_items", "get_cart"]:
            summary["by_operation"][backend][op] = compute_stats([x for x in items if x["operation"] == op])

    out_path.write_text(json.dumps(summary, indent=2))
    print(f"Wrote {out_path}")


if __name__ == "__main__":
    main()
