package pv

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
}

// BuildGallery discovers image files under path recursively.
// Path may be a single file, a flat directory, or a directory of directories.
func BuildGallery(path string, latest bool) ([]string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(path))
		if !imageExts[ext] {
			return nil, fmt.Errorf("not an image file: %s", path)
		}
		// Single file: return its directory's images, like video-player
		dir := filepath.Dir(path)
		return buildFromDir(dir, latest)
	}

	return buildFromDir(path, latest)
}

func buildFromDir(dir string, latest bool) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if imageExts[ext] {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if latest {
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
		return nil, fmt.Errorf("no image files found in %s", dir)
	}
	return files, nil
}
