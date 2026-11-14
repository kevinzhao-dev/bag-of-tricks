#!/usr/bin/env bash
set -euo pipefail

APP_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd -- "${APP_DIR}/.." >/dev/null 2>&1 && pwd)"

cd "$APP_DIR"

# shellcheck source=../scripts/activate.sh
source "${REPO_ROOT}/scripts/activate.sh"

exec python "${APP_DIR}/dashboard.py"
