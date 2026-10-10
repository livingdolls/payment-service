#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_dir"

die() { printf '%s\n' "$*" >&2; exit 2; }
need() { command -v "$1" >/dev/null 2>&1 || die "Required command unavailable: $1"; }

source "$repo_dir/scripts/with-env.sh"

align_go() {
  need go
  local payment_go_root
  payment_go_root="$(go env GOROOT)"
  [[ -x "$payment_go_root/bin/go" ]] || die "Selected Go toolchain is unavailable."
  export GOROOT="$payment_go_root" GOTOOLCHAIN=local PATH="$payment_go_root/bin:$PATH"
  [[ "$(go env GOVERSION)" == go1.26* ]] || die "This project requires Go 1.26. Run with Go toolchain auto-selection enabled first."
  export GOCACHE="${PAYMENT_GO_CACHE:-/tmp/payment-service-go-cache}"
}

database_url_for() {
  local base="$1" name="$2" suffix=""
  [[ "$base" == postgres://* || "$base" == postgresql://* ]] || die "Database settings must use a PostgreSQL connection URI."
  if [[ "$base" == *\?* ]]; then suffix="?${base#*\?}"; fi
  base="${base%%\?*}"
  printf '%s/%s%s' "${base%/*}" "$name" "$suffix"
}

prepare_database() {
  local target_url="$1" path name admin_url exists
  path="${target_url%%\?*}"
  name="${path##*/}"
  [[ "$name" =~ ^[a-z][a-z0-9_]*_test$ && ${#name} -le 63 ]] || die "Refusing database work: name must be lowercase ASCII and end in _test (maximum 63 characters)."
  admin_url="$(database_url_for "$target_url" postgres)"
  exists="$(psql --dbname "$admin_url" --no-psqlrc -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname = '$name'" 2>/dev/null)" || die "Cannot reach PostgreSQL; start the existing compose database and check DATABASE_URL."
  if [[ "$exists" != 1 ]]; then
    psql --dbname "$admin_url" --no-psqlrc -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$name\"" >/dev/null 2>&1 || die "Cannot create dedicated test database."
  fi
  printf 'Dedicated test database ready: %s\n' "$name"
}

load_env "${ENV_FILE:-.env}"
align_go
mode="${1:-local}"
if [[ $# -gt 0 ]]; then shift; fi
need psql
need goose

case "$mode" in
  local)
    if [[ -z "${TEST_DATABASE_URL:-}" ]]; then
      export TEST_DATABASE_URL="$(database_url_for "${DATABASE_URL:-postgres://payment:payment@127.0.0.1:5433/payment?sslmode=disable}" "${PAYMENT_TEST_DATABASE:-payment_service_test}")"
    fi
    prepare_database "$TEST_DATABASE_URL"
    export PAYMENT_TEST_REPORT_DIR="${PAYMENT_TEST_REPORT_DIR:-$repo_dir/artifacts/payment-tests}"
    go test -race -cover ./...
    # Each integration test migrates and removes its own random schema.
    go test -race -count=1 -tags=integration ./internal/e2e "$@"
    ;;
  sandbox|sandbox-setup)
    export APP_ENV="${APP_ENV:-development}"
    source_database_url="${DATABASE_URL:-postgres://payment:payment@127.0.0.1:5433/payment?sslmode=disable}"
    export DATABASE_URL="${SANDBOX_DATABASE_URL:-$(database_url_for "$source_database_url" "${PAYMENT_SANDBOX_DATABASE:-payment_service_sandbox_test}")}"
    export HTTP_PORT="${PAYMENT_SANDBOX_PORT:-8091}"
    export XENDIT_BASE_URL="${XENDIT_BASE_URL:-https://api.xendit.co}"
    [[ -n "${ADMIN_API_TOKEN:-}" ]] || die "ADMIN_API_TOKEN is required; add a random value of at least 32 characters to .env."
    [[ ${#ADMIN_API_TOKEN} -ge 32 ]] || die "ADMIN_API_TOKEN must contain at least 32 characters."
    [[ "$HTTP_PORT" =~ ^[0-9]+$ && "$HTTP_PORT" -ge 1024 && "$HTTP_PORT" -le 65535 ]] || die "PAYMENT_SANDBOX_PORT must be 1024–65535."
    run_dir="$(mktemp -d /tmp/payment-sandbox.XXXXXX)"
    api_pid="" tunnel_pid=""
    cleanup() {
      [[ -z "$tunnel_pid" ]] || kill "$tunnel_pid" 2>/dev/null || true
      [[ -z "$api_pid" ]] || kill "$api_pid" 2>/dev/null || true
    }
    trap cleanup EXIT
    go build -o "$run_dir/payment-test" ./cmd/payment-test
    "$run_dir/payment-test" -check-config -api-url "http://127.0.0.1:$HTTP_PORT"
    if [[ "${SANDBOX_REUSE_API:-}" != 1 ]]; then
      if (exec 3<>"/dev/tcp/127.0.0.1/$HTTP_PORT") 2>/dev/null; then
        die "Sandbox port is already in use. Choose PAYMENT_SANDBOX_PORT or explicitly set SANDBOX_REUSE_API=1 for the intended API."
      fi
      prepare_database "$DATABASE_URL"
      GOOSE_DRIVER=postgres GOOSE_DBSTRING="$DATABASE_URL" goose -env= -dir migrations up >/dev/null 2>&1 || die "Sandbox migrations failed."
      go build -o "$run_dir/payment-api" ./cmd/api
      "$run_dir/payment-api" >"$run_dir/api.log" 2>&1 &
      api_pid="$!"
    fi
    need curl
    for ((attempt=0; attempt<40; attempt++)); do
      curl -fsS "http://127.0.0.1:$HTTP_PORT/health/ready" >/dev/null 2>&1 && break
      if [[ -n "$api_pid" ]]; then
        kill -0 "$api_pid" 2>/dev/null || die "Sandbox API exited during startup; inspect its local log after checking configuration."
      fi
      sleep 0.25
    done
    curl -fsS "http://127.0.0.1:$HTTP_PORT/health/ready" >/dev/null 2>&1 || die "Sandbox API did not become ready."
    if [[ -n "$api_pid" ]]; then
      kill -0 "$api_pid" 2>/dev/null || die "Spawned sandbox API exited; refusing to use another server on this port."
    fi
    if [[ -z "${SANDBOX_PUBLIC_URL:-}" ]]; then
      need ngrok
      need python3
      ngrok http "$HTTP_PORT" --log=stdout --log-format=json >"$run_dir/ngrok.log" 2>&1 &
      tunnel_pid="$!"
      for ((attempt=0; attempt<40; attempt++)); do
        kill -0 "$tunnel_pid" 2>/dev/null || die "The ngrok process exited; refusing to reuse a different process's tunnel. Set SANDBOX_PUBLIC_URL for an existing tunnel."
        # Read only our own agent log and verify its exact loopback upstream;
        # querying port 4040 could accidentally select someone else's tunnel.
        discovered_url="$(python3 - "$run_dir/ngrok.log" "$HTTP_PORT" <<'PY'
import json
import sys
from urllib.parse import urlparse
with open(sys.argv[1]) as stream:
    for line in stream:
        try:
            entry = json.loads(line)
            target = urlparse(entry.get('addr', ''))
            public = urlparse(entry.get('url', ''))
            if target.hostname in ('localhost', '127.0.0.1', '::1') and target.port == int(sys.argv[2]) and public.scheme == 'https' and public.hostname:
                print(public.geturl())
                break
        except (ValueError, TypeError):
            pass
PY
)"
        if [[ "$discovered_url" == https://* ]]; then
          export SANDBOX_PUBLIC_URL="$discovered_url"
          break
        fi
        sleep 0.25
      done
      kill -0 "$tunnel_pid" 2>/dev/null || die "Spawned ngrok tunnel is no longer running."
    fi
    [[ "${SANDBOX_PUBLIC_URL:-}" == https://* ]] || die "No HTTPS tunnel found. Set SANDBOX_PUBLIC_URL to a tunnel forwarding to this API."
    printf '\nSandbox setup: API and workers are running on port %s.\n' "$HTTP_PORT"
    printf 'In Xendit TEST mode use the exact callback entry Payment Requests v3 – Payment Status:\n%s/webhooks/xendit/payments\n' "${SANDBOX_PUBLIC_URL%/}"
    printf 'The legacy Virtual Account callback is incompatible with this endpoint. Use the existing XENDIT_WEBHOOK_TOKEN from .env and verify v3 delivery before payments.\n'
    if [[ "$mode" == sandbox-setup ]]; then
      printf 'Setup only: no payments will be created. Keep this terminal running; Ctrl+C stops API/tunnel.\n'
      if [[ -n "$api_pid" ]]; then
        wait "$api_pid"
      else
        while :; do sleep 30; done
      fi
    else
      if [[ "${SANDBOX_CALLBACK_CONFIGURED:-}" != 1 ]]; then
        if [[ -t 0 ]]; then
          printf 'After dashboard configuration and a successful callback test, type configured to start: '
          IFS= read -r answer
          [[ "$answer" == configured ]] || die "Sandbox payment run canceled before creating payments."
          export SANDBOX_CALLBACK_CONFIGURED=1
        else
          die "Setup complete; payments blocked until callback configuration is confirmed. Rerun with SANDBOX_PUBLIC_URL and SANDBOX_CALLBACK_CONFIGURED=1."
        fi
      fi
      "$run_dir/payment-test" -api-url "http://127.0.0.1:$HTTP_PORT" -report-dir "${PAYMENT_TEST_REPORT_DIR:-$repo_dir/artifacts/payment-tests}" "$@"
    fi
    ;;
  *) die "Usage: scripts/test-payment.sh [local|sandbox|sandbox-setup] [test/runner options]" ;;
esac
