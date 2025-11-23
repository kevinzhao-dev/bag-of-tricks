package app

import "testing"

func TestSanitizeMarkdownRemovesPRListSection(t *testing.T) {
	input := `# Release Notes

## New Feature
- Item (PR#1, alice)

### PR List
1. something
2. more

## Bug Fix
- Fix (PR#2, bob)`

	want := `# Release Notes

## New Feature
- Item (PR#1, alice)

## Bug Fix
- Fix (PR#2, bob)`

	got := sanitizeMarkdown(input)
	if got != want {
		t.Fatalf("unexpected sanitize output.\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
