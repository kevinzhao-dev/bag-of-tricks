package prlookup

import "testing"

func TestParsePRNumber(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"Merge pull request #123 from feature/branch", 123},
		{"Refactor code (#456)", 456},
		{"No PR reference here", 0},
		{"Multiple #10 and #20", 10}, // first match wins
		{"hash symbol only #", 0},
	}

	for _, tc := range tests {
		got := parsePRNumber(tc.input)
		if got != tc.want {
			t.Fatalf("parsePRNumber(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
