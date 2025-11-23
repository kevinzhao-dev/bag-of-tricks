package prompt

import (
	"fmt"
	"strings"

	"release-note/internal/config"
	"release-note/internal/model"
)

// BuildUserPrompt formats commit + PR data for the LLM.
func BuildUserPrompt(cfg config.PromptConfig, note model.ReleaseNote) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Generate release notes for changes between %s and %s.\n", note.FromRef, note.ToRef)
	fmt.Fprintf(&b, "%s\n\n", cfg.UserInstructions)
	b.WriteString("Summaries should be grouped by New Feature, Performance Improvement, Bug Fix, Internal Changes.\n")
	b.WriteString("Each bullet MUST end with the suffix format: (PR#<number>, <author>).\n")
	b.WriteString("Use the PR author when available; otherwise use the commit author. If the PR number is unknown, use PR#unknown.\n")
	b.WriteString("Do NOT include links in the bullets and do NOT add any PR list/table/section.\n\n")

	b.WriteString("Commits and PR context:\n")
	for _, c := range note.Commits {
		shortSHA := c.SHA
		if len(shortSHA) > 8 {
			shortSHA = shortSHA[:8]
		}
		fmt.Fprintf(&b, "- Commit %s by %s: %s\n", shortSHA, valueOr(c.Author, "unknown"), c.Title)
		if c.PR != nil {
			fmt.Fprintf(&b, "  PR #%d: %s (%s) by %s\n", c.PR.Number, c.PR.Title, c.PR.URL, valueOr(c.PR.Author, "unknown"))
		} else {
			b.WriteString("  PR: unknown (no PR match)\n")
		}
		if len(c.Files) > 0 {
			b.WriteString("  Files: ")
			b.WriteString(strings.Join(c.Files, ", "))
			b.WriteString("\n")
		}
		if c.Message != "" {
			b.WriteString("  Commit message:\n  ")
			b.WriteString(strings.ReplaceAll(c.Message, "\n", "\n  "))
			b.WriteString("\n")
		}
	}

	b.WriteString("\nAfter the categorized notes, include a complete PR list with number, link, original title, and affected files.\n")
	return b.String()
}

func valueOr(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}
