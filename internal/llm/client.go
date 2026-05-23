package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultURL              = "http://localhost:1234/v1/chat/completions"
	defaultModel            = "qwen/qwen3.6-27b"
	defaultTemperature      = 0.1
	defaultRetryTemperature = 0.4
	defaultTimeout          = 60 * time.Second
)

// request is the payload sent to the OpenAI-compatible chat completions endpoint.
type request struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	Stream    bool      `json:"stream"`
	Temperature *float64 `json:"temperature,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// response is the payload returned by the OpenAI-compatible chat completions endpoint.
type response struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message message `json:"message"`
}

// Client sends prompts to an OpenAI-compatible chat completions endpoint.
type Client struct {
	HTTPClient    *http.Client
	URL           string
	Model         string
	Temperature   float64
	RetryTemperature float64
}

// NewClient returns a Client with defaults. The URL is read from the
// AICOMMIT_URL environment variable, falling back to defaultURL. The model
// is read from the AICOMMIT_MODEL environment variable, falling back to defaultModel.
// Temperature is read from AICOMMIT_TEMPERATURE (first request) and
// AICOMMIT_RETRY_TEMPERATURE (retry requests), falling back to their defaults.
func NewClient() *Client {
	return &Client{
		HTTPClient:     &http.Client{Timeout: defaultTimeout},
		URL:            envURL(),
		Model:          envModel(),
		Temperature:    envTemperature(),
		RetryTemperature: envRetryTemperature(),
	}
}

// envURL returns the backend URL from the AICOMMIT_URL environment variable,
// falling back to the default if not set.
func envURL() string {
	if u := os.Getenv("AICOMMIT_URL"); u != "" {
		return u
	}
	return defaultURL
}

// envModel returns the model name from the AICOMMIT_MODEL environment variable,
// falling back to the default if not set.
func envModel() string {
	if m := os.Getenv("AICOMMIT_MODEL"); m != "" {
		return m
	}
	return defaultModel
}

// envTemperature returns the temperature from the AICOMMIT_TEMPERATURE
// environment variable, falling back to the default if not set.
func envTemperature() float64 {
	if t := os.Getenv("AICOMMIT_TEMPERATURE"); t != "" {
		var val float64
		if _, err := fmt.Sscanf(t, "%f", &val); err == nil {
			return val
		}
	}
	return defaultTemperature
}

// envRetryTemperature returns the retry temperature from the
// AICOMMIT_RETRY_TEMPERATURE environment variable, falling back to the
// default if not set.
func envRetryTemperature() float64 {
	if t := os.Getenv("AICOMMIT_RETRY_TEMPERATURE"); t != "" {
		var val float64
		if _, err := fmt.Sscanf(t, "%f", &val); err == nil {
			return val
		}
	}
	return defaultRetryTemperature
}

// Generate sends a prompt to the LLM API and returns the generated text.
func (c *Client) Generate(prompt string) (string, error) {
	return c.GenerateWithTemperature(prompt, c.Temperature)
}

// GenerateWithTemperature sends a prompt to the LLM API with the given
// temperature and returns the generated text.
func (c *Client) GenerateWithTemperature(prompt string, temperature float64) (string, error) {
	body, err := json.Marshal(request{
		Model:     c.Model,
		Messages:  []message{{Role: "user", Content: prompt}},
		Stream:    false,
		Temperature: &temperature,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.HTTPClient.Post(c.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("sending request to LLM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM returned status %d: %s", resp.StatusCode, string(respBody))
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected content type %q: %s", ct, string(respBody))
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}