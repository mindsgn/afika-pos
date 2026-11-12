#!/usr/bin/env bash
#
# Run the whole Afika POS stack locally for testing.
#
#   scripts/dev.sh            Backend (API + all workers) and the Expo app
#   scripts/dev.sh backend    Backend only
#   scripts/dev.sh app        Expo app (Metro) only
#
# The backend is started with backend/scripts/backend.sh run, which applies the
# shared local D1 migrations (api, balance, forex, transaction, receipt) and
# serves every service on its own port. Metro serves the app on
# http://localhost:8081. Ctrl-C stops everything.
#
# Environment:
#   AFIKA_D1_PERSIST_DIR  Keep local D1 state in this directory (default: temp)
#   ALCHEMY_API_KEY       Passed to the balance and transaction workers
#   METRO_URL             Metro address (default: http://localhost:8081)

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target="${1:-all}"
metro_url="${METRO_URL:-http://localhost:8081}"

case "$target" in
all | backend | app) ;;
-h | --help | help)
	sed -n '2,18p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
	exit 0
	;;
*)
	echo "error: unknown target '$target' (expected: all, backend, app)" >&2
	exit 1
	;;
esac

pids=()
cleanup() {
	trap - EXIT INT TERM
	for pid in "${pids[@]:-}"; do
		kill "$pid" 2>/dev/null || true
	done
	wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

start_backend() {
	echo "==> Starting the API and workers (backend/scripts/backend.sh run)"
	"$repo_root/backend/scripts/backend.sh" run &
	pids+=("$!")
}

start_app() {
	if curl -sf --max-time 5 "$metro_url/status" >/dev/null; then
		echo "==> Metro is already running at $metro_url"
		return
	fi
	echo "==> Starting Metro at $metro_url"
	(cd "$repo_root/app" && yarn expo start --localhost) &
	pids+=("$!")
}

case "$target" in
backend)
	start_backend
	;;
app)
	start_app
	;;
all)
	start_backend
	start_app
	;;
esac

echo
echo "Press Ctrl-C to stop."
wait || true
