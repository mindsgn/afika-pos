#!/usr/bin/env bash
#
# Runs the Maestro end-to-end flows against the iOS simulator.
#
#   scripts/e2e.sh                          # onboarding, add-product, sell
#   scripts/e2e.sh .maestro/sell.yaml       # one flow
#   RESET_DB=0 scripts/e2e.sh               # keep the local database
#   MODE=release scripts/e2e.sh             # test a Release build (no Metro)
#   START_METRO=0 scripts/e2e.sh            # require an already-running Metro
#
# A Debug build is an Expo development client: `launchApp clearState` would
# drop the packager URL and strand the test on the Expo launcher. Instead the
# script wipes only the app's SQLite database and the flows deep-link straight
# into Metro.
#
# Metro must bundle the app with EXPO_PUBLIC_E2E=1 so the sign-in screen accepts
# the fixed test code instead of asking Privy for a real email code. The script
# starts Metro itself with that flag when none is running.
set -euo pipefail

cd "$(dirname "$0")/.."

APP_ID="${APP_ID:-phumzile.afika.pos}"
DB_NAME="${DB_NAME:-crypto-pos.db}"
METRO_URL="${METRO_URL:-http://localhost:8081}"
RESET_DB="${RESET_DB:-1}"
MODE="${MODE:-dev}"
SCHEME="${SCHEME:-afika}"
export EXPO_PUBLIC_E2E="${EXPO_PUBLIC_E2E:-1}"

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

metro_pid=""
cleanup_metro() {
  if [[ -n "$metro_pid" ]]; then
    kill "$metro_pid" 2>/dev/null || true
  fi
}
trap cleanup_metro EXIT

if [[ "$MODE" == "dev" ]]; then
  if curl -sf --max-time 5 "$METRO_URL/status" >/dev/null; then
    echo "==> Using the Metro server already running at $METRO_URL"
    if [[ "$EXPO_PUBLIC_E2E" == "1" ]]; then
      echo "    It must have been started with EXPO_PUBLIC_E2E=1 or the sign-in flow will fail."
    fi
  elif [[ "${START_METRO:-1}" == "1" ]]; then
    metro_log="${AFIKA_E2E_LOG_DIR:-$(mktemp -d)}/metro.log"
    echo "==> Starting Metro with EXPO_PUBLIC_E2E=$EXPO_PUBLIC_E2E (logs: $metro_log)"
    # Yarn PnP installs have no node_modules, so `npx expo` cannot resolve the
    # SDK; run Expo through the project's package manager.
    if [[ -f yarn.lock ]]; then
      expo_cmd=(yarn expo)
    else
      expo_cmd=(npx expo)
    fi
    EXPO_PUBLIC_E2E="$EXPO_PUBLIC_E2E" "${expo_cmd[@]}" start --localhost >"$metro_log" 2>&1 &
    metro_pid="$!"
    for _ in $(seq 1 90); do
      curl -sf --max-time 2 "$METRO_URL/status" >/dev/null && break
      if ! kill -0 "$metro_pid" 2>/dev/null; then
        echo "Metro exited before it was ready; see $metro_log" >&2
        exit 1
      fi
      sleep 1
    done
    if ! curl -sf --max-time 2 "$METRO_URL/status" >/dev/null; then
      echo "Metro did not become ready at $METRO_URL; see $metro_log" >&2
      exit 1
    fi
  else
    echo "Metro is not responding at $METRO_URL. Start it with: EXPO_PUBLIC_E2E=1 yarn expo start --localhost" >&2
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
