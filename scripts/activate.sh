# Source this file (not execute) to activate the repo-wide Python venv.
#
#   source scripts/activate.sh
#
# Auto-runs scripts/setup-python.sh if the venv is missing.

# Resolve the directory of this script even when sourced.
if [ -n "${BASH_SOURCE:-}" ]; then
    _activate_src="${BASH_SOURCE[0]}"
elif [ -n "${ZSH_VERSION:-}" ]; then
    _activate_src="${(%):-%N}"
else
    _activate_src="$0"
fi

_activate_dir="$(cd -- "$(dirname -- "$_activate_src")/.." > /dev/null 2>&1 && pwd)"
_venv_activate="${_activate_dir}/.venv/bin/activate"

if [ ! -f "$_venv_activate" ]; then
    echo "venv not found — running scripts/setup-python.sh"
    "${_activate_dir}/scripts/setup-python.sh" || return 1
fi

# shellcheck disable=SC1090
. "$_venv_activate"

unset _activate_src _activate_dir _venv_activate
