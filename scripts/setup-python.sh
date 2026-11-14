#!/usr/bin/env bash
# One-shot Python environment setup for bag-of-tricks.
#
# Creates a single repo-wide venv at <repo>/.venv and installs the
# dependencies for every Python sub-project here (site_monitor, calsync).
#
# Re-running is safe and idempotent.

set -euo pipefail

REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." &> /dev/null && pwd)"
VENV_DIR="${REPO_ROOT}/.venv"
PYTHON_BIN="${PYTHON:-python3}"
MIN_PY_MAJOR=3
MIN_PY_MINOR=11

cd "$REPO_ROOT"

# --- helpers ----------------------------------------------------------------
info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()    { printf '\033[1;32m✓\033[0m  %s\n' "$*"; }
warn()  { printf '\033[1;33m!\033[0m  %s\n' "$*" >&2; }
die()   { printf '\033[1;31m✗\033[0m  %s\n' "$*" >&2; exit 1; }

# --- python check -----------------------------------------------------------
command -v "$PYTHON_BIN" >/dev/null 2>&1 || die "Python interpreter '$PYTHON_BIN' not found. Install Python ${MIN_PY_MAJOR}.${MIN_PY_MINOR}+ or set PYTHON=/path/to/python3."

PY_VERSION="$("$PYTHON_BIN" -c 'import sys; print(f"{sys.version_info[0]}.{sys.version_info[1]}")')"
"$PYTHON_BIN" -c "import sys; sys.exit(0 if sys.version_info >= (${MIN_PY_MAJOR}, ${MIN_PY_MINOR}) else 1)" \
    || die "Python ${MIN_PY_MAJOR}.${MIN_PY_MINOR}+ required, found ${PY_VERSION} at $(command -v "$PYTHON_BIN")."

info "Using $(command -v "$PYTHON_BIN") (Python ${PY_VERSION})"

# --- venv -------------------------------------------------------------------
if [[ -d "$VENV_DIR" ]]; then
    ok "venv already exists at ${VENV_DIR}"
else
    info "Creating venv at ${VENV_DIR}"
    "$PYTHON_BIN" -m venv "$VENV_DIR"
    ok "venv created"
fi

# shellcheck disable=SC1091
source "${VENV_DIR}/bin/activate"

info "Upgrading pip / setuptools / wheel"
python -m pip install --upgrade --quiet pip setuptools wheel

# --- site_monitor deps ------------------------------------------------------
info "Installing site_monitor dependencies (requests, beautifulsoup4, lxml)"
python -m pip install --quiet requests beautifulsoup4 lxml

# --- calsync (editable) -----------------------------------------------------
# calsync is macOS-only (PyObjC + EventKit via maccal). Skip silently on
# other platforms so Linux/CI runs don't fail.
if [[ "$(uname -s)" == "Darwin" ]]; then
    info "Installing calsync (editable) + dev extras"
    python -m pip install --quiet -e "./calsync[dev]"

    # maccal pulls in pyobjc-framework-EventKit, but the unrelated
    # 'eventkit' PyPI package (ib-insync dep) can shadow Apple's framework.
    # Re-pin EventKit if the shadow is present.
    if python -c "import importlib, sys; sys.exit(0 if importlib.util.find_spec('eventkit') else 1)" 2>/dev/null; then
        warn "'eventkit' PyPI package detected — reinstalling pyobjc-framework-EventKit to avoid shadowing"
        python -m pip install --quiet --force-reinstall --no-deps pyobjc-framework-EventKit
    fi
else
    warn "Skipping calsync install — macOS only (current OS: $(uname -s))"
    info "Installing pytest for site_monitor tests"
    python -m pip install --quiet pytest
fi

deactivate

# --- done -------------------------------------------------------------------
cat <<EOF

$(ok "Setup complete.")

Activate the environment in your current shell:

    source ${VENV_DIR}/bin/activate
    # or, shorter:
    source scripts/activate.sh

Then you can run, for example:

    cd site_monitor && python dashboard.py     # site monitor dashboard
    calsync --help                              # calsync CLI (macOS only)

To deactivate later: \`deactivate\`.
EOF
