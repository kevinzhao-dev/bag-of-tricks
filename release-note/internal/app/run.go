package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"release-note/internal/config"
	"release-note/internal/gitlog"
	"release-note/internal/model"
	"release-note/internal/openai"
	"release-note/internal/prlookup"
	"release-note/internal/prompt"
)

// Run orchestrates the full workflow: git data -> PR enrichment -> LLM -> Markdown output.
func Run(opts Options) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	fromRef, toRef := refPair(opts)

	collector := gitlog.Collector{RepoPath: opts.RepoPath}
	commits, err := collector.CommitsBetween(fromRef, toRef)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return errors.New("no commits found in the specified range")
	}

	lookup := prlookup.Lookup{RepoPath: opts.RepoPath}
	commits, prErrs := lookup.AttachPRInfo(commits)
	for _, warn := range prErrs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", warn)
	}

	if len(cfg.AuthorFilter) > 0 {
		commits = filterCommitsByAuthor(commits, cfg.AuthorFilter)
		if len(commits) == 0 {
			return errors.New("no commits found after applying author filter")
		}
	}

	release := model.ReleaseNote{
		FromRef: fromRef,
		ToRef:   toRef,
		Commits: commits,
	}

	userPrompt := prompt.BuildUserPrompt(cfg, release)

	client, err := openai.NewClientFromEnv(cfg)
	if err != nil {
		return err
	}

	markdown, err := client.Generate(cfg.SystemPrompt, userPrompt)
	if err != nil {
		return err
	}

	markdown = sanitizeMarkdown(markdown)

	if err := os.WriteFile(opts.OutputPath, []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("write release notes: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Release notes saved to %s\n", absPath(opts.OutputPath))
	return nil
}

func refPair(opts Options) (string, string) {
	if opts.FromTag != "" {
		return opts.FromTag, opts.ToTag
	}
	return opts.FromCommit, opts.ToCommit
}

// filterCommitsByAuthor keeps commits whose PR author (preferred) or commit author
// matches one of the allowed GitHub logins (case-insensitive).
func filterCommitsByAuthor(commits []model.Commit, allowed []string) []model.Commit {
	if len(allowed) == 0 {
		return commits
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[strings.ToLower(strings.TrimSpace(a))] = struct{}{}
	}

	var filtered []model.Commit
	for _, c := range commits {
		switch {
		case c.PR != nil && matchAuthor(c.PR.Author, allowedSet):
			filtered = append(filtered, c)
		case matchAuthor(c.Author, allowedSet):
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func matchAuthor(author string, allowed map[string]struct{}) bool {
	if author == "" {
		return false
	}
	_, ok := allowed[strings.ToLower(strings.TrimSpace(author))]
	return ok
}

func absPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func valueOr(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

// sanitizeMarkdown removes any PR list headings/tables the model might have hallucinated.
func sanitizeMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var out []string
	skipping := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if isPRListHeading(trim) {
			skipping = true
			continue
		}
		if skipping {
			// stop skipping when we hit another heading
			if strings.HasPrefix(trim, "#") {
				skipping = false
				// fall through to process this heading normally
			} else {
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func isPRListHeading(line string) bool {
	if !strings.HasPrefix(line, "#") {
		return false
	}
	l := strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "#")))
	return strings.Contains(l, "pr list")
}
