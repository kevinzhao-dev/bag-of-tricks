package pp

import (
	"fmt"
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

func BuildPlaylist(path string, latest bool) (files []string, startIndex int, err error) {
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
