package model

// PRInfo captures metadata about a pull request.
type PRInfo struct {
	Number int
	Title  string
	URL    string
	Author string
}

// Commit represents a git commit plus optional PR context.
type Commit struct {
	SHA     string
	Title   string
	Author  string
	Files   []string
	PR      *PRInfo
	Message string
}

// ReleaseNote bundles the pieces we will render to Markdown and feed to the LLM.
type ReleaseNote struct {
	FromRef string
	ToRef   string
	Commits []Commit
}
