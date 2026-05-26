#!/usr/bin/env bash
# Realistic-lifecycle coordstore soak launcher (ga-w08fz workload).
# Driven by env: COORDSTORE_RESULTS_DIR (required), COORDSTORE_SOAK_DURATION (default 2h),
# COORDSTORE_DOLT_DSN (optional — set ONLY to an isolated throwaway dolt, never :28232).
set -uo pipefail
export GOPATH=/home/jaword/mayor-claude/go
export GOCACHE=/home/jaword/mayor-claude/.cache/go-build
export GOMODCACHE=/home/jaword/mayor-claude/go/pkg/mod
export GOROOT=/usr/lib/golang
export PATH="$GOROOT/bin:/usr/lib64/ccache:/usr/bin:/bin:$PATH"
export CGO_ENABLED=1
export COORDSTORE_SOAK=1
export COORDSTORE_FULL_MATRIX=1
export COORDSTORE_SQLITE_CGO=1
export COORDSTORE_SOAK_DURATION="${COORDSTORE_SOAK_DURATION:-2h}"
: "${COORDSTORE_RESULTS_DIR:?must set COORDSTORE_RESULTS_DIR}"
mkdir -p "$COORDSTORE_RESULTS_DIR"
cd /var/tmp/coordstore-soak-wt || exit 1
{
  echo "soak_launch=$(date -u +%FT%TZ)"
  echo "duration_per_backend=$COORDSTORE_SOAK_DURATION"
  echo "results=$COORDSTORE_RESULTS_DIR"
  echo "dolt_dsn=${COORDSTORE_DOLT_DSN:-<none — dolt leg skipped>}"
  echo "branch_commit=$(git rev-parse HEAD 2>/dev/null)"
} > "$COORDSTORE_RESULTS_DIR/launch.txt"
exec go test -tags sqlite_cgo -count=1 ./internal/benchmarks/coordstore/ -run TestBenchmarkSoakFullMatrix -timeout 0 -v \
  > "$COORDSTORE_RESULTS_DIR/workflow.log" 2>&1
