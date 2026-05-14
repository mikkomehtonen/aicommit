package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	defaultURL   = "http://localhost:1234/v1/chat/completions"
	defaultModel = "qwen/qwen3.6-27b"
)

// request is the payload sent to the OpenAI-compatible chat completions endpoint.
type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
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
	HTTPClient *http.Client
	URL        string
	Model      string
}

// NewClient returns a Client with defaults. The URL is read from the
// AICOMMIT_URL environment variable, falling back to defaultURL. The model
// is read from the AICOMMIT_MODEL environment variable, falling back to defaultModel.
func NewClient() *Client {
	return &Client{
		HTTPClient: http.DefaultClient,
		URL:        envURL(),
		Model:      envModel(),
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

// Generate sends a prompt to the LLM API and returns the generated text.
func (c *Client) Generate(prompt string) (string, error) {
	body, err := json.Marshal(request{
		Model: c.Model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		Stream: false,
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

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}