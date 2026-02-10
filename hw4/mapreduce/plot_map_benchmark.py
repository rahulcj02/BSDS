import argparse
import csv
import os
import statistics
from typing import List, Tuple

import matplotlib.pyplot as plt


def read_csv(path: str) -> Tuple[List[int], List[int]]:
    seq: List[int] = []
    par: List[int] = []
    with open(path, newline="") as f:
        reader = csv.DictReader(f)
        required = {"sequential_ms", "parallel_ms"}
        if reader.fieldnames is None or not required.issubset(set(reader.fieldnames)):
            raise ValueError(
                f"CSV must have columns {sorted(required)}. Found: {reader.fieldnames}"
            )
        for row in reader:
            seq.append(int(row["sequential_ms"]))
            par.append(int(row["parallel_ms"]))
    if not seq or not par:
        raise ValueError("CSV has no rows.")
    return seq, par


def safe_stdev(xs: List[int]) -> float:
    return statistics.stdev(xs) if len(xs) >= 2 else 0.0


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--csv", required=True, help="mapreduce/plot_map_benchmark.py")
    ap.add_argument("--out", default="map_benchmark_plot.png", help="Output PNG path")
    args = ap.parse_args()

    seq, par = read_csv(args.csv)

    avg_seq = statistics.mean(seq)
    avg_par = statistics.mean(par)
    sd_seq = safe_stdev(seq)
    sd_par = safe_stdev(par)
    speedup = (avg_seq / avg_par) if avg_par != 0 else float("inf")

    labels = ["Sequential", "Parallel"]
    means = [avg_seq, avg_par]
    errs = [sd_seq, sd_par]

    plt.figure(figsize=(6, 4))
    plt.bar(labels, means, yerr=errs, capsize=6)
    plt.ylabel("Time (ms)")
    plt.title("Map stage: Sequential vs Parallel (avg ± stdev)")
    plt.tight_layout()

    out_png = args.out
    plt.savefig(out_png, dpi=200)
    plt.close()

    out_txt = os.path.splitext(out_png)[0] + "_summary.txt"
    with open(out_txt, "w") as f:
        f.write(f"avg_sequential_ms={avg_seq:.2f}\n")
        f.write(f"stdev_sequential_ms={sd_seq:.2f}\n")
        f.write(f"avg_parallel_ms={avg_par:.2f}\n")
        f.write(f"stdev_parallel_ms={sd_par:.2f}\n")
        f.write(f"speedup={speedup:.2f}x\n")
        f.write(f"runs={len(seq)}\n")

    print("Saved:")
    print(f"  Plot:    {out_png}")
    print(f"  Summary: {out_txt}")
    print("\nNumbers:")
    print(f"  avg_sequential_ms = {avg_seq:.2f} (stdev {sd_seq:.2f})")
    print(f"  avg_parallel_ms   = {avg_par:.2f} (stdev {sd_par:.2f})")
    print(f"  speedup           = {speedup:.2f}x")


if __name__ == "__main__":
    main()
