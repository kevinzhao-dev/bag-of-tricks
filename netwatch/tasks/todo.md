# netwatch 開發計畫

## Phase 1: 專案初始化 + Collector
- [x] 初始化 Go module、建立目錄結構
- [x] 實作 `collector.go` — 解析 `netstat -ib` 取得 interface 的 Ibytes/Obytes
- [x] 單元測試：驗證解析邏輯正確（用 mock netstat 輸出）
- [x] 手動驗證：每秒輸出 rx/tx delta

## Phase 2: Tracker（累計 + 門檻）
- [x] 實作 `tracker.go` — DailyAccumulator：累計當日 TX/RX
- [x] 實作 ThresholdChecker：100GB / 130GB / 150GB 門檻判斷
- [x] 午夜 reset 邏輯
- [x] 偵測 counter reset（重開機）與 sleep/wake（delta time > 5s 跳過）
- [x] 單元測試

## Phase 3: TUI Dashboard
- [x] 實作 bubbletea Model — 即時速率顯示（MB/s）
- [x] 當日累計流量顯示（GB）
- [x] Progress bar + 百分比
- [x] 門檻變色警告（100GB 黃、130GB 橘、150GB 紅）
- [x] 連續超標天數顯示
- [x] 整合 Collector + Tracker 進 TUI 事件循環

## Phase 4: Storage + history 子命令
- [x] 實作 `store.go` — 讀寫 `~/.netwatch/history.json`
- [x] 每日流量自動寫入（退出時 + midnight）
- [x] 實作 `history.go` cobra 子命令 — 列出近 7 天紀錄
- [x] 連續超標天數計算

## Phase 5: CLI flags + 收尾
- [x] `--threshold` flag 自訂門檻值
- [x] `--iface` flag 自訂網路介面
- [x] root.go + main.go cobra 設定
- [x] `go build` 驗證產出單一 binary

## Review

### 變更摘要
所有 5 個 Phase 已完成。產出結構：

```
netwatch/
├── main.go                          # 入口
├── cmd/
│   ├── root.go                      # cobra root + --iface / --threshold flags
│   └── history.go                   # `netwatch history` 子命令
├── internal/
│   ├── collector/
│   │   ├── collector.go             # 解析 netstat -ib、計算 delta、sleep/wake 偵測
│   │   └── collector_test.go        # 4 tests
│   ├── tracker/
│   │   ├── tracker.go               # 日累計、門檻判斷、midnight reset
│   │   └── tracker_test.go          # 5 tests
│   ├── storage/
│   │   ├── store.go                 # JSON 歷史讀寫、30天保留、連續超標計算
│   │   └── store_test.go            # 5 tests
│   └── ui/
│       └── dashboard.go             # bubbletea TUI、progress bar、變色警告
├── go.mod
└── go.sum
```

### 關鍵設計決策
- **Counter reset 偵測**：若 current < prev，視為重開機，以 current 值作為 delta
- **Sleep/wake 偵測**：delta time > 5s 時跳過該 sample，避免異常 spike
- **門檻分級**：自訂 threshold 時，三級門檻按比例計算（67%/87%/100%）
- **歷史寫入時機**：退出時 + 日期翻轉時自動存檔
- **測試覆蓋**：collector、tracker、storage 共 14 個單元測試全通過
