import json
import sys
from pathlib import Path


def load_json(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def main():
    if len(sys.argv) != 4:
        print("Usage: combine_results.py <mysql.json> <dynamodb.json> <combined.json>")
        sys.exit(1)

    mysql_path = Path(sys.argv[1])
    dynamo_path = Path(sys.argv[2])
    combined_path = Path(sys.argv[3])

    mysql = load_json(mysql_path)
    dynamo = load_json(dynamo_path)

    if len(mysql) != 150 or len(dynamo) != 150:
        raise SystemExit("Each input file must contain exactly 150 operations.")

    combined = []
    for item in mysql:
        item = dict(item)
        item["backend"] = "mysql"
        combined.append(item)
    for item in dynamo:
        item = dict(item)
        item["backend"] = "dynamodb"
        combined.append(item)

    with open(combined_path, "w", encoding="utf-8") as f:
        json.dump(combined, f, indent=2)

    print(f"Wrote {len(combined)} entries to {combined_path}")


if __name__ == "__main__":
    main()
