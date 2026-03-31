package tracker

import (
	"testing"
	"time"
)

func TestAddAndTotal(t *testing.T) {
	tr := NewWithDefaults()
	now := time.Now()
	tr.Add(1000, 500, now)
	tr.Add(2000, 300, now)

	if tr.RxTotal != 3000 {
		t.Errorf("RxTotal = %d, want 3000", tr.RxTotal)
	}
	if tr.TxTotal != 800 {
		t.Errorf("TxTotal = %d, want 800", tr.TxTotal)
	}
	if tr.Total() != 3800 {
		t.Errorf("Total = %d, want 3800", tr.Total())
	}
}

func TestDayRollover(t *testing.T) {
	tr := NewWithDefaults()
	today := time.Date(2026, 3, 31, 10, 0, 0, 0, time.Local)
	tr.Date = truncateToDay(today)

	tr.Add(1000, 500, today)
	if tr.Total() != 1500 {
		t.Fatalf("Total = %d, want 1500", tr.Total())
	}

	// Next day
	tomorrow := today.Add(24 * time.Hour)
	rolled := tr.Add(200, 100, tomorrow)
	if !rolled {
		t.Error("expected day rollover")
	}
	if tr.Total() != 300 {
		t.Errorf("Total after rollover = %d, want 300", tr.Total())
	}
}

func TestCheckLevel(t *testing.T) {
	tr := NewWithDefaults()
	now := time.Now()

	if tr.CheckLevel() != LevelNormal {
		t.Error("expected LevelNormal at 0 bytes")
	}

	// Add 101 GB
	tr.Add(101*GB, 0, now)
	if tr.CheckLevel() != LevelWarning {
		t.Errorf("expected LevelWarning at 101 GB, got %d", tr.CheckLevel())
	}

	// Reset and add 131 GB
	tr.RxTotal = 131 * GB
	tr.TxTotal = 0
	if tr.CheckLevel() != LevelDanger {
		t.Errorf("expected LevelDanger at 131 GB, got %d", tr.CheckLevel())
	}

	// 151 GB
	tr.RxTotal = 151 * GB
	if tr.CheckLevel() != LevelCritical {
		t.Errorf("expected LevelCritical at 151 GB, got %d", tr.CheckLevel())
	}
}

func TestProgress(t *testing.T) {
	tr := NewWithDefaults()
	now := time.Now()
	tr.Add(75*GB, 0, now) // 75 GB out of 150 GB = 50%
	p := tr.Progress()
	if p < 0.49 || p > 0.51 {
		t.Errorf("Progress = %f, want ~0.5", p)
	}
}

func TestCustomThreshold(t *testing.T) {
	tr := New(200) // 200 GB limit
	if tr.LimitGB() < 199.9 || tr.LimitGB() > 200.1 {
		t.Errorf("LimitGB = %f, want ~200", tr.LimitGB())
	}
}
