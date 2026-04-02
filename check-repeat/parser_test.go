package main

import "testing"

func TestParseVideoCode(t *testing.T) {
	tests := []struct {
		filename string
		wantCode string
		wantOK   bool
	}{
		// FC2PPV explicit
		{"fc2ppv-4550644.mp4", "FC2PPV-4550644", true},
		{"FC2PPV-4595572.mp4", "FC2PPV-4595572", true},
		{"fc2ppv-4597468.mp4", "FC2PPV-4597468", true},
		{"FC2PPV-4752215-1.mp4", "FC2PPV-4752215", true},
		{"FC2PPV-4868790-2.mp4", "FC2PPV-4868790", true},
		{"FC2-PPV-4663619.mp4", "FC2PPV-4663619", true},

		// FC2 shorthand
		{"fc2643836.mp4", "FC2PPV-643836", true},

		// FC shorthand
		{"fc4765606.mp4", "FC2PPV-4765606", true},

		// Standard JAV codes
		{"SONE-912.mp4", "SONE-912", true},
		{"MAAN-1041.mp4", "MAAN-1041", true},
		{"ABF-114.mp4", "ABF-114", true},
		{"230ORECO-979.mp4", "230ORECO-979", true},
		{"543TAXD-028.mp4", "543TAXD-028", true},

		// Unparseable
		{"random-document.pdf", "", false},
		{"notes.txt", "", false},
		{"12345.mp4", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			code, ok := ParseVideoCode(tt.filename)
			if ok != tt.wantOK {
				t.Errorf("ParseVideoCode(%q) ok = %v, want %v", tt.filename, ok, tt.wantOK)
			}
			if code != tt.wantCode {
				t.Errorf("ParseVideoCode(%q) = %q, want %q", tt.filename, code, tt.wantCode)
			}
		})
	}
}
