# netwatch

A terminal dashboard that monitors daily network traffic on macOS. Built to warn you before hitting ISP daily limits (e.g., HiNet's 150 GB/day threshold that triggers speed throttling).

```
╭──────────────────────────────────────────────╮
│  netwatch v0.1.0              iface: en0     │
│                                              │
│  Current Speed                               │
│  ↓ 12.3 MB/s    ↑ 2.1 MB/s                  │
│                                              │
│  Today (2026-03-31)                          │
│  ↓ 82.45 GB    ↑ 11.23 GB                   │
│  Total: 93.68 GB                             │
│                                              │
│  ██████████████████░░░░░░░░░░  62% of 150 GB │
│                                              │
│  Consecutive days > 150 GB: 1 (2026-03-30)   │
│  ⚠ Throttle risk: 2 more days to trigger     │
│                                              │
│  Press q to quit                             │
╰──────────────────────────────────────────────╯
```

## Features

- **Real-time speed** — upload/download rate updated every second
- **Daily accumulation** — tracks total RX/TX since midnight with progress bar
- **Threshold warnings** — color-coded alerts at 100 GB (yellow), 130 GB (orange), 150 GB (red)
- **Persistent tracking** — checkpoint system recovers accumulated traffic across program restarts
- **History** — stores daily records in `~/.netwatch/history.json` (30-day retention)
- **Consecutive day tracking** — warns when approaching multi-day throttle triggers

## Install

Requires Go 1.22+.

```bash
go build -o netwatch .
```

## Usage

```bash
# Start the dashboard (defaults: en0, 150 GB threshold)
./netwatch

# Use a different interface
./netwatch --iface en1

# Custom threshold
./netwatch --threshold 200

# View recent history
./netwatch history

# Show last 14 days
./netwatch history -n 14
```

## How it works

1. **Collector** reads OS network counters via `netstat -ib` every second
2. **Tracker** accumulates daily RX/TX and checks threshold levels
3. **Dashboard** renders a live TUI via [Bubble Tea](https://github.com/charmbracelet/bubbletea)
4. **Storage** persists daily records and checkpoints to `~/.netwatch/`

Traffic is recovered across restarts using a checkpoint file that stores the last known OS counter values. When the program starts, it computes the difference to catch up on traffic that occurred while it wasn't running.

### Limitations

- macOS only (uses `netstat -ib`; Linux support planned)
- OS counters reset on reboot — traffic during the reboot gap cannot be recovered
- Local measurement is an estimate; verify against your ISP's dashboard for accuracy

## Project structure

```
netwatch/
├── main.go
├── cmd/
│   ├── root.go              # CLI entry, --iface / --threshold flags
│   └── history.go           # `netwatch history` subcommand
└── internal/
    ├── collector/            # Reads OS network counters, computes deltas
    ├── tracker/              # Daily accumulation, threshold checks
    ├── storage/              # JSON history + checkpoint persistence
    └── ui/                   # Bubble Tea TUI dashboard
```

## License

MIT
