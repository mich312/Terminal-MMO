#!/usr/bin/env bash
# Deploy durstworld (Terminal-MMO). Runs ON THE SERVER — the GitHub Actions
# workflow (.github/workflows/deploy.yml) pipes this in over SSH.
#
# Build-on-server (Go, CGO-free static binary): pull main, rebuild, and
# health-check the web listener's own loopback /healthz — so a deploy succeeds
# even before DNS/cert for mmo.mich312.com is live (the edge proxy is verified
# separately). Rolls back on failure; a failed build leaves the old container
# running. The SSH host key and SQLite world live on ./.ssh and ./data volumes
# and are untouched by a rollback.
set -euo pipefail

REPO_DIR="$HOME/Terminal-MMO"
URL="http://127.0.0.1:8080/healthz"   # container's loopback web publish
cd "$REPO_DIR"

compose() { docker compose -f docker-compose.yml -f docker-compose.edge.yml "$@"; }

healthy() {
  sleep 3
  for _ in $(seq 1 18); do   # ~90s
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 "$URL") || code=000
    echo "  $URL -> $code"
    case "$code" in 200) return 0 ;; esac
    sleep 5
  done
  return 1
}

PREV=$(git rev-parse HEAD)
git fetch origin --quiet
git reset --hard origin/main
NEW=$(git rev-parse HEAD)
echo "deploying ${PREV:0:8} -> ${NEW:0:8}"

compose up -d --build

echo "health-checking $URL ..."
if healthy; then
  echo "✅ deploy OK: ${NEW:0:8}"
  docker image prune -f >/dev/null 2>&1 || true   # keep the disk from creeping up
  exit 0
fi

echo "❌ unhealthy — rolling back to ${PREV:0:8}"
git reset --hard "$PREV"
compose up -d --build
if healthy; then echo "rolled back to ${PREV:0:8}"; else echo "rollback ALSO unhealthy — needs a look"; fi
exit 1
