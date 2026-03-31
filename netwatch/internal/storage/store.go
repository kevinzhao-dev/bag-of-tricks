package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	dirName        = ".netwatch"
	fileName       = "history.json"
	checkpointFile = "checkpoint.json"
)

// DayRecord holds traffic data for a single day.
type DayRecord struct {
	Date    string `json:"date"`    // "2006-01-02"
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

// TotalBytes returns rx + tx.
func (r DayRecord) TotalBytes() uint64 {
	return r.RxBytes + r.TxBytes
}

// Checkpoint stores the last known OS counters and accumulated totals,
// so traffic during program downtime can be recovered on restart.
type Checkpoint struct {
	Date       string `json:"date"`        // "2006-01-02"
	RxAbs      uint64 `json:"rx_abs"`      // last seen OS absolute counter
	TxAbs      uint64 `json:"tx_abs"`      // last seen OS absolute counter
	RxAccum    uint64 `json:"rx_accum"`    // accumulated rx for the day
	TxAccum    uint64 `json:"tx_accum"`    // accumulated tx for the day
}

// Store manages the history JSON file.
type Store struct {
	path       string
	cpPath     string // checkpoint file path
}

// New creates a Store. It ensures the directory exists.
func New() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{
		path:   filepath.Join(dir, fileName),
		cpPath: filepath.Join(dir, checkpointFile),
	}, nil
}

// Load reads all records from disk.
func (s *Store) Load() ([]DayRecord, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []DayRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// Save writes today's record, updating if it already exists.
// It keeps only the last 30 days.
func (s *Store) Save(rec DayRecord) error {
	records, err := s.Load()
	if err != nil {
		records = nil // start fresh if corrupt
	}

	// Update or append
	found := false
	for i, r := range records {
		if r.Date == rec.Date {
			records[i] = rec
			found = true
			break
		}
	}
	if !found {
		records = append(records, rec)
	}

	// Sort by date
	sort.Slice(records, func(i, j int) bool {
		return records[i].Date < records[j].Date
	})

	// Keep last 30 days only
	if len(records) > 30 {
		records = records[len(records)-30:]
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// Recent returns the last n days of records.
func (s *Store) Recent(n int) ([]DayRecord, error) {
	records, err := s.Load()
	if err != nil {
		return nil, err
	}
	if len(records) <= n {
		return records, nil
	}
	return records[len(records)-n:], nil
}

// ConsecutiveDaysOver returns how many consecutive recent days exceeded the limit (in bytes),
// counting backwards from yesterday. Also returns the dates of those days.
func (s *Store) ConsecutiveDaysOver(limitBytes uint64) (int, []string, error) {
	records, err := s.Load()
	if err != nil {
		return 0, nil, err
	}
	if len(records) == 0 {
		return 0, nil, nil
	}

	// Walk backwards from most recent record
	today := time.Now().Format("2006-01-02")
	count := 0
	var dates []string
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		if r.Date == today {
			continue // don't count today, it's still in progress
		}
		if r.TotalBytes() > limitBytes {
			count++
			dates = append(dates, r.Date)
		} else {
			break // streak broken
		}
	}
	return count, dates, nil
}

// LoadCheckpoint reads the checkpoint file.
func (s *Store) LoadCheckpoint() (*Checkpoint, error) {
	data, err := os.ReadFile(s.cpPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, nil // treat corrupt file as missing
	}
	return &cp, nil
}

// SaveCheckpoint writes the checkpoint file.
func (s *Store) SaveCheckpoint(cp Checkpoint) error {
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.cpPath, data, 0644)
}
