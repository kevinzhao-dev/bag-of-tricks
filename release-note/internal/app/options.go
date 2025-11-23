package app

import (
	"errors"
	"path/filepath"
)

// Options collect validated inputs for running the CLI.
type Options struct {
	RepoPath   string
	ConfigPath string
	OutputPath string

	FromTag    string
	ToTag      string
	FromCommit string
	ToCommit   string
}

// FlagValues mirrors the command-line flags so we can keep parsing/validation in one place.
type FlagValues struct {
	FromTag    string
	ToTag      string
	FromCommit string
	ToCommit   string
	RepoPath   string
	ConfigPath string
	OutputPath string
}

// OptionsFromFlags validates user input and resolves default values.
func OptionsFromFlags(f FlagValues) (Options, error) {
	hasTags := f.FromTag != "" || f.ToTag != ""
	hasCommits := f.FromCommit != "" || f.ToCommit != ""

	if hasTags && hasCommits {
		return Options{}, errors.New("use either tag range flags OR commit range flags, not both")
	}

	if !hasTags && !hasCommits {
		return Options{}, errors.New("provide a starting tag (--from-tag) or commit (--from-commit)")
	}

	if f.FromTag != "" && f.ToTag == "" {
		f.ToTag = "HEAD"
	}
	if f.FromCommit != "" && f.ToCommit == "" {
		f.ToCommit = "HEAD"
	}

	if f.FromTag == "" && f.ToTag != "" {
		return Options{}, errors.New("when using tags, --from-tag is required")
	}
	if f.FromCommit == "" && f.ToCommit != "" {
		return Options{}, errors.New("when using commit hashes, --from-commit is required")
	}

	return Options{
		RepoPath:   filepath.Clean(f.RepoPath),
		ConfigPath: filepath.Clean(f.ConfigPath),
		OutputPath: filepath.Clean(f.OutputPath),
		FromTag:    f.FromTag,
		ToTag:      f.ToTag,
		FromCommit: f.FromCommit,
		ToCommit:   f.ToCommit,
	}, nil
}
