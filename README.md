# bag-of-tricks

A collection of small tools. Most sub-projects are Go; `site_monitor` and `calsync` are Python.

## Python setup (one-shot)

Creates a single repo-wide venv at `.venv/` and installs deps for every Python sub-project (`site_monitor`, `calsync`).

```bash
./scripts/setup-python.sh        # idempotent, re-run any time
source scripts/activate.sh       # activate the venv in current shell
```

`scripts/activate.sh` auto-runs the setup if `.venv/` is missing, so a fresh checkout only needs `source scripts/activate.sh`.

Once activated:

```bash
cd site_monitor && python dashboard.py   # site monitor dashboard
calsync --help                            # calsync CLI (macOS only)
deactivate                                # leave the venv
```

Use a different interpreter via `PYTHON=/path/to/python3 ./scripts/setup-python.sh` (requires Python 3.11+).
