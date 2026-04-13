#!/bin/bash
# ============================================================================
# Run this ON the EC2 instance after copying files there
# Usage: bash setup-and-run.sh
# ============================================================================

set -e

echo "=== Setting up Album Store ==="

# Check required env vars
for var in DATABASE_URL S3_BUCKET AWS_REGION; do
    if [ -z "${!var}" ]; then
        echo "ERROR: $var is not set. Export it first."
        echo ""
        echo "Example:"
        echo "  export DATABASE_URL='postgres://albumuser:PASSWORD@RDS_HOST:5432/albumstore?sslmode=disable'"
        echo "  export S3_BUCKET='your-bucket-name'"
        echo "  export AWS_REGION='us-west-2'"
        echo "  export PORT='8080'"
        exit 1
    fi
done

# Build
echo "[1/3] Building..."
export PATH=$PATH:/usr/local/go/bin
go mod tidy
go build -o album-store .
echo "   ✓ Built successfully"

# Kill any existing instance
echo "[2/3] Stopping any existing instance..."
pkill -f './album-store' 2>/dev/null || true
sleep 1

# Run
echo "[3/3] Starting server..."
nohup ./album-store > app.log 2>&1 &
sleep 2

# Verify
if curl -s http://localhost:${PORT:-8080}/health | grep -q '"ok"'; then
    echo "   ✓ Server is running!"
    echo ""
    echo "Test it: curl http://localhost:${PORT:-8080}/health"
    echo "Logs:    tail -f app.log"
else
    echo "   ✗ Server failed to start. Check app.log"
    tail -20 app.log
fi
