package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	defaultURL   = "http://localhost:1234/api/generate"
	defaultModel = "qwen/qwen3.6-27b"
)

// request is the payload sent to the LM Studio /api/generate endpoint.
type request struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// response is the payload returned by the LM Studio /api/generate endpoint.
type response struct {
	Response string `json:"response"`
}

// Generate sends a prompt to the local LM Studio API and returns the generated text.
func Generate(prompt string) (string, error) {
	body, err := json.Marshal(request{
		Model:  defaultModel,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := http.Post(defaultURL, "application/json", bytes.NewReader(body))
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

	return result.Response, nil
}