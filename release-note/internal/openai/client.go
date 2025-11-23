package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"release-note/internal/config"
)

// Client is a minimal HTTP client for OpenAI's chat completions API.
type Client struct {
	apiKey      string
	model       string
	temperature float32
	maxTokens   int
	httpClient  *http.Client
}

func NewClientFromEnv(cfg config.PromptConfig) (Client, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return Client{}, errors.New("OPENAI_API_KEY is not set")
	}

	return Client{
		apiKey:      key,
		model:       cfg.Model,
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Generate sends the system + user prompts to OpenAI and returns the text content.
func (c Client) Generate(systemPrompt, userPrompt string) (string, error) {
	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": c.temperature,
		"max_tokens":  c.maxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal openai payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call openai: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("openai responded with status %s", resp.Status)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return "", errors.New("openai returned no choices")
	}

	return parsed.Choices[0].Message.Content, nil
}
