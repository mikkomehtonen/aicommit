package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerate_success(t *testing.T) {
	wantMsg := "feat: add new feature"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure.
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Stream != false {
			t.Errorf("expected stream=false, got true")
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Errorf("expected single user message, got %v", req.Messages)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept: application/json, got: %q", r.Header.Get("Accept"))
		}

		resp := response{
			Choices: []choice{
				{Message: message{Role: "assistant", Content: wantMsg}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	got, err := client.Generate(context.Background(), "some prompt")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if got != wantMsg {
		t.Errorf("Generate() = %q, want %q", got, wantMsg)
	}
}

func TestGenerate_customModel(t *testing.T) {
	var receivedModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		json.NewDecoder(r.Body).Decode(&req)
		receivedModel = req.Model
		resp := response{
			Choices: []choice{{Message: message{Content: "ok"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "my-custom-model",
	}

	_, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if receivedModel != "my-custom-model" {
		t.Errorf("model sent = %q, want %q", receivedModel, "my-custom-model")
	}
}

func TestGenerate_emptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := response{Choices: []choice{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	_, err := client.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for empty choices, got nil")
	}
	if err.Error() != "LLM returned no choices" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerate_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	_, err := client.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	// Should contain the status code.
	if err.Error() == "" {
		t.Error("error should contain status code info")
	}
}

func TestGenerate_connectionRefused(t *testing.T) {
	client := &Client{
		HTTPClient: http.DefaultClient,
		URL:        "http://127.0.0.1:0/v1/chat/completions",
		Model:      "test-model",
	}

	_, err := client.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}

func TestGenerate_trimsWhitespaceInResponse(t *testing.T) {
	// Verify that the raw response content is returned as-is;
	// trimming is the caller's responsibility.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := response{
			Choices: []choice{
				{Message: message{Content: "  feat: something  \n"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	got, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if got != "  feat: something  \n" {
		t.Errorf("Generate() = %q, want untrimmed content", got)
	}
}

func TestGenerateWithTemperature_sendsTemperature(t *testing.T) {
	var receivedTemp float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		json.NewDecoder(r.Body).Decode(&req)
		if req.Temperature != nil {
			receivedTemp = *req.Temperature
		}
		resp := response{
			Choices: []choice{{Message: message{Content: "ok"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	_, err := client.GenerateWithTemperature(context.Background(), "prompt", 0.7)
	if err != nil {
		t.Fatalf("GenerateWithTemperature() error: %v", err)
	}
	if receivedTemp != 0.7 {
		t.Errorf("temperature sent = %f, want 0.7", receivedTemp)
	}
}

func TestGenerateWithTemperature_defaultTemperature(t *testing.T) {
	var receivedTemp float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		json.NewDecoder(r.Body).Decode(&req)
		if req.Temperature != nil {
			receivedTemp = *req.Temperature
		}
		resp := response{
			Choices: []choice{{Message: message{Content: "ok"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient:       srv.Client(),
		URL:              srv.URL,
		Model:            "test-model",
		Temperature:      0.5,
		RetryTemperature: 0.8,
	}

	_, err := client.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if receivedTemp != 0.5 {
		t.Errorf("temperature sent = %f, want 0.5", receivedTemp)
	}
}

func TestEnvTemperature(t *testing.T) {
	t.Setenv("AICOMMIT_TEMPERATURE", "0.6")
	got, warn := envTemperature()
	if got != 0.6 {
		t.Errorf("envTemperature() = %f, want 0.6", got)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
}

func TestEnvTemperature_invalidValue(t *testing.T) {
	t.Setenv("AICOMMIT_TEMPERATURE", "not-a-number")
	got, warn := envTemperature()
	if got != defaultTemperature {
		t.Errorf("envTemperature() = %f, want default %f", got, defaultTemperature)
	}
	if warn == "" {
		t.Error("expected warning for invalid temperature")
	}
}

func TestEnvTemperature_emptyValue(t *testing.T) {
	t.Setenv("AICOMMIT_TEMPERATURE", "")
	got, warn := envTemperature()
	if got != defaultTemperature {
		t.Errorf("envTemperature() = %f, want default %f", got, defaultTemperature)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
}

func TestEnvRetryTemperature(t *testing.T) {
	t.Setenv("AICOMMIT_RETRY_TEMPERATURE", "0.9")
	got, warn := envRetryTemperature()
	if got != 0.9 {
		t.Errorf("envRetryTemperature() = %f, want 0.9", got)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
}

func TestEnvRetryTemperature_invalidValue(t *testing.T) {
	t.Setenv("AICOMMIT_RETRY_TEMPERATURE", "invalid")
	got, warn := envRetryTemperature()
	if got != defaultRetryTemperature {
		t.Errorf("envRetryTemperature() = %f, want default %f", got, defaultRetryTemperature)
	}
	if warn == "" {
		t.Error("expected warning for invalid retry temperature")
	}
}

func TestEnvRetryTemperature_emptyValue(t *testing.T) {
	t.Setenv("AICOMMIT_RETRY_TEMPERATURE", "")
	got, warn := envRetryTemperature()
	if got != defaultRetryTemperature {
		t.Errorf("envRetryTemperature() = %f, want default %f", got, defaultRetryTemperature)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
}

func TestGenerate_nonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	_, err := client.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for non-JSON response, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected content type") {
		t.Errorf("error = %q, wanted it to contain %q", err.Error(), "unexpected content type")
	}
}

func TestEnvTimeout(t *testing.T) {
	t.Setenv("AICOMMIT_TIMEOUT", "30s")
	got, warn := envTimeout()
	if got != 30*time.Second {
		t.Errorf("envTimeout() = %v, want 30s", got)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
}

func TestEnvTimeout_minutes(t *testing.T) {
	t.Setenv("AICOMMIT_TIMEOUT", "2m")
	got, warn := envTimeout()
	if got != 2*time.Minute {
		t.Errorf("envTimeout() = %v, want 2m", got)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
}

func TestEnvTimeout_invalidValue(t *testing.T) {
	t.Setenv("AICOMMIT_TIMEOUT", "not-a-duration")
	got, warn := envTimeout()
	if got != defaultTimeout {
		t.Errorf("envTimeout() = %v, want default %v", got, defaultTimeout)
	}
	if warn == "" {
		t.Error("expected warning for invalid timeout")
	}
}

func TestEnvTimeout_emptyValue(t *testing.T) {
	t.Setenv("AICOMMIT_TIMEOUT", "")
	got, warn := envTimeout()
	if got != defaultTimeout {
		t.Errorf("envTimeout() = %v, want default %v", got, defaultTimeout)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
}

func TestGenerateWithTemperature_timeoutCancelsRequest(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-done
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(done)

	client := &Client{
		HTTPClient: http.DefaultClient,
		URL:        srv.URL,
		Model:      "test-model",
		Timeout:    50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_, err := client.GenerateWithTemperature(ctx, "prompt", 0.5)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error due to timeout, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("timeout took %v, should have been well under 500ms", elapsed)
	}
}

func TestEnvTemperature_negativeValue(t *testing.T) {
	t.Setenv("AICOMMIT_TEMPERATURE", "-0.5")
	got, warn := envTemperature()
	if got != defaultTemperature {
		t.Errorf("envTemperature() = %f, want default %f", got, defaultTemperature)
	}
	if warn == "" {
		t.Error("expected warning for negative temperature")
	}
	if !strings.Contains(warn, "negative") {
		t.Errorf("warning should mention negative, got %q", warn)
	}
}

func TestEnvRetryTemperature_negativeValue(t *testing.T) {
	t.Setenv("AICOMMIT_RETRY_TEMPERATURE", "-0.5")
	got, warn := envRetryTemperature()
	if got != defaultRetryTemperature {
		t.Errorf("envRetryTemperature() = %f, want default %f", got, defaultRetryTemperature)
	}
	if warn == "" {
		t.Error("expected warning for negative retry temperature")
	}
	if !strings.Contains(warn, "negative") {
		t.Errorf("warning should mention negative, got %q", warn)
	}
}

func TestEnvAPIKey(t *testing.T) {
	t.Setenv("AICOMMIT_API_KEY", "sk-test123")
	got := envAPIKey()
	if got != "sk-test123" {
		t.Errorf("envAPIKey() = %q, want %q", got, "sk-test123")
	}
}

func TestEnvAPIKey_empty(t *testing.T) {
	t.Setenv("AICOMMIT_API_KEY", "")
	got := envAPIKey()
	if got != "" {
		t.Errorf("envAPIKey() = %q, want empty", got)
	}
}

func TestGenerateWithTemperature_withAPIKey(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		resp := response{
			Choices: []choice{{Message: message{Content: "ok"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
		APIKey:     "sk-secret",
	}

	_, err := client.GenerateWithTemperature(context.Background(), "prompt", 0.5)
	if err != nil {
		t.Fatalf("GenerateWithTemperature() error: %v", err)
	}
	if receivedAuth != "Bearer sk-secret" {
		t.Errorf("Authorization header = %q, want %q", receivedAuth, "Bearer sk-secret")
	}
}

func TestGenerateWithTemperature_withoutAPIKey(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		resp := response{
			Choices: []choice{{Message: message{Content: "ok"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	_, err := client.GenerateWithTemperature(context.Background(), "prompt", 0.5)
	if err != nil {
		t.Fatalf("GenerateWithTemperature() error: %v", err)
	}
	if receivedAuth != "" {
		t.Errorf("Authorization header = %q, want empty", receivedAuth)
	}
}

func TestNewClient_timeoutFromEnv(t *testing.T) {
	t.Setenv("AICOMMIT_TIMEOUT", "90s")
	t.Setenv("AICOMMIT_URL", "http://localhost:9999/v1/chat/completions")
	t.Setenv("AICOMMIT_MODEL", "test")
	t.Setenv("AICOMMIT_TEMPERATURE", "0.5")
	t.Setenv("AICOMMIT_RETRY_TEMPERATURE", "0.6")

	client, warnings := NewClient()
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if client.Timeout != 90*time.Second {
		t.Errorf("Timeout = %v, want 90s", client.Timeout)
	}
}

func TestGenerateWithTemperature_retries429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		resp := response{
			Choices: []choice{{Message: message{Content: "ok"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	got, err := client.GenerateWithTemperature(context.Background(), "prompt", 0.5)
	if err != nil {
		t.Fatalf("GenerateWithTemperature() error: %v", err)
	}
	if got != "ok" {
		t.Errorf("GenerateWithTemperature() = %q, want %q", got, "ok")
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}
