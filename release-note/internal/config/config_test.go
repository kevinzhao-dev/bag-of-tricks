package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFallsBackToDefaultsWhenMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SystemPrompt == "" || cfg.UserInstructions == "" || cfg.Model == "" {
		t.Fatalf("expected defaults to be populated, got %+v", cfg)
	}
}

func TestLoadOverridesDefaultsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	json := `{
		"model": "gpt-xyz",
		"author_filter": ["alice", "bob"],
		"user_instructions": "custom text"
	}`
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Model != "gpt-xyz" {
		t.Fatalf("expected model override, got %q", cfg.Model)
	}
	if len(cfg.AuthorFilter) != 2 || cfg.AuthorFilter[0] != "alice" {
		t.Fatalf("expected author_filter to be read, got %+v", cfg.AuthorFilter)
	}
	if cfg.UserInstructions != "custom text" {
		t.Fatalf("expected user_instructions override, got %q", cfg.UserInstructions)
	}
}
