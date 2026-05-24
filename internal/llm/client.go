package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
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
	maxResponseBody         = 1 << 20 // 1 MB
	maxRetries              = 3
)

// request is the payload sent to the OpenAI-compatible chat completions endpoint.
type request struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature *float64  `json:"temperature,omitempty"`
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
	HTTPClient       *http.Client
	URL              string
	Model            string
	Temperature      float64
	RetryTemperature float64
	Timeout          time.Duration
}

// NewClient returns a Client with defaults. The URL is read from the
// AICOMMIT_URL environment variable, falling back to defaultURL. The model
// is read from the AICOMMIT_MODEL environment variable, falling back to defaultModel.
// Temperature is read from AICOMMIT_TEMPERATURE (first request) and
// AICOMMIT_RETRY_TEMPERATURE (retry requests), falling back to their defaults.
// Timeout is read from AICOMMIT_TIMEOUT (as a Go duration string like "60s"),
// falling back to defaultTimeout.
// The second return value is a slice of warning messages (empty if all env vars are valid).
func NewClient() (*Client, []string) {
	var warnings []string

	timeout, timeoutWarn := envTimeout()
	if timeoutWarn != "" {
		warnings = append(warnings, timeoutWarn)
	}
	temp, tempWarn := envTemperature()
	if tempWarn != "" {
		warnings = append(warnings, tempWarn)
	}
	retryTemp, retryTempWarn := envRetryTemperature()
	if retryTempWarn != "" {
		warnings = append(warnings, retryTempWarn)
	}

	return &Client{
		HTTPClient:       &http.Client{},
		URL:              envURL(),
		Model:            envModel(),
		Temperature:      temp,
		RetryTemperature: retryTemp,
		Timeout:          timeout,
	}, warnings
}

// envTimeout returns the timeout from the AICOMMIT_TIMEOUT environment variable
// (parsed as a Go duration string, e.g. "60s" or "2m"), falling back to the
// default if not set.
// Returns (value, warning) where warning is non-empty if the env var was invalid.
func envTimeout() (time.Duration, string) {
	if t := os.Getenv("AICOMMIT_TIMEOUT"); t != "" {
		d, err := time.ParseDuration(t)
		if err == nil {
			return d, ""
		}
		return defaultTimeout, fmt.Sprintf("warning: AICOMMIT_TIMEOUT=%q is not a valid duration, using default %s", t, defaultTimeout)
	}
	return defaultTimeout, ""
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
// Returns (value, warning) where warning is non-empty if the env var was invalid.
func envTemperature() (float64, string) {
	if t := os.Getenv("AICOMMIT_TEMPERATURE"); t != "" {
		var val float64
		if _, err := fmt.Sscanf(t, "%f", &val); err == nil {
			if val < 0 {
				return defaultTemperature, fmt.Sprintf("warning: AICOMMIT_TEMPERATURE=%f is negative, using default %f", val, defaultTemperature)
			}
			return val, ""
		}
		return defaultTemperature, fmt.Sprintf("warning: AICOMMIT_TEMPERATURE=%q is not a valid float, using default %f", t, defaultTemperature)
	}
	return defaultTemperature, ""
}

// envRetryTemperature returns the retry temperature from the
// AICOMMIT_RETRY_TEMPERATURE environment variable, falling back to the
// default if not set.
// Returns (value, warning) where warning is non-empty if the env var was invalid.
func envRetryTemperature() (float64, string) {
	if t := os.Getenv("AICOMMIT_RETRY_TEMPERATURE"); t != "" {
		var val float64
		if _, err := fmt.Sscanf(t, "%f", &val); err == nil {
			if val < 0 {
				return defaultRetryTemperature, fmt.Sprintf("warning: AICOMMIT_RETRY_TEMPERATURE=%f is negative, using default %f", val, defaultRetryTemperature)
			}
			return val, ""
		}
		return defaultRetryTemperature, fmt.Sprintf("warning: AICOMMIT_RETRY_TEMPERATURE=%q is not a valid float, using default %f", t, defaultRetryTemperature)
	}
	return defaultRetryTemperature, ""
}

// Generate sends a prompt to the LLM API and returns the generated text.
func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	return c.GenerateWithTemperature(ctx, prompt, c.Temperature)
}

// GenerateWithTemperature sends a prompt to the LLM API with the given
// temperature and returns the generated text. The client's Timeout is applied
// as an overall deadline covering all retry attempts.
func (c *Client) GenerateWithTemperature(ctx context.Context, prompt string, temperature float64) (string, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	body, err := json.Marshal(request{
		Model:       c.Model,
		Messages:    []message{{Role: "user", Content: prompt}},
		Stream:      false,
		Temperature: &temperature,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("generating commit message: %w", err)
		}

		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			if delay > 10*time.Second {
				delay = 10 * time.Second
			}
			jitter := time.Duration(rand.Float64() * float64(delay) * 0.3)
			select {
			case <-time.After(delay + jitter):
			case <-ctx.Done():
				return "", fmt.Errorf("generating commit message: %w", ctx.Err())
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if isRetryableError(err) && attempt < maxRetries-1 {
				lastErr = err
				continue
			}
			return "", fmt.Errorf("sending request to LLM: %w", err)
		}

		if isRetryableStatusCode(resp.StatusCode) && attempt < maxRetries-1 {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("LLM returned status %d", resp.StatusCode)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
			resp.Body.Close()
			return "", fmt.Errorf("LLM returned status %d: %s", resp.StatusCode, string(respBody))
		}

		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/json") {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
			resp.Body.Close()
			return "", fmt.Errorf("unexpected content type %q: %s", ct, string(respBody))
		}

		var result response
		limited := io.LimitReader(resp.Body, maxResponseBody)
		if err := json.NewDecoder(limited).Decode(&result); err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("decoding response: %w", err)
		}

		if len(result.Choices) == 0 {
			resp.Body.Close()
			return "", fmt.Errorf("LLM returned no choices")
		}

		resp.Body.Close()
		return result.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("sending request to LLM after %d attempts: %w", maxRetries, lastErr)
}

func isRetryableError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

func isRetryableStatusCode(code int) bool {
	return code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
}
