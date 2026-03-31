package tracker

import (
	"time"
)

// ThresholdLevel represents the severity of a threshold breach.
type ThresholdLevel int

const (
	LevelNormal ThresholdLevel = iota
	LevelWarning               // 100 GB
	LevelDanger                // 130 GB
	LevelCritical              // 150 GB
)

const (
	GB = 1024 * 1024 * 1024 // bytes
)

// Threshold defines a traffic threshold and its level.
type Threshold struct {
	Bytes uint64
	Level ThresholdLevel
	Label string
}

// Tracker accumulates daily traffic and checks thresholds.
type Tracker struct {
	RxTotal    uint64
	TxTotal    uint64
	Date       time.Time // the date being tracked (truncated to day)
	Thresholds []Threshold
}

// New creates a Tracker with the given threshold in GB.
// It generates three threshold levels at ~67%, ~87%, and 100% of the limit.
func New(limitGB float64) *Tracker {
	limit := uint64(limitGB * float64(GB))
	return &Tracker{
		Date: truncateToDay(time.Now()),
		Thresholds: []Threshold{
			{Bytes: uint64(float64(limit) * 100.0 / 150.0), Level: LevelWarning, Label: "100 GB"},
			{Bytes: uint64(float64(limit) * 130.0 / 150.0), Level: LevelDanger, Label: "130 GB"},
			{Bytes: limit, Level: LevelCritical, Label: "150 GB"},
		},
	}
}

// NewWithDefaults creates a Tracker with the default 150 GB limit.
func NewWithDefaults() *Tracker {
	return New(150)
}

// Seed sets the initial accumulated values (used to restore from checkpoint).
func (t *Tracker) Seed(rx, tx uint64, date time.Time) {
	t.RxTotal = rx
	t.TxTotal = tx
	t.Date = truncateToDay(date)
}

// Add accumulates rx/tx bytes. If the date has changed (past midnight), it
// returns true to signal a day rollover and resets the counters.
func (t *Tracker) Add(rx, tx uint64, now time.Time) (dayRolled bool) {
	today := truncateToDay(now)
	if today != t.Date {
		t.RxTotal = 0
		t.TxTotal = 0
		t.Date = today
		dayRolled = true
	}
	t.RxTotal += rx
	t.TxTotal += tx
	return dayRolled
}

// Total returns the combined rx+tx bytes for today.
func (t *Tracker) Total() uint64 {
	return t.RxTotal + t.TxTotal
}

// TotalGB returns the combined traffic in GB.
func (t *Tracker) TotalGB() float64 {
	return float64(t.Total()) / float64(GB)
}

// RxGB returns download traffic in GB.
func (t *Tracker) RxGB() float64 {
	return float64(t.RxTotal) / float64(GB)
}

// TxGB returns upload traffic in GB.
func (t *Tracker) TxGB() float64 {
	return float64(t.TxTotal) / float64(GB)
}

// CheckLevel returns the highest threshold level currently breached.
func (t *Tracker) CheckLevel() ThresholdLevel {
	total := t.Total()
	level := LevelNormal
	for _, th := range t.Thresholds {
		if total >= th.Bytes {
			level = th.Level
		}
	}
	return level
}

// Progress returns the fraction of the highest threshold used (0.0 to 1.0+).
func (t *Tracker) Progress() float64 {
	if len(t.Thresholds) == 0 {
		return 0
	}
	limit := t.Thresholds[len(t.Thresholds)-1].Bytes
	if limit == 0 {
		return 0
	}
	return float64(t.Total()) / float64(limit)
}

// LimitGB returns the top threshold in GB.
func (t *Tracker) LimitGB() float64 {
	if len(t.Thresholds) == 0 {
		return 0
	}
	return float64(t.Thresholds[len(t.Thresholds)-1].Bytes) / float64(GB)
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
