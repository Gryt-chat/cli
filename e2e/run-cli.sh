#!/usr/bin/env bash
# End-to-end check for the gryt CLI, from nothing: build it, create and start a server
# without a terminal, have a Playwright guest chat and upload, pull, then remove.
#
# GRYT-1565. Ported from the throwaway harness in GRYT-1506, which drove the wizard
# through tmux — gryt create and gryt start (GRYT-1567) replace that entirely.
set -uo pipefail
source "$(dirname "$0")/lib.sh"
# start_web_client runs a container this script doesn't otherwise own; take it down on
# any exit, not only a clean one, or a failed run leaves it behind.
trap 'docker rm -f e2e-web >/dev/null 2>&1 || true' EXIT
BIN_DIR=${BIN_DIR:-$HOME/.local/bin}
mkdir -p "$BIN_DIR"
export PATH="$BIN_DIR:$PATH"
CFG=${GRYT_CONFIG_DIR:-$HOME/.config/gryt}
SERVER=e2e
# Read after create rather than assumed: something else on the machine may already
# hold the port gryt would otherwise offer first.
A=127.0.0.1:0

s1_install() {
  go build -o "$BIN_DIR/gryt" ./cmd/gryt || return 1
  command -v gryt || { echo "gryt is not on PATH after the build"; return 1; }
  gryt version
  gryt doctor
}

s2_create() {
  # community: local identities, so a guest with no Gryt account can join.
  gryt create --name "$SERVER" --security community --yes || return 1
  gryt list
}

s2_start() {
  gryt start "$SERVER" || return 1
  wait_http "http://$A/health" 120 || { docker ps -a; return 1; }
}

s2_containers_healthy() {
  local ok=0
  wait_healthy "gryt-$SERVER" 120 || ok=1
  wait_healthy "gryt-$SERVER-image-worker" 60 || ok=1
  wait_healthy gryt-sfu 120 || ok=1
  docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
  return $ok
}

s2_open_joins() {
  # "Who can join" lives in the server's own database rather than a create flag,
  # reached through the management API gryt create/start already set up a token for.
  local port token
  port=$(jq -r '.adminPort' "$CFG/servers/$SERVER/profile.json")
  token=$(sed -n 's/^GRYT_ADMIN_TOKEN=//p' "$CFG/servers/$SERVER/admin.env")
  [[ -n "$port" && "$port" != null && -n "$token" ]] || { echo "no admin port or token in the profile"; return 1; }
  curl -sf -X PATCH "http://127.0.0.1:$port/management/settings" \
    -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    -d '{"joinPolicy":"open"}' | tee /tmp/settings.json
  grep -q '"joinPolicy":"open"' /tmp/settings.json
}

s3_client_chat() { client chat "$A"; }

s4_pull() { gryt pull "$SERVER" && wait_healthy "gryt-$SERVER" 120; }

s5_remove() {
  gryt remove "$SERVER" --yes || return 1
  gryt list
  [[ ! -e "$CFG/servers/$SERVER" ]] || { echo "servers/$SERVER is still there"; return 1; }
}

s5_leftovers() {
  echo "--- our containers"; docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}' | grep -E "^gryt-$SERVER(-|\$)|^gryt-sfu\$" || true
  echo "--- networks"; docker network ls --format '{{.Name}}' | grep -vE '^(bridge|host|none)$' || true
  echo "--- files"; find "$CFG" -maxdepth 3 2>/dev/null || true
  local left=0
  # Scoped to this run's own names: a shared machine may already have unrelated
  # gryt-* containers, which a clean CI runner never does.
  docker ps -a --format '{{.Names}}' | grep -E "^gryt-$SERVER(-|\$)|^gryt-sfu\$" && left=1
  docker network ls --format '{{.Name}}' | grep -qx gryt && { echo "network gryt left"; left=1; }
  [[ -d "$CFG/servers" ]] && [[ -n "$(ls -A "$CFG/servers" 2>/dev/null)" ]] && { echo "server folders left"; left=1; }
  return $left
}

run_step "1 install gryt (source build)" s1_install || { summary; exit 1; }
run_step "2 create (non-interactive)" s2_create
A="127.0.0.1:$(jq -r '.port // 0' "$CFG/servers/$SERVER/profile.json" 2>/dev/null)"
run_step "2 start" s2_start
run_step "2 containers healthy" s2_containers_healthy
run_step "2 who can join -> open" s2_open_joins
start_web_client
run_step "3 client: join, message, upload" s3_client_chat
run_step "4 gryt pull" s4_pull
run_step "5 remove" s5_remove
run_step "5 nothing left behind" s5_leftovers
summary
grep -q '^FAIL' "$RESULTS" && exit 1 || exit 0
