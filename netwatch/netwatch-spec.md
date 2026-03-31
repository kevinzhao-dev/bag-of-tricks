# `netwatch` — CLI 網路流量監控工具

## Problem Statement

中華電信 HiNet 光世代對日流量超過 150GB 連續三天會降速（降至 300M/300M，持續兩天）。目前缺乏本地端即時監控工具，無法在逼近門檻時提前預警，導致被動收到簡訊才知道超標。

## Target User

macOS 開發者，未來可擴展至 Linux。

## Core Requirements

### P0 — Must Have

| 功能 | 說明 |
|------|------|
| 即時速率顯示 | 每秒更新當前 upload/download 速率（MB/s） |
| 當日累計流量 | 從午夜零點起算，顯示 TX/RX 累計（GB） |
| 門檻警告 | 累計流量達 100GB / 130GB / 150GB 時 terminal 變色提醒 |
| TUI Dashboard | 單一 terminal 畫面，不用捲動 |

### P1 — Should Have

| 功能 | 說明 |
|------|------|
| 歷史紀錄 | 每日流量寫入 `~/.netwatch/history.json`，保留 30 天 |
| `netwatch history` 子命令 | 列出近 7 天每日流量 |
| 連續超標追蹤 | 顯示目前已連續幾天超過 150GB |

### P2 — Nice to Have

| 功能 | 說明 |
|------|------|
| 自訂門檻值 | `--threshold 150` |
| 自訂網路介面 | `--iface en0` |
| Sparkline 圖表 | 過去 60 秒速率趨勢 |

## Tech Stack

| 層級 | 選擇 | 理由 |
|------|------|------|
| 語言 | Go 1.22+ | 單一 binary、cross-compile、系統層存取方便 |
| TUI 框架 | `github.com/charmbracelet/bubbletea` | 成熟的 Elm-style terminal UI 框架 |
| 網路數據來源 | OS network counters | macOS: `netstat -ib`；Linux: `/proc/net/dev` |
| 儲存 | JSON flat file | 簡單、無外部依賴 |
| CLI 框架 | `github.com/spf13/cobra` | 子命令管理標準選擇 |

## Architecture

```
netwatch/
├── cmd/                  # cobra 命令定義
│   ├── root.go           # 預設啟動 dashboard
│   └── history.go        # history 子命令
├── internal/
│   ├── collector/        # 每秒輪詢網路介面統計
│   │   └── collector.go  # GetStats() → (rx, tx uint64)
│   ├── tracker/          # 累計計算、門檻判斷
│   │   └── tracker.go    # DailyAccumulator, ThresholdChecker
│   ├── storage/          # 歷史 JSON 讀寫
│   │   └── store.go
│   └── ui/               # TUI 渲染
│       └── dashboard.go
├── main.go
├── go.mod
└── go.sum
```

## Data Flow

```
OS network counters
       │
       ▼  (每秒 poll)
   Collector ──→ delta(rx, tx) per second
       │
       ▼
   Tracker ──→ 累計當日流量 + 檢查門檻
       │
       ▼
   Dashboard UI ──→ 即時渲染到 terminal
       │
       ▼  (每日 midnight 或退出時)
   Storage ──→ 寫入 ~/.netwatch/history.json
```

## Dashboard Layout

```
╔══════════════════════════════════════════╗
║  netwatch v0.1.0         iface: en0     ║
╠══════════════════════════════════════════╣
║                                         ║
║   ↓ Download    ↑ Upload                ║
║   12.3 MB/s     2.1 MB/s                ║
║                                         ║
║   Today (2026-03-31)                    ║
║   ↓ 82.45 GB    ↑ 11.23 GB             ║
║   Total: 93.68 GB                       ║
║                                         ║
║   ██████████████░░░░░░  62% of 150 GB   ║
║                                         ║
║   Consecutive days > 150GB: 1 (03/30)   ║
║   ⚠ 降速風險：再連續超標 2 天觸發       ║
║                                         ║
╚══════════════════════════════════════════╝
```

## macOS 數據採集方式

| 方案 | 方式 | 優缺點 |
|------|------|--------|
| ✅ 方案 1（建議） | `netstat -ib` 解析 `Ibytes` / `Obytes` | 最簡單，跨版本相容性好 |
| 方案 2 | `syscall` + `sysctl` 讀取 `net.interface.stats` | 效能最好，但實作較複雜 |
| ❌ 方案 3 | `nettop -P -L 1` | per-process 太重，不適合 |

Phase 1 先用方案 1，穩定後再考慮遷移至方案 2。

## Execution Plan

| Phase | 內容 | 預估時間 | 交付物 |
|-------|------|----------|--------|
| **Phase 1** | 專案初始化 + Collector（讀取 OS counters、計算 delta） | 2 hr | `collector.go` 能每秒輸出 rx/tx delta |
| **Phase 2** | Tracker（累計邏輯、午夜 reset、門檻判斷） | 1.5 hr | `tracker.go` 正確累計並觸發警告 |
| **Phase 3** | Dashboard TUI（bubbletea 渲染、顏色警告、progress bar） | 3 hr | 完整 TUI dashboard 可運行 |
| **Phase 4** | Storage + history 子命令 | 1.5 hr | `netwatch history` 輸出近 7 天紀錄 |
| **Phase 5** | CLI flags（`--threshold`, `--iface`）、README | 1 hr | 可配置、文件齊全 |
| **Total** | | **~9 hr** | |

## Risk & Mitigation

| 風險 | 影響 | 對策 |
|------|------|------|
| macOS counters 重開機歸零 | 當日累計失準 | 每次 poll 存 checkpoint，偵測 counter reset |
| VPN / 多介面 | 流量計算漏算或重複 | 預設 `en0`，支援 `--iface` 指定或 `all` 加總 |
| Sleep / Wake | 時間跳躍導致 delta 異常 | 偵測 delta time > 5s 時跳過該 sample |
| HiNet 計算方式差異 | 本地統計與 ISP 不完全一致 | 文件標註為「參考用估算」，建議搭配 HiNet APP 驗證 |

## Success Criteria

- [ ] `netwatch` 啟動後即時顯示 upload/download 速率
- [ ] 當日累計流量與 HiNet 查詢結果誤差 < 10%
- [ ] 流量超過門檻時有明確視覺警告
- [ ] `netwatch history` 正確顯示過去 7 天紀錄
- [ ] 單一 binary，`go build` 即可產出，無外部 runtime 依賴

## Future Considerations

- Linux 支援（讀取 `/proc/net/dev`）
- 系統通知整合（macOS Notification Center）
- Prometheus exporter（`/metrics` endpoint）
- 多 ISP 門檻 preset（中華電信、遠傳、台灣大）
