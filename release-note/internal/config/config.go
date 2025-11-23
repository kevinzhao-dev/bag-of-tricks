package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// PromptConfig is kept intentionally small so it is easy to tweak for experiments.
type PromptConfig struct {
	Model            string  `json:"model"`
	SystemPrompt     string  `json:"system_prompt"`
	UserInstructions string  `json:"user_instructions"`
	Temperature      float32 `json:"temperature"`
	MaxTokens        int     `json:"max_tokens"`
}

func Load(path string) (PromptConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return PromptConfig{}, fmt.Errorf("read config: %w", err)
	}

	var cfg PromptConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return PromptConfig{}, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.3
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 1200
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = "You are an expert release-note writer. Keep outputs concise and user-facing."
	}
	if cfg.UserInstructions == "" {
		cfg.UserInstructions = "Focus on user-visible behavior changes first, then internal details. Categorize items into New Feature, Performance Improvement, Bug Fix, Internal Changes."
	}

	return cfg, nil
}
