package app

import (
	"release-note/internal/model"
	"testing"
)

func TestFilterCommitsByAuthor(t *testing.T) {
	commits := []model.Commit{
		{
			SHA:    "1",
			Author: "alice",
			PR: &model.PRInfo{
				Author: "alice",
			},
		},
		{
			SHA:    "2",
			Author: "bob",
			PR: &model.PRInfo{
				Author: "charlie",
			},
		},
		{
			SHA:    "3",
			Author: "dave",
		},
	}

	filtered := filterCommitsByAuthor(commits, []string{"alice", "dave"})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 commits after filter, got %d", len(filtered))
	}
	if filtered[0].SHA != "1" || filtered[1].SHA != "3" {
		t.Fatalf("unexpected commits after filter: %+v", filtered)
	}
}
