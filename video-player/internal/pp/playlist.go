package pp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var videoExts = map[string]bool{
	".mp4":  true,
	".avi":  true,
	".mkv":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".webm": true,
	".m4v":  true,
}

func BuildPlaylist(path string, latest, recursive bool) (files []string, startIndex int, err error) {
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}

	dir := path
	startFile := ""
	if !info.IsDir() {
		dir = filepath.Dir(path)
		startFile = path
	}

	if recursive {
		err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && p != dir {
					return fs.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if videoExts[ext] {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	} else {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, 0, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if !videoExts[ext] {
				continue
			}
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}

	if latest {
		// Sort by modification time, most recent first
		sort.Slice(files, func(i, j int) bool {
			infoI, errI := os.Stat(files[i])
			infoJ, errJ := os.Stat(files[j])
			if errI != nil || errJ != nil {
				return files[i] < files[j]
			}
			return infoI.ModTime().After(infoJ.ModTime())
		})
	} else {
		sort.Strings(files)
	}
	if len(files) == 0 {
		return nil, 0, fmt.Errorf("no video files found in %s", dir)
	}
	if startFile != "" {
		for i, f := range files {
			if f == startFile {
				return files, i, nil
			}
		}
	}
	return files, 0, nil
}

// BuildDisplayNames returns relative paths for display. For non-recursive mode
// this produces basenames; for recursive mode it shows subfolder/file.mp4.
func BuildDisplayNames(files []string, baseDir string) []string {
	names := make([]string, len(files))
	for i, f := range files {
		rel, err := filepath.Rel(baseDir, f)
		if err != nil {
			rel = filepath.Base(f)
		}
		names[i] = rel
	}
	return names
}
