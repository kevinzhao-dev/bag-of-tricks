package gitlog

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"release-note/internal/model"
)

// Collector wraps git operations. We keep it small and focused on readability.
type Collector struct {
	RepoPath string
}

// CommitsBetween pulls commit metadata and touched files for a ref range.
func (c Collector) CommitsBetween(fromRef, toRef string) ([]model.Commit, error) {
	rangeSpec := fmt.Sprintf("%s..%s", fromRef, toRef)
	shas, err := c.commitSHAs(rangeSpec)
	if err != nil {
		return nil, err
	}

	var commits []model.Commit
	for _, sha := range shas {
		meta, err := c.commitMeta(sha)
		if err != nil {
			return nil, err
		}

		files, err := c.commitFiles(sha)
		if err != nil {
			return nil, err
		}
		meta.Files = files

		commits = append(commits, meta)
	}

	return commits, nil
}

func (c Collector) commitSHAs(rangeSpec string) ([]string, error) {
	cmd := exec.Command("git", "-C", c.RepoPath, "log", "--pretty=format:%H", rangeSpec)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s: %w", rangeSpec, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var shas []string
	for _, line := range lines {
		if line != "" {
			shas = append(shas, strings.TrimSpace(line))
		}
	}
	return shas, nil
}

func (c Collector) commitMeta(sha string) (model.Commit, error) {
	cmd := exec.Command(
		"git", "-C", c.RepoPath,
		"show", "-s", "--format=%H%x1f%an%x1f%s%x1f%B",
		sha,
	)
	out, err := cmd.Output()
	if err != nil {
		return model.Commit{}, fmt.Errorf("git show metadata %s: %w", sha, err)
	}

	parts := strings.SplitN(string(out), "\x1f", 4)
	commit := model.Commit{}
	if len(parts) > 0 {
		commit.SHA = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		commit.Author = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		commit.Title = strings.TrimSpace(parts[2])
	}
	if len(parts) > 3 {
		commit.Message = strings.TrimSpace(parts[3])
	}
	return commit, nil
}

func (c Collector) commitFiles(sha string) ([]string, error) {
	cmd := exec.Command("git", "-C", c.RepoPath, "show", "--name-only", "--pretty=format:", sha)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show files %s: %w", sha, err)
	}

	lines := bytes.Split(out, []byte("\n"))
	var files []string
	for _, line := range lines {
		path := strings.TrimSpace(string(line))
		if path != "" {
			files = append(files, path)
		}
	}
	return files, nil
}
