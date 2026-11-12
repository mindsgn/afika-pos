#!/usr/bin/env bash
#
# Test, run, and deploy every Afika backend service.
#
#   ./scripts/backend.sh test     Run Go tests, vet, gofmt, and the D1 checks
#   ./scripts/backend.sh run      Run the API and all workers locally
#   ./scripts/backend.sh deploy   Apply remote migrations, deploy, health check
#
# The services share one D1 database named `afika`. Local `run` uses one
# persistent directory for all services so they see the same rows.
#
# Environment:
#   AFIKA_D1_PERSIST_DIR  Local D1 directory (default: a temporary directory)
#   AFIKA_API_URL         API base URL for the deploy health check
#                         (default: https://afika-api.seni-tembe.workers.dev)
#   ALCHEMY_API_KEY       Passed to balance/transaction when set

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

services=(api balance forex transaction receipt)
service_dirs=(api workers/balance workers/forex workers/transaction workers/receipt)
service_ports=(8787 8788 8789 8790 8791)
service_labels=("API" "balance worker" "forex worker" "transaction worker" "receipt worker")

# Every service that owns migrations for the shared `afika` database. api and
# receipt both define pos_payments (idempotent, distinct filenames); api comes
# first so the checkout table exists before the reconciler starts.
migration_dirs=(api workers/balance workers/forex workers/transaction workers/receipt)

usage() {
	cat <<'EOF'
Usage: ./scripts/backend.sh <command>

Commands:
  test     Run go test -race, go vet, gofmt, and the shared D1 checks.
  run      Apply local migrations and run the API plus all workers.
  deploy   Apply remote migrations, deploy all services, verify /v1/health.

Environment:
  AFIKA_D1_PERSIST_DIR  Local D1 directory (default: a temporary directory).
  AFIKA_API_URL         API base URL for the deploy health check.
  ALCHEMY_API_KEY       Passed to the balance and transaction workers.
EOF
}

log() {
	printf '==> %s\n' "$*"
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

ensure_wrangler() {
	local dir="$1"
	if [[ -x "$dir/node_modules/.bin/wrangler" ]]; then
		return
	fi
	log "Installing dependencies for $dir"
	if [[ -f "$dir/package-lock.json" ]]; then
		(cd "$dir" && npm ci --no-audit --no-fund --silent)
	else
		# workers/receipt has no committed lockfile yet.
		(cd "$dir" && npm install --no-audit --no-fund --silent)
	fi
}

ensure_dev_vars() {
	local dir="$1"
	if [[ ! -f "$dir/.dev.vars" && -f "$dir/.dev.vars.example" ]]; then
		log "Creating $dir/.dev.vars from the example"
		cp "$dir/.dev.vars.example" "$dir/.dev.vars"
	fi
}

run_tests() {
	local dir

	for dir in "${service_dirs[@]}"; do
		log "Testing $dir"
		(
			cd "$repo_root/$dir"
			go test -race ./...
			go vet ./...
		)
	done

	log "Checking gofmt"
	local unformatted
	unformatted="$(cd "$repo_root" && gofmt -l api workers)"
	if [[ -n "$unformatted" ]]; then
		printf '%s\n' "$unformatted" >&2
		die "gofmt found unformatted files"
	fi

	log "Running shared D1 integration checks"
	"$repo_root/scripts/test-database.sh"
}

local_migrations() {
	local persist_dir="$1"
	local dir

	for dir in "${migration_dirs[@]}"; do
		dir="$repo_root/$dir"
		ensure_wrangler "$dir"
		log "Applying local migrations for ${dir#"$repo_root/"}"
		(
			cd "$dir"
			CI=1 WRANGLER_SEND_METRICS=false \
				npx wrangler d1 migrations apply afika --local --persist-to "$persist_dir"
		)
	done
}

remote_migrations() {
	local dir

	for dir in "${migration_dirs[@]}"; do
		dir="$repo_root/$dir"
		ensure_wrangler "$dir"
		log "Applying remote migrations for ${dir#"$repo_root/"}"
		(
			cd "$dir"
			CI=1 WRANGLER_SEND_METRICS=false \
				npx wrangler d1 migrations apply afika --remote
		)
	done
}

run_local() {
	local persist_dir log_dir cleanup_persist=0
	if [[ -n "${AFIKA_D1_PERSIST_DIR:-}" ]]; then
		persist_dir="$AFIKA_D1_PERSIST_DIR"
		mkdir -p "$persist_dir"
	else
		persist_dir="$(mktemp -d)"
		cleanup_persist=1
	fi
	log_dir="${AFIKA_LOG_DIR:-$(mktemp -d)}"

	local pids=()
	local i

	cleanup() {
		trap - EXIT INT TERM
		for i in "${pids[@]:-}"; do
			kill "$i" 2>/dev/null || true
		done
		wait 2>/dev/null || true
		if [[ "$cleanup_persist" == "1" ]]; then
			rm -rf "$persist_dir"
		fi
	}
	trap cleanup EXIT INT TERM

	log "Local D1 state: $persist_dir"
	log "Logs: $log_dir"
	local_migrations "$persist_dir"

	for i in "${!services[@]}"; do
		local name="${services[$i]}"
		local dir="$repo_root/${service_dirs[$i]}"
		local port="${service_ports[$i]}"
		local args=(--port "$port" --persist-to "$persist_dir")

		ensure_wrangler "$dir"
		ensure_dev_vars "$dir"

		if [[ "$name" != "api" ]]; then
			args+=(--test-scheduled)
		fi
		if [[ "$name" == "balance" || "$name" == "transaction" ]] && [[ -n "${ALCHEMY_API_KEY:-}" ]]; then
			args+=(--var "ALCHEMY_API_KEY:$ALCHEMY_API_KEY")
		fi

		log "Starting ${service_labels[$i]} on http://localhost:$port"
		(
			cd "$dir"
			CI=1 WRANGLER_SEND_METRICS=false \
				exec npx wrangler dev "${args[@]}"
		) >"$log_dir/$name.log" 2>&1 &
		pids+=("$!")
	done

	cat <<EOF

All services are starting. Logs are in $log_dir.

API:              http://localhost:${service_ports[0]}/v1/health
Balance cron:     curl "http://localhost:${service_ports[1]}/__scheduled?cron=0+*+*+*+*"
Forex cron:       curl "http://localhost:${service_ports[2]}/__scheduled?cron=0+*+*+*+*"
Transaction cron: curl "http://localhost:${service_ports[3]}/__scheduled?cron=*/5+*+*+*+*"
Receipt cron:     curl "http://localhost:${service_ports[4]}/__scheduled?cron=*+*+*+*+*"

Press Ctrl-C to stop all services.
EOF

	wait || true
}

deploy_all() {
	local api_url="${AFIKA_API_URL:-https://afika-api.seni-tembe.workers.dev}"

	remote_migrations

	local i
	for i in "${!services[@]}"; do
		local dir="$repo_root/${service_dirs[$i]}"
		ensure_wrangler "$dir"
		log "Deploying ${service_labels[$i]}"
		(
			cd "$dir"
			CI=1 WRANGLER_SEND_METRICS=false npm run deploy
		)
	done

	log "Checking $api_url/v1/health"
	curl -fsS --retry 5 --retry-delay 2 "$api_url/v1/health"
	printf '\n'
	log "All services deployed"
}

command="${1:-}"
case "$command" in
test)
	run_tests
	;;
run)
	run_local
	;;
deploy)
	run_tests
	deploy_all
	;;
-h | --help | help | "")
	usage
	;;
*)
	usage >&2
	die "unknown command: $command"
	;;
esac
