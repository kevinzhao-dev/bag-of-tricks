package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

type downloadResult struct {
	URL string
	OK  bool
	Msg string
}

var urlToken = regexp.MustCompile(`(https?://\S+|video\.twimg\.com/\S+)`)

func main() {
	destFlag := flag.String("dir", "~/Downloads/mobile/", "download directory")
	workersFlag := flag.Int("workers", defaultWorkers(), "number of parallel downloads")
	flag.Parse()

	destDir, err := expandPath(*destFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve download directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create download directory: %v\n", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		rawURLs, shouldQuit := promptURLs(reader)
		urls := gatherURLs(rawURLs)

		if shouldQuit && len(urls) == 0 {
			fmt.Println("Goodbye.")
			return
		}
		if len(urls) == 0 {
			fmt.Println("No URLs provided. Paste URLs or type :q to quit.")
			if shouldQuit {
				return
			}
			continue
		}

		workerCount := clampWorkers(*workersFlag, len(urls))
		fmt.Printf("Downloading %d file(s) to %s with %d worker(s)...\n", len(urls), destDir, workerCount)

		results := downloadAll(urls, destDir, workerCount)
		report(results)

		fmt.Println("Batch complete.\n")
		if shouldQuit {
			return
		}
	}
}

func defaultWorkers() int {
	cpus := runtime.NumCPU()
	if cpus < 2 {
		return 1
	}
	return cpus / 2
}

func clampWorkers(requested int, urls int) int {
	if requested < 1 {
		return 1
	}
	if requested > urls {
		return urls
	}
	return requested
}

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return filepath.Clean(path), nil
}

func promptURLs(r *bufio.Reader) ([]string, bool) {
	fmt.Println("Paste MP4 URLs (one per line). Blank lines are ignored. Type ':go' to start, ':q' to quit.")

	var urls []string
	for {
		fmt.Print("> ")
		line, err := r.ReadString('\n')
		if err != nil {
			line = strings.TrimSpace(line)
			if line != "" {
				urls = append(urls, line)
			}
			return urls, true
		}

		stripped := strings.TrimSpace(line)
		switch stripped {
		case ":q", ":quit", ":exit":
			return urls, true
		case ":go", ":start", ":run":
			return urls, false
		}
		if stripped == "" {
			continue
		}
		urls = append(urls, line)
	}
}

func gatherURLs(raw []string) []string {
	seen := make(map[string]bool)
	var cleaned []string
	for _, candidate := range raw {
		if url, ok := cleanURL(candidate); ok && !seen[url] {
			seen[url] = true
			cleaned = append(cleaned, url)
		}
	}
	return cleaned
}

func cleanURL(raw string) (string, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", false
	}

	match := urlToken.FindString(text)
	if match == "" {
		return "", false
	}
	candidate := strings.Trim(match, "><()[]{}.,;:\"'`")

	if !strings.HasPrefix(candidate, "http://") && !strings.HasPrefix(candidate, "https://") {
		candidate = "https://" + strings.TrimLeft(candidate, "/")
	}

	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" {
		return "", false
	}

	query := parsed.Query()
	if len(query) > 0 {
		delete(query, "tag")
		parsed.RawQuery = query.Encode()
	}
	parsed.Fragment = ""

	normalized := parsed.String()
	normalized = strings.TrimSuffix(normalized, "?")
	return normalized, true
}

func downloadAll(urls []string, destDir string, workers int) []downloadResult {
	if workers <= 1 {
		results := make([]downloadResult, 0, len(urls))
		for _, u := range urls {
			results = append(results, downloadOne(u, destDir))
		}
		return results
	}

	// Buffer channels so fast workers don't block when the main goroutine
	// hasn't started reading from results yet.
	jobs := make(chan string, len(urls))
	results := make(chan downloadResult, len(urls))
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				results <- downloadOne(u, destDir)
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for _, u := range urls {
		jobs <- u
	}
	close(jobs)

	var collected []downloadResult
	for res := range results {
		collected = append(collected, res)
	}
	return collected
}

func downloadOne(targetURL, destDir string) downloadResult {
	cmd := exec.Command("wget", "-c", "-P", destDir, targetURL)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return downloadResult{URL: targetURL, OK: true, Msg: "ok"}
	}

	if isNotFound(err) {
		return downloadResult{URL: targetURL, OK: false, Msg: "wget not found; install wget and retry"}
	}

	msg := strings.TrimSpace(string(output))
	if msg == "" {
		msg = err.Error()
	}
	return downloadResult{URL: targetURL, OK: false, Msg: msg}
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
		return true
	}
	return false
}

func report(results []downloadResult) {
	var success []string
	var failed []downloadResult
	for _, res := range results {
		if res.OK {
			success = append(success, res.URL)
			continue
		}
		failed = append(failed, res)
	}

	if len(success) > 0 {
		fmt.Printf("Downloaded %d file(s).\n", len(success))
	}
	if len(failed) > 0 {
		fmt.Printf("Failed %d file(s):\n", len(failed))
		for _, res := range failed {
			fmt.Printf("- %s :: %s\n", res.URL, res.Msg)
		}
	}
}
