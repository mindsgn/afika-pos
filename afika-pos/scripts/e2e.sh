#!/usr/bin/env bash
#
# Runs the Maestro end-to-end flows against the iOS simulator.
#
#   scripts/e2e.sh                          # onboarding, add-product, sell
#   scripts/e2e.sh .maestro/sell.yaml       # one flow
#   RESET_DB=0 scripts/e2e.sh               # keep the local database
#   MODE=release scripts/e2e.sh             # test a Release build (no Metro)
#
# A Debug build is an Expo development client: `launchApp clearState` would
# drop the packager URL and strand the test on the Expo launcher. Instead the
# script wipes only the app's SQLite database and the flows deep-link straight
# into Metro.
set -euo pipefail

cd "$(dirname "$0")/.."

APP_ID="${APP_ID:-phumzile.afika.pos}"
DB_NAME="${DB_NAME:-crypto-pos.db}"
METRO_URL="${METRO_URL:-http://localhost:8081}"
RESET_DB="${RESET_DB:-1}"
MODE="${MODE:-dev}"
SCHEME="${SCHEME:-afika}"

command -v maestro >/dev/null || { echo "maestro is not installed" >&2; exit 1; }

if [[ -n "${MAESTRO_DEVICE:-}" ]]; then
  DEVICE="$MAESTRO_DEVICE"
else
  DEVICE="$(xcrun simctl list devices booted --json | python3 -c '
import json, sys
data = json.load(sys.stdin)["devices"]
for runtime, devices in data.items():
    if "iOS" not in runtime:
        continue
    for device in devices:
        if device.get("state") == "Booted":
            print(device["udid"])
            sys.exit(0)
sys.exit("No booted iOS simulator. Boot one or set MAESTRO_DEVICE.")
')"
fi
export MAESTRO_DEVICE="$DEVICE"
echo "==> Simulator: $DEVICE"

if [[ "$MODE" == "dev" ]]; then
  if ! curl -sf --max-time 5 "$METRO_URL/status" >/dev/null; then
    echo "Metro is not responding at $METRO_URL. Start it with: npx expo start --localhost" >&2
    exit 1
  fi
  ENCODED="$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$METRO_URL")"
  LAUNCH_URL="$SCHEME://expo-development-client/?url=$ENCODED"
else
  LAUNCH_URL="$SCHEME://"
fi

if [[ "$RESET_DB" == "1" ]]; then
  echo "==> Resetting the local database"
  xcrun simctl terminate "$DEVICE" "$APP_ID" 2>/dev/null || true
  CONTAINER="$(xcrun simctl get_app_container "$DEVICE" "$APP_ID" data)"
  rm -f "$CONTAINER/Documents/SQLite/$DB_NAME" \
        "$CONTAINER/Documents/SQLite/$DB_NAME-wal" \
        "$CONTAINER/Documents/SQLite/$DB_NAME-shm"
fi

if [[ "$#" -eq 0 ]]; then
  set -- .maestro/onboarding.yaml .maestro/add-product.yaml .maestro/sell.yaml
fi

for flow in "$@"; do
  echo "==> maestro test $flow"
  # Each flow resets the database itself through this script's RESET_DB hook;
  # only the first flow starts from a clean slate when running them all.
  maestro test --device "$DEVICE" -e APP_LAUNCH_URL="$LAUNCH_URL" "$flow"
  if [[ "$RESET_DB" == "1" && "$#" -gt 1 ]]; then
    xcrun simctl terminate "$DEVICE" "$APP_ID" 2>/dev/null || true
    CONTAINER="$(xcrun simctl get_app_container "$DEVICE" "$APP_ID" data)"
    rm -f "$CONTAINER/Documents/SQLite/$DB_NAME"*
  fi
done
