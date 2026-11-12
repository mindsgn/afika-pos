#!/usr/bin/env bash
set -euo pipefail

api_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
persist_dir="$(mktemp -d)"
cleanup() {
	rm -rf "$persist_dir"
}
trap cleanup EXIT INT TERM

cd "$api_dir"
CI=1 WRANGLER_SEND_METRICS=false \
	npx wrangler d1 migrations apply afika --local --persist-to "$persist_dir"

CI=1 WRANGLER_SEND_METRICS=false \
npx wrangler dev --port 4173 --persist-to "$persist_dir" \
		--var API_TOKEN:playwright-test-token \
		--var SESSION_SECRET:playwright-local-session-secret \
		--var CHECKOUT_BASE_URL:http://127.0.0.1:4173 \
		--var E2E_MODE:true
