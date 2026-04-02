package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Priority 1: explicit FC2PPV prefix (case-insensitive)
	// Matches: fc2ppv-4550644, FC2-PPV-4595572, fc2-ppv-4752215-1
	reFC2PPV = regexp.MustCompile(`(?i)^fc2-?ppv-?(\d+)`)

	// Priority 2: fc2 shorthand (no "ppv")
	// Matches: fc2643836
	reFC2Short = regexp.MustCompile(`(?i)^fc2(\d{5,})`)

	// Priority 3: fc shorthand (just "fc" followed by many digits)
	// Matches: fc4765606
	reFCShort = regexp.MustCompile(`(?i)^fc(\d{7,})`)

	// Priority 4: standard JAV code (optional numeric prefix + alpha + hyphen + number)
	// Matches: SONE-912, MAAN-1041, 230ORECO-979
	reStandard = regexp.MustCompile(`(?i)^(\d*[a-zA-Z]+)-(\d+)`)
)

// ParseVideoCode extracts a normalized video code from a filename.
// Returns the code (e.g. "FC2PPV-4663619") and true, or "" and false if unparseable.
func ParseVideoCode(filename string) (string, bool) {
	// Strip extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Priority 1: FC2PPV explicit
	if m := reFC2PPV.FindStringSubmatch(name); m != nil {
		return "FC2PPV-" + m[1], true
	}

	// Priority 2: FC2 shorthand
	if m := reFC2Short.FindStringSubmatch(name); m != nil {
		return "FC2PPV-" + m[1], true
	}

	// Priority 3: FC shorthand
	if m := reFCShort.FindStringSubmatch(name); m != nil {
		return "FC2PPV-" + m[1], true
	}

	// Priority 4: standard JAV code
	if m := reStandard.FindStringSubmatch(name); m != nil {
		prefix := strings.ToUpper(m[1])
		number := m[2]
		return prefix + "-" + number, true
	}

	return "", false
}
