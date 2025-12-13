package pp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type TimestampStore struct {
	path string // empty => in-memory only (no persistence)
	m    map[string]float64
}

func NewTimestampStore(path string) *TimestampStore {
	return &TimestampStore{
		path: path,
		m:    map[string]float64{},
	}
}

func (t *TimestampStore) Load() error {
	if t == nil || t.path == "" {
		return nil
	}
	b, err := os.ReadFile(t.path)
	if err != nil {
		return nil
	}
	_ = json.Unmarshal(b, &t.m)
	if t.m == nil {
		t.m = map[string]float64{}
	}
	return nil
}

func (t *TimestampStore) Save() error {
	if t == nil || t.path == "" {
		return nil
	}
	tmp := t.path + ".tmp"
	b, err := json.MarshalIndent(t.m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.path)
}

func (t *TimestampStore) Get(path string) (float64, bool) {
	if t == nil {
		return 0, false
	}
	v, ok := t.m[path]
	return v, ok
}

func (t *TimestampStore) Set(path string, sec float64) {
	if t == nil {
		return
	}
	if t.m == nil {
		t.m = map[string]float64{}
	}
	t.m[path] = sec
}

func DefaultTimestampPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pp_timestamps_go.json")
}
