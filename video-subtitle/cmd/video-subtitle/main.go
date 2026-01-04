package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	defaultWhisperModel     = "whisper-1"
	defaultTranslateModel   = "gpt-4o-mini"
	defaultSourceLang       = "ja"
	defaultTargetLang       = "zh-TW"
	defaultChunkSeconds     = 600
	defaultMaxAudioMB       = 24
	defaultTranslateWorkers = 4
	defaultTimeoutSeconds   = 900
	maxRetries              = 4
	baseRetryDelay          = 1 * time.Second
	maxRetryDelay           = 20 * time.Second
)

type Segment struct {
	Start float64
	End   float64
	Text  string
}

type apiError struct {
	StatusCode int
	Message    string
	Type       string
	Code       string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("openai error (%d)", e.StatusCode)
	}
	return fmt.Sprintf("openai error (%d): %s", e.StatusCode, e.Message)
}

type openAIClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func newOpenAIClient(apiKey string, timeout time.Duration) *openAIClient {
	baseURL := strings.TrimRight(os.Getenv("OPENAI_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if timeout <= 0 {
		timeout = time.Duration(defaultTimeoutSeconds) * time.Second
	}
	return &openAIClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type transcriptionResponse struct {
	Text     string                 `json:"text"`
	Segments []transcriptionSegment `json:"segments"`
}

type transcriptionSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (c *openAIClient) do(req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "video-subtitle/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(resp.StatusCode, body)
	}
	return body, nil
}

func (c *openAIClient) Transcribe(ctx context.Context, audioPath, model, language string) ([]Segment, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("model", model); err != nil {
		return nil, err
	}
	if language != "" {
		if err := writer.WriteField("language", language); err != nil {
			return nil, err
		}
	}
	if err := writer.WriteField("response_format", "verbose_json"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("timestamp_granularities[]", "segment"); err != nil {
		return nil, err
	}

	file, err := os.Open(audioPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	body, err := c.do(req)
	if err != nil {
		return nil, err
	}

	var resp transcriptionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	segments := make([]Segment, 0, len(resp.Segments))
	for _, seg := range resp.Segments {
		segments = append(segments, Segment{
			Start: seg.Start,
			End:   seg.End,
			Text:  seg.Text,
		})
	}
	if len(segments) == 0 && strings.TrimSpace(resp.Text) != "" {
		segments = append(segments, Segment{Start: 0, End: 0, Text: resp.Text})
	}
	return segments, nil
}

func (c *openAIClient) Translate(ctx context.Context, model, sourceLang, targetLang, text string) (string, error) {
	systemPrompt := "You are a precise translator. Return only the translation."
	userPrompt := fmt.Sprintf(
		"Translate the following text from %s to %s. Preserve punctuation and line breaks.\n\n%s",
		sourceLang,
		targetLang,
		text,
	)

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	var resp chatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("translation returned no choices")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func parseAPIError(statusCode int, body []byte) error {
	var resp openAIErrorResponse
	if err := json.Unmarshal(body, &resp); err == nil && resp.Error.Message != "" {
		return &apiError{
			StatusCode: statusCode,
			Message:    resp.Error.Message,
			Type:       resp.Error.Type,
			Code:       resp.Error.Code,
		}
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return &apiError{StatusCode: statusCode, Message: message}
}

func describeError(err error) string {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		errType := apiErr.Type
		if errType == "" {
			errType = "api_error"
		}
		return fmt.Sprintf("%s (%d)", errType, apiErr.StatusCode)
	}
	return err.Error()
}

func retry(
	ctx context.Context,
	maxRetries int,
	baseDelay time.Duration,
	maxDelay time.Duration,
	shouldRetry func(error) bool,
	onRetry func(int, time.Duration, error),
	fn func() error,
) error {
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := fn()
		if err == nil {
			return nil
		}
		if attempt >= maxRetries {
			return err
		}
		if shouldRetry != nil && !shouldRetry(err) {
			return err
		}
		delay := baseDelay * time.Duration(1<<attempt)
		if delay > maxDelay {
			delay = maxDelay
		}
		jitter := time.Duration(float64(delay) * (0.25 * randFloat()))
		delay += jitter
		if onRetry != nil {
			onRetry(attempt+1, delay, err)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func randFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

func isRetryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 408, 409, 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

func shouldFallbackToChunking(err error) bool {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		message := strings.ToLower(apiErr.Message)
		if strings.Contains(message, "reading your request") {
			return true
		}
		if strings.Contains(strings.ToLower(apiErr.Type), "invalid_request_error") {
			return true
		}
		if apiErr.StatusCode == 413 {
			return true
		}
		if apiErr.StatusCode >= 500 {
			return true
		}
	}
	return false
}

func informativeRuneCount(text string) int {
	count := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			count++
		}
	}
	return count
}

func isLowInfoText(text string, minChars int) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	if minChars <= 0 {
		return false
	}
	return informativeRuneCount(trimmed) < minChars
}

func countTranslatableSegments(segments []Segment, minChars int) int {
	total := 0
	for _, seg := range segments {
		if !isLowInfoText(seg.Text, minChars) {
			total++
		}
	}
	return total
}

func formatSRTTimestamp(seconds float64) string {
	millis := int64(seconds * 1000)
	hours := millis / 3600000
	millis %= 3600000
	minutes := millis / 60000
	millis %= 60000
	secs := millis / 1000
	millis %= 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, millis)
}

func writeSRT(segments []Segment, outputPath string) error {
	var buf strings.Builder
	for idx, seg := range segments {
		start := formatSRTTimestamp(seg.Start)
		end := formatSRTTimestamp(seg.End)
		buf.WriteString(strconv.Itoa(idx + 1))
		buf.WriteString("\n")
		buf.WriteString(start)
		buf.WriteString(" --> ")
		buf.WriteString(end)
		buf.WriteString("\n")
		buf.WriteString(strings.TrimSpace(seg.Text))
		buf.WriteString("\n\n")
	}
	return os.WriteFile(outputPath, []byte(buf.String()), 0644)
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s failed: %s", name, message)
	}
	return nil
}

func runCommandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s failed: %s", name, message)
	}
	return stdout.String(), nil
}

func extractAudio(inputPath, outputPath string) error {
	return runCommand(
		"ffmpeg",
		"-y",
		"-i",
		inputPath,
		"-vn",
		"-ac",
		"1",
		"-ar",
		"16000",
		"-f",
		"wav",
		outputPath,
	)
}

func extractAudioSegment(inputPath, outputPath string, startSeconds, durationSeconds float64, accurate bool) error {
	args := []string{"-y"}
	if !accurate {
		args = append(args, "-ss", fmt.Sprintf("%.3f", startSeconds))
	}
	args = append(args, "-i", inputPath)
	if accurate {
		args = append(args, "-ss", fmt.Sprintf("%.3f", startSeconds))
	}
	args = append(
		args,
		"-t",
		fmt.Sprintf("%.3f", durationSeconds),
		"-vn",
		"-ac",
		"1",
		"-ar",
		"16000",
		"-f",
		"wav",
		outputPath,
	)
	return runCommand("ffmpeg", args...)
}

func audioDuration(path string) (float64, error) {
	output, err := runCommandOutput(
		"ffprobe",
		"-v",
		"error",
		"-show_entries",
		"format=duration",
		"-of",
		"default=noprint_wrappers=1:nokey=1",
		path,
	)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(output)
	duration, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse audio duration: %w", err)
	}
	return duration, nil
}

func audioSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func chooseChunkSeconds(path string, defaultChunk int, maxAudioBytes int64) (int, error) {
	duration, err := audioDuration(path)
	if err != nil {
		return defaultChunk, err
	}
	if duration <= 0 {
		return defaultChunk, nil
	}
	sizeBytes, err := audioSize(path)
	if err != nil {
		return defaultChunk, err
	}
	bytesPerSecond := float64(sizeBytes) / duration
	if bytesPerSecond <= 0 {
		return defaultChunk, nil
	}
	estimated := int(float64(maxAudioBytes) / bytesPerSecond)
	if estimated <= 0 {
		return defaultChunk, nil
	}
	if estimated < 30 {
		return 30, nil
	}
	if estimated > defaultChunk {
		return defaultChunk, nil
	}
	return estimated, nil
}

func transcribeWithRetry(
	ctx context.Context,
	client *openAIClient,
	audioPath, model, language string,
	logf func(string, ...any),
) ([]Segment, error) {
	var segments []Segment
	var err error
	retryErr := retry(
		ctx,
		maxRetries,
		baseRetryDelay,
		maxRetryDelay,
		isRetryable,
		func(attempt int, delay time.Duration, err error) {
			logf("Transcription failed; retrying in %.1fs (attempt %d). %s", delay.Seconds(), attempt, describeError(err))
		},
		func() error {
			segments, err = client.Transcribe(ctx, audioPath, model, language)
			return err
		},
	)
	if retryErr != nil {
		return nil, retryErr
	}
	return segments, nil
}

func transcribeInChunks(
	ctx context.Context,
	client *openAIClient,
	audioPath, model, language string,
	chunkSeconds int,
	accurate bool,
	logf func(string, ...any),
) ([]Segment, error) {
	duration, err := audioDuration(audioPath)
	if err != nil {
		return nil, err
	}
	if duration <= 0 {
		return nil, errors.New("audio duration is zero")
	}

	segments := []Segment{}
	current := 0.0
	chunkIndex := 0
	baseDir := filepath.Dir(audioPath)

	for current < duration-0.01 {
		remaining := duration - current
		segmentDuration := float64(chunkSeconds)
		if remaining < segmentDuration {
			segmentDuration = remaining
		}
		chunkPath := filepath.Join(baseDir, fmt.Sprintf("chunk_%04d.wav", chunkIndex))
		logf("Transcribing chunk %d at %.1fs...", chunkIndex+1, current)
		if err := extractAudioSegment(audioPath, chunkPath, current, segmentDuration, accurate); err != nil {
			return nil, err
		}
		chunkSegments, err := transcribeWithRetry(ctx, client, chunkPath, model, language, logf)
		if err != nil {
			return nil, err
		}
		for _, seg := range chunkSegments {
			seg.Start += current
			seg.End += current
			segments = append(segments, seg)
		}
		current += segmentDuration
		chunkIndex++
	}
	return segments, nil
}

func translateSegments(
	ctx context.Context,
	client *openAIClient,
	segments []Segment,
	sourceLang, targetLang, model string,
	workers int,
	minTranslateChars int,
	logf func(string, ...any),
) ([]Segment, error) {
	if workers <= 0 {
		workers = 1
	}
	translated := make([]Segment, len(segments))
	copy(translated, segments)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	workerFn := func() {
		defer wg.Done()
		for idx := range jobs {
			if ctx.Err() != nil {
				return
			}
			text := strings.TrimSpace(translated[idx].Text)
			if text == "" {
				continue
			}
			if isLowInfoText(text, minTranslateChars) {
				continue
			}
			var output string
			err := retry(
				ctx,
				maxRetries,
				baseRetryDelay,
				maxRetryDelay,
				isRetryable,
				func(attempt int, delay time.Duration, err error) {
					logf("Translation failed; retrying in %.1fs (attempt %d). %s", delay.Seconds(), attempt, describeError(err))
				},
				func() error {
					var err error
					output, err = client.Translate(ctx, model, sourceLang, targetLang, text)
					return err
				},
			)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				cancel()
				return
			}
			translated[idx].Text = output
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go workerFn()
	}

sendLoop:
	for i := range segments {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()

	select {
	case err := <-errCh:
		return nil, err
	default:
	}
	return translated, nil
}

func copyFile(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	output, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Sync()
}

func run() int {
	quiet := flag.Bool("quiet", false, "Suppress progress output")
	output := flag.String("output", "", "Output SRT path (defaults to input path with .srt)")
	shortOutput := flag.String("o", "", "Output SRT path (shorthand)")
	whisperModel := flag.String("whisper-model", defaultWhisperModel, "Whisper model")
	sourceLang := flag.String("source-lang", defaultSourceLang, "Source language")
	targetLang := flag.String("target-lang", defaultTargetLang, "Target language")
	translateModel := flag.String("translate-model", defaultTranslateModel, "Translation model")
	noTranslate := flag.Bool("no-translate", false, "Skip translation and output original transcript")
	chunkSeconds := flag.Int("chunk-seconds", 0, "Split audio into chunks of N seconds before transcription")
	maxAudioMB := flag.Int("max-audio-mb", defaultMaxAudioMB, "Auto-chunk when extracted audio exceeds this size (MB)")
	keepAudio := flag.Bool("keep-audio", false, "Keep the extracted audio file")
	translateWorkers := flag.Int("translate-workers", defaultTranslateWorkers, "Number of concurrent translation workers")
	minTranslateChars := flag.Int("min-translate-chars", 4, "Skip translation for segments with fewer than N letters/numbers (0 to disable)")
	timeoutSeconds := flag.Int("timeout-seconds", defaultTimeoutSeconds, "HTTP timeout for OpenAI requests (seconds)")
	highAccuracy := flag.Bool("high-accuracy", false, "Use higher-accuracy transcription settings (slower)")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Input file is required.")
		flag.Usage()
		return 1
	}

	inputPath := flag.Arg(0)
	info, err := os.Stat(inputPath)
	if err != nil || info.IsDir() {
		fmt.Fprintf(os.Stderr, "Input file not found: %s\n", inputPath)
		return 1
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Fprintln(os.Stderr, "ffmpeg is required on PATH.")
		return 1
	}

	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY is not set")
		return 1
	}

	outputPath := *output
	if outputPath == "" {
		outputPath = *shortOutput
	}
	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		outputPath = strings.TrimSuffix(inputPath, ext) + ".srt"
	}

	client := newOpenAIClient(apiKey, time.Duration(*timeoutSeconds)*time.Second)
	ctx := context.Background()

	logf := func(format string, args ...any) {
		if *quiet {
			return
		}
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}

	tmpDir, err := os.MkdirTemp("", "video-subtitle-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmpDir)

	audioPath := filepath.Join(tmpDir, "audio.wav")
	logf("Extracting audio...")
	if err := extractAudio(inputPath, audioPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	audioSizeBytes, err := audioSize(audioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read extracted audio: %v\n", err)
		return 1
	}
	if audioSizeBytes < 1024 {
		fmt.Fprintln(os.Stderr, "Extracted audio is empty or too small.")
		return 1
	}

	if *highAccuracy {
		*minTranslateChars = 0
	}

	maxAudioBytes := int64(*maxAudioMB) * 1024 * 1024
	useChunking := *chunkSeconds > 0 || audioSizeBytes > maxAudioBytes
	chunkSecondsValue := *chunkSeconds

	if useChunking {
		if _, err := exec.LookPath("ffprobe"); err != nil {
			fmt.Fprintln(os.Stderr, "ffprobe is required for chunked transcription.")
			return 1
		}
	}
	if *chunkSeconds <= 0 && audioSizeBytes > maxAudioBytes {
		chunkSecondsValue, err = chooseChunkSeconds(audioPath, defaultChunkSeconds, maxAudioBytes)
		if err != nil {
			logf("Failed to calculate chunk size; using default %ds.", defaultChunkSeconds)
			chunkSecondsValue = defaultChunkSeconds
		}
		logf("Audio is large (%.1f MB); auto-chunking with %ds segments.", float64(audioSizeBytes)/(1024*1024), chunkSecondsValue)
	} else if *chunkSeconds > 0 {
		logf("Chunking audio into %ds segments.", chunkSecondsValue)
	}

	logf("Transcribing with Whisper...")
	segments, err := func() ([]Segment, error) {
		if useChunking {
			return transcribeInChunks(ctx, client, audioPath, *whisperModel, *sourceLang, chunkSecondsValue, *highAccuracy, logf)
		}
		return transcribeWithRetry(ctx, client, audioPath, *whisperModel, *sourceLang, logf)
	}()
	if err != nil {
		if !useChunking && shouldFallbackToChunking(err) {
			if _, errProbe := exec.LookPath("ffprobe"); errProbe != nil {
				fmt.Fprintln(os.Stderr, "ffprobe is required for chunked transcription.")
				return 1
			}
			logf("Whisper request failed; retrying in chunks. Chunk size: %ds.", defaultChunkSeconds)
			segments, err = transcribeInChunks(ctx, client, audioPath, *whisperModel, *sourceLang, defaultChunkSeconds, *highAccuracy, logf)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Transcription failed: %v\n", err)
		return 1
	}

	if !*noTranslate && *sourceLang != *targetLang {
		workers := *translateWorkers
		if workers <= 0 {
			workers = runtime.NumCPU()
		}
		translatable := countTranslatableSegments(segments, *minTranslateChars)
		if translatable == 0 {
			logf("Skipping translation: segments are low-info.")
		} else {
			logf("Translating segments (%d of %d segments, %d workers)...", translatable, len(segments), workers)
			translated, err := translateSegments(ctx, client, segments, *sourceLang, *targetLang, *translateModel, workers, *minTranslateChars, logf)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Translation failed: %v\n", err)
				return 1
			}
			segments = translated
		}
	}

	logf("Writing SRT...")
	if err := writeSRT(segments, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write SRT: %v\n", err)
		return 1
	}

	if *keepAudio {
		kept := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".wav"
		if err := copyFile(audioPath, kept); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to keep audio: %v\n", err)
			return 1
		}
		logf("Kept audio at %s", kept)
	}

	logf("Wrote %s", outputPath)
	return 0
}

func main() {
	os.Exit(run())
}
