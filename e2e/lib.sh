# Shared by every e2e script: a step runner that writes one row per check plus a
# GitHub Actions step summary, and small waiters around curl and docker inspect.
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
OUT=${OUT:-$HERE/out}
mkdir -p "$OUT"
RESULTS=$OUT/results.tsv
: >"$RESULTS"
N=0
WEB_PORT=15738

record() { # status name seconds detail
  printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" >>"$RESULTS"
  printf '%-4s %-60s %6ss  %s\n' "$1" "$2" "$3" "$4"
}

# run_step "name" command... : output to its own log, a row in results.tsv.
run_step() {
  local name=$1; shift
  N=$((N + 1))
  local log; log=$(printf '%s/%02d-%s.log' "$OUT" "$N" "$(echo "$name" | tr -c 'a-zA-Z0-9\n' '-' | cut -c1-40)")
  local start=$SECONDS
  echo "::group::$name"
  "$@" > >(tee "$log") 2>&1
  local rc=$?
  sleep 0.2
  echo "::endgroup::"
  local secs=$((SECONDS - start))
  # client.mjs prints its own RESULT rows; keep them as sub-steps.
  grep -a '^RESULT' "$log" | while IFS=$'\t' read -r _ st sub t detail; do
    record "$st" "  $sub" "${t%s}" "$detail"
  done
  if [[ $rc -eq 0 ]]; then
    record PASS "$name" "$secs" ""
  else
    record FAIL "$name" "$secs" "$(grep -av '^RESULT' "$log" | tail -3 | tr '\n' ' ' | cut -c1-300)"
  fi
  return $rc
}

wait_http() { # url seconds
  local i
  for ((i = 0; i < $2; i++)); do curl -sf "$1" >/dev/null && return 0; sleep 1; done
  echo "no answer from $1 after $2s"; return 1
}

wait_healthy() { # container seconds
  local i s
  for ((i = 0; i < $2; i++)); do
    s=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$1" 2>/dev/null)
    [[ "$s" == healthy || "$s" == running ]] && { echo "$1: $s"; return 0; }
    sleep 1
  done
  echo "$1 is ${s:-missing} after $2s"; docker logs --tail 30 "$1" 2>&1; return 1
}

start_web_client() {
  # The released web client over plain http. app.gryt.chat only ever dials https,
  # and 127.0.0.1:$WEB_PORT is on the server's default CORS list.
  docker rm -f e2e-web >/dev/null 2>&1
  docker run -d --name e2e-web -p 127.0.0.1:$WEB_PORT:80 ghcr.io/gryt-chat/client:latest >/dev/null
  wait_http "http://127.0.0.1:$WEB_PORT/" 60
}

client() { # mode host
  (cd "$HERE/browser" && GRYT_E2E_APP=http://127.0.0.1:$WEB_PORT GRYT_E2E_OUT="$OUT/shots" node client.mjs "$@")
}

summary() {
  {
    echo "| | Step | Time | Detail |"
    echo "|---|---|---|---|"
    while IFS=$'\t' read -r st name secs detail; do
      echo "| $st | $name | ${secs}s | ${detail//|/\\|} |"
    done <"$RESULTS"
  } >"$OUT/results.md"
  [[ -n "${GITHUB_STEP_SUMMARY:-}" ]] && cat "$OUT/results.md" >>"$GITHUB_STEP_SUMMARY"
  cat "$OUT/results.md"
}
