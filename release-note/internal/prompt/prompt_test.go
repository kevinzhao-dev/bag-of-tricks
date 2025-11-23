package prompt

import (
	"strings"
	"testing"

	"release-note/internal/config"
	"release-note/internal/model"
)

func TestBuildUserPromptIncludesKeySections(t *testing.T) {
	cfg := config.PromptConfig{
		UserInstructions: "Please summarize behavior changes.",
	}
	note := model.ReleaseNote{
		FromRef: "v1.0.0",
		ToRef:   "HEAD",
		Commits: []model.Commit{
			{
				SHA:    "abcdef123456",
				Author: "alice",
				Title:  "feat: add payments",
				Files:  []string{"payments/service.go", "payments/api.go"},
				PR: &model.PRInfo{
					Number: 42,
					Title:  "Add payment processing",
					URL:    "https://github.com/example/repo/pull/42",
					Author: "alice",
				},
				Message: "Implement payment flow\nImprove validation",
			},
		},
	}

	out := BuildUserPrompt(cfg, note)

	for _, snippet := range []string{
		"Generate release notes for changes between v1.0.0 and HEAD",
		"Please summarize behavior changes.",
		"New Feature, Performance Improvement, Bug Fix, Internal Changes",
		"(PR#<number>, <author>)",
		"PR #42: Add payment processing",
		"payments/service.go",
		"Implement payment flow",
	} {
		if !strings.Contains(out, snippet) {
			t.Fatalf("prompt missing expected content: %q", snippet)
		}
	}
}
