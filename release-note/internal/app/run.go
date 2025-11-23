package app

import (
	"errors"
	"fmt"
	"os"

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

	if err := os.WriteFile(opts.OutputPath, []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("write release notes: %w", err)
	}
	return nil
}

func refPair(opts Options) (string, string) {
	if opts.FromTag != "" {
		return opts.FromTag, opts.ToTag
	}
	return opts.FromCommit, opts.ToCommit
}
