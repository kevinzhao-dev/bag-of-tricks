package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return &Store{
		path:   filepath.Join(dir, "history.json"),
		cpPath: filepath.Join(dir, "checkpoint.json"),
	}
}

func TestSaveAndLoad(t *testing.T) {
	s := tempStore(t)

	err := s.Save(DayRecord{Date: "2026-03-30", RxBytes: 100_000_000_000, TxBytes: 20_000_000_000})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Save(DayRecord{Date: "2026-03-31", RxBytes: 50_000_000_000, TxBytes: 10_000_000_000})
	if err != nil {
		t.Fatal(err)
	}

	records, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].Date != "2026-03-30" {
		t.Errorf("first record date = %s, want 2026-03-30", records[0].Date)
	}
}

func TestSaveUpdatesExisting(t *testing.T) {
	s := tempStore(t)
	s.Save(DayRecord{Date: "2026-03-31", RxBytes: 100, TxBytes: 50})
	s.Save(DayRecord{Date: "2026-03-31", RxBytes: 200, TxBytes: 100})

	records, _ := s.Load()
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].RxBytes != 200 {
		t.Errorf("RxBytes = %d, want 200", records[0].RxBytes)
	}
}

func TestRecent(t *testing.T) {
	s := tempStore(t)
	for i := 1; i <= 10; i++ {
		s.Save(DayRecord{Date: "2026-03-" + pad(i), RxBytes: uint64(i) * 1000})
	}

	recent, err := s.Recent(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 3 {
		t.Fatalf("got %d, want 3", len(recent))
	}
	if recent[0].Date != "2026-03-08" {
		t.Errorf("first recent = %s, want 2026-03-08", recent[0].Date)
	}
}

func TestKeeps30Days(t *testing.T) {
	s := tempStore(t)
	for i := 1; i <= 35; i++ {
		s.Save(DayRecord{Date: "2026-01-" + pad(i%31+1), RxBytes: uint64(i)})
	}
	records, _ := s.Load()
	if len(records) > 30 {
		t.Errorf("got %d records, want <= 30", len(records))
	}
}

func TestLoadMissingFile(t *testing.T) {
	s := &Store{path: filepath.Join(os.TempDir(), "nonexistent_test_file.json")}
	records, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Errorf("expected nil records for missing file")
	}
}

func TestCheckpointSaveAndLoad(t *testing.T) {
	s := tempStore(t)
	cp := Checkpoint{
		Date:    "2026-03-31",
		RxAbs:   500_000_000_000,
		TxAbs:   100_000_000_000,
		RxAccum: 80_000_000_000,
		TxAccum: 15_000_000_000,
	}
	if err := s.SaveCheckpoint(cp); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected checkpoint, got nil")
	}
	if loaded.Date != cp.Date || loaded.RxAbs != cp.RxAbs || loaded.RxAccum != cp.RxAccum {
		t.Errorf("checkpoint mismatch: got %+v, want %+v", loaded, cp)
	}
}

func TestCheckpointMissingFile(t *testing.T) {
	s := &Store{
		path:   filepath.Join(os.TempDir(), "nonexistent_history.json"),
		cpPath: filepath.Join(os.TempDir(), "nonexistent_cp.json"),
	}
	cp, err := s.LoadCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if cp != nil {
		t.Error("expected nil checkpoint for missing file")
	}
}

func pad(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
