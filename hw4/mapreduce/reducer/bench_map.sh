set -e

RUNS=10
OUTFILE="map_benchmark.csv"

ms() {
python3 - <<'PY'
import time
print(int(time.time()*1000))
PY
}

echo "run,sequential_ms,parallel_ms" > "$OUTFILE"

SPLIT_JSON=$(curl -s "http://$SPLITTER_IP:8080/split?input=s3://$BUCKET/input/hamlet.txt")

C1=$(echo "$SPLIT_JSON" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["chunks"][0])')
C2=$(echo "$SPLIT_JSON" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["chunks"][1])')
C3=$(echo "$SPLIT_JSON" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["chunks"][2])')

check200() {
  url="$1"
  code=$(curl -s -o /dev/null -w "%{http_code}" "$url")
  if [ "$code" != "200" ]; then
    echo "FAILED ($code): $url"
    exit 1
  fi
}

check200 "http://$M1_IP:8080/map?input=$C1"
check200 "http://$M2_IP:8080/map?input=$C2"
check200 "http://$M3_IP:8080/map?input=$C3"

for i in $(seq 1 $RUNS); do
  s0=$(ms)
  curl -s -o /dev/null "http://$M1_IP:8080/map?input=$C1"
  curl -s -o /dev/null "http://$M2_IP:8080/map?input=$C2"
  curl -s -o /dev/null "http://$M3_IP:8080/map?input=$C3"
  s1=$(ms)
  seq_ms=$((s1-s0))

  p0=$(ms)
  curl -s -o /dev/null "http://$M1_IP:8080/map?input=$C1" & p1=$!
  curl -s -o /dev/null "http://$M2_IP:8080/map?input=$C2" & p2=$!
  curl -s -o /dev/null "http://$M3_IP:8080/map?input=$C3" & p3=$!
  wait $p1 $p2 $p3
  p1t=$(ms)
  par_ms=$((p1t-p0))

  echo "$i,$seq_ms,$par_ms" >> "$OUTFILE"
  echo "run $i: sequential_ms=$seq_ms parallel_ms=$par_ms"
done

python3 - <<'PY'
import csv, statistics
rows=list(csv.DictReader(open("map_benchmark.csv")))
seq=[int(r["sequential_ms"]) for r in rows]
par=[int(r["parallel_ms"]) for r in rows]
avg_seq=sum(seq)/len(seq)
avg_par=sum(par)/len(par)
print("\nAverages:")
print(f"avg_sequential_ms = {avg_seq:.2f}")
print(f"avg_parallel_ms   = {avg_par:.2f}")
print(f"speedup = {avg_seq/avg_par:.2f}x")
PY
