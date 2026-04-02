package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

var videoExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".wmv": true,
}

func main() {
	godotenv.Load()

	defaultDir := envOrDefault("DOWNLOAD_DIR", "~/Downloads")
	defaultDB := envOrDefault("DB_PATH", "~/Downloads/video_catalog.db")

	dir := flag.String("dir", defaultDir, "directory to scan for video files")
	dbPath := flag.String("db", defaultDB, "path to video_catalog.db")
	flag.Parse()

	scanDir := expandHome(*dir)
	dbFile := expandHome(*dbPath)

	// Copy DB to temp file to avoid iCloud/journal issues
	tmpFile, err := copyToTemp(dbFile)
	if err != nil {
		log.Fatalf("Failed to copy database to temp: %v", err)
	}
	defer os.Remove(tmpFile)

	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	entries, err := os.ReadDir(scanDir)
	if err != nil {
		log.Fatalf("Failed to read directory %s: %v", scanDir, err)
	}

	var scanned, duplicates, newFiles, unparseable int

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !videoExtensions[ext] {
			continue
		}
		scanned++

		code, ok := ParseVideoCode(entry.Name())
		if !ok {
			unparseable++
			continue
		}

		rows, err := db.Query("SELECT filename, filepath, file_size FROM videos WHERE parsed_code = ?", code)
		if err != nil {
			log.Printf("DB query error for %s: %v", code, err)
			continue
		}

		var matches []dbMatch
		for rows.Next() {
			var m dbMatch
			rows.Scan(&m.Filename, &m.Filepath, &m.FileSize)
			matches = append(matches, m)
		}
		rows.Close()

		if len(matches) == 0 {
			newFiles++
			continue
		}

		duplicates++
		info, _ := entry.Info()
		dlSize := info.Size()

		fmt.Printf("\033[33m[DUPLICATE]\033[0m %s\n", entry.Name())
		fmt.Printf("  Code:     %s\n", code)
		for _, m := range matches {
			fmt.Printf("  Exists:   %s\n", m.Filepath)
			sizeNote := compareSizes(dlSize, m.FileSize)
			fmt.Printf("  Size:     %s (download) vs %s (existing)  %s\n",
				formatSize(dlSize), formatSize(m.FileSize), sizeNote)
		}
		fmt.Println()
	}

	fmt.Println("--- Summary ---")
	fmt.Printf("Scanned: %d | Duplicates: %d | New: %d | Unparseable: %d\n",
		scanned, duplicates, newFiles, unparseable)
}

type dbMatch struct {
	Filename string
	Filepath string
	FileSize int64
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func copyToTemp(src string) (string, error) {
	tmp, err := os.CreateTemp("", "video_catalog-*.db")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	f, err := os.Open(src)
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(tmp, f); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func formatSize(bytes int64) string {
	const (
		GB = 1024 * 1024 * 1024
		MB = 1024 * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func compareSizes(dlSize, existingSize int64) string {
	diff := dlSize - existingSize
	switch {
	case diff > 0:
		return fmt.Sprintf("\033[32m** download is larger (+%s) **\033[0m", formatSize(diff))
	case diff < 0:
		return fmt.Sprintf("existing is larger (+%s)", formatSize(-diff))
	default:
		return "same size"
	}
}
