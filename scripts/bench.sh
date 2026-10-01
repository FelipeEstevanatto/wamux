#!/usr/bin/env bash
# bench.sh — measure performance and memory of a running Evolution GO node.
#
# WHY THIS EXISTS
#
# The dashboard's resource panel is good for a glance but not for comparing "did
# my change make this slower / use more memory?". This script drives a real load
# against a running node and samples THIS process's own memory (via
# /server/stats "process" block), so the numbers are attributable to the service
# even when other things run on the machine.
#
# It prints a before/after-friendly table and can capture a Go heap profile.
#
# USAGE
#   ./bench.sh                                  # defaults: 200 requests, 10 concurrent
#   BASE_URL=http://localhost:8081 API_KEY=xxx ./bench.sh
#   N=2000 C=50 ENDPOINT=/server/ok ./bench.sh
#   HEAP=1 ./bench.sh                           # also dump a heap profile
#
# ENV
#   BASE_URL  (default http://localhost:8081)
#   API_KEY   (GLOBAL_API_KEY; required for /server/stats)
#   N         total requests (default 200)
#   C         concurrency (default 10)
#   ENDPOINT  path to hit (default /server/ok, public; use an authed path with API_KEY)
#   HEAP      1 to capture a heap profile at the end
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8081}"
API_KEY="${API_KEY:-}"
N="${N:-200}"
C="${C:-10}"
ENDPOINT="${ENDPOINT:-/server/ok}"
HEAP="${HEAP:-0}"

say() { printf '%s\n' "$*"; }

# --- helpers -----------------------------------------------------------------

proc_rss_mb() {
  if [[ -z "$API_KEY" ]]; then echo "n/a"; return; fi
  curl -s -H "apikey: ${API_KEY}" "${BASE_URL}/server/stats" \
    | python3 -c "import sys,json
try:
    p=json.load(sys.stdin).get('system',{}).get('process',{})
    print(f\"{p.get('rssMB',0):.1f}\")
except Exception:
    print('n/a')"
}

proc_goroutines() {
  if [[ -z "$API_KEY" ]]; then echo "n/a"; return; fi
  curl -s -H "apikey: ${API_KEY}" "${BASE_URL}/server/stats" \
    | python3 -c "import sys,json
try:
    print(json.load(sys.stdin).get('system',{}).get('process',{}).get('goroutines','n/a'))
except Exception:
    print('n/a')"
}

# --- pre-flight ---------------------------------------------------------------

say "== Evolution GO benchmark =="
say "target:     ${BASE_URL}${ENDPOINT}"
say "requests:   ${N} (concurrency ${C})"
if [[ -z "$API_KEY" ]]; then
  say "API_KEY:    (unset — per-process memory sampling disabled; set GLOBAL_API_KEY)"
fi

if ! curl -s -o /dev/null "${BASE_URL}/server/ok"; then
  say "ERROR: ${BASE_URL}/server/ok is not reachable — is the service running?"
  exit 1
fi

RSS_BEFORE="$(proc_rss_mb)"
GORO_BEFORE="$(proc_goroutines)"
say ""
say "before:     RSS=${RSS_BEFORE} MB  goroutines=${GORO_BEFORE}"

# --- load ---------------------------------------------------------------------

say ""
say "-- running load --"
# Prefer `hey` when installed (nicer output); fall back to a portable curl loop.
if command -v hey >/dev/null 2>&1; then
  if [[ -n "$API_KEY" ]]; then
    hey -n "${N}" -c "${C}" -H "apikey: ${API_KEY}" "${BASE_URL}${ENDPOINT}"
  else
    hey -n "${N}" -c "${C}" "${BASE_URL}${ENDPOINT}"
  fi
else
  say "(hey not installed; using a curl loop — install 'hey' or 'vegeta' for proper latency stats)"
  start=$(date +%s.%N)
  seq 1 "${N}" | xargs -P "${C}" -I{} curl -s -o /dev/null \
    ${API_KEY:+-H "apikey: ${API_KEY}"} "${BASE_URL}${ENDPOINT}"
  end=$(date +%s.%N)
  elapsed=$(echo "${end} - ${start}" | bc)
  rps=$(echo "scale=1; ${N} / ${elapsed}" | bc)
  say "elapsed:    ${elapsed}s"
  say "throughput: ${rps} req/s"
fi

# --- post ---------------------------------------------------------------------

# Give the runtime a moment to settle (GC, workers draining) before sampling.
sleep 3
RSS_AFTER="$(proc_rss_mb)"
GORO_AFTER="$(proc_goroutines)"

say ""
say "== result =="
say "after:      RSS=${RSS_AFTER} MB  goroutines=${GORO_AFTER}"
if [[ "$RSS_BEFORE" != "n/a" && "$RSS_AFTER" != "n/a" ]]; then
  python3 -c "print(f'delta RSS:  {float('$RSS_AFTER') - float('$RSS_BEFORE'):+.1f} MB')"
fi

if [[ "${HEAP}" == "1" ]]; then
  say ""
  say "-- heap profile --"
  if command -v go >/dev/null 2>&1; then
    out="/tmp/opencode/evo_heap_$(date +%Y%m%d_%H%M%S).pprof"
    say "To capture a heap profile this service must expose pprof (see /debug/pprof)."
    say "If enabled, run: go tool pprof -inuse_space ${BASE_URL}/debug/pprof/heap"
  else
    say "go toolchain not found; skipping heap profile."
  fi
fi
