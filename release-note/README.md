## Release-note CLI

A small, educational Go CLI that reads git history, grabs PR metadata, and asks an OpenAI model to draft release notes.

### Prerequisites
- Go 1.20+
- A GitHub repo with history available locally
- `gh` CLI authenticated (for PR lookups)
- `OPENAI_API_KEY` exported in your shell

### Quickstart
```bash
go run ./cmd/release-note \
  --from-tag v1.2.0 \
  --to-tag v1.2.3 \
  --output release-notes.md
```

Or by commit range:
```bash
go run ./cmd/release-note \
--from-commit abc1234 \
--to-commit HEAD
```

Flags:
- `--config` path to the prompt/config JSON (defaults to `prompt.json`)
- `--repo` path to the git repo (defaults to `.`)
- `--output` target Markdown file

Installation without a config file:
```bash
go install ./cmd/release-note
# built-in prompt/config defaults will be used
release-note --from-tag v1.2.0 --to-tag v1.2.3
```

### How it works
1) Collects commits in the range via `git log`, fetching file lists per commit.  
2) Attempts to find the PR for each commit via `gh` (parsing commit messages first, then falling back to `gh api repos/:owner/:repo/commits/<sha>/pulls`).  
3) Builds an LLM prompt that already carries the commit/PR context and behavior-focused instructions.  
4) Calls OpenAI's chat completions API and writes the resulting Markdown to disk.

### Tweaking the prompt
Edit `prompt.json`:
- `model`: any OpenAI chat model
- `system_prompt`: high-level tone/role
- `user_instructions`: detailed guidance for behavior-first notes
- `temperature` / `max_tokens`: generation controls
- `author_filter`: optional array of GitHub logins to keep only commits authored by those people (PR author preferred, falls back to git author)

If `prompt.json` is absent, the tool falls back to safe defaults baked into the binary.

### Output expectations
- Release notes are categorized into: New Feature, Performance Improvement, Bug Fix, Internal Changes.
- Each item ends with `(PR#<number>, <name>)` (no links in bullets).
