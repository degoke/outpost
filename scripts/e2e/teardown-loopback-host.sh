#!/usr/bin/env bash
# Optional cleanup for local loopback e2e runs. CI runners are ephemeral.
set -euo pipefail

STATE_DIR="${OUTPOST_E2E_STATE_DIR:-/tmp/outpost-e2e}"
rm -f "${STATE_DIR}/loopback_key" "${STATE_DIR}/loopback_key.pub" 2>/dev/null || true
