package prlookup

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"release-note/internal/model"
)

// Lookup enriches commits with pull request metadata using the GitHub CLI when available.
type Lookup struct {
	RepoPath string
}

// AttachPRInfo tries to find PR metadata for each commit. Failures are reported but do not stop processing.
func (l Lookup) AttachPRInfo(commits []model.Commit) ([]model.Commit, []error) {
	var errs []error

	for i, c := range commits {
		pr, err := l.findPR(c)
		if err != nil {
			errs = append(errs, fmt.Errorf("commit %s: %w", c.SHA, err))
		}
		if pr != nil {
			commits[i].PR = pr
		}
	}

	return commits, errs
}

func (l Lookup) findPR(c model.Commit) (*model.PRInfo, error) {
	if prNumber := parsePRNumber(c.Title + "\n" + c.Message); prNumber != 0 {
		return l.fetchPRByNumber(prNumber)
	}

	prNumber, err := l.prNumberFromCommit(c.SHA)
	if err != nil || prNumber == 0 {
		return nil, err
	}
	return l.fetchPRByNumber(prNumber)
}

func (l Lookup) fetchPRByNumber(number int) (*model.PRInfo, error) {
	cmd := exec.Command(
		"gh", "pr", "view", fmt.Sprintf("%d", number),
		"--json", "number,title,url,author",
	)
	cmd.Dir = l.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr view #%d: %w", number, err)
	}

	var parsed struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		URL    string `json:"url"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse gh pr view response: %w", err)
	}

	return &model.PRInfo{
		Number: parsed.Number,
		Title:  parsed.Title,
		URL:    parsed.URL,
		Author: parsed.Author.Login,
	}, nil
}

func (l Lookup) prNumberFromCommit(sha string) (int, error) {
	// The :owner/:repo placeholders let GH infer the repo from the current directory.
	cmd := exec.Command(
		"gh", "api",
		fmt.Sprintf("repos/:owner/:repo/commits/%s/pulls", sha),
		"--jq", ".[0].number",
	)
	cmd.Dir = l.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("gh api lookup for commit %s: %w", sha, err)
	}

	numStr := strings.TrimSpace(string(out))
	if numStr == "null" || numStr == "" {
		return 0, nil
	}
	return strconv.Atoi(numStr)
}

var prNumberPattern = regexp.MustCompile(`#(\d+)`)

func parsePRNumber(text string) int {
	match := prNumberPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return n
}
