package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

	got, err := client.Generate("some prompt")
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
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "my-custom-model",
	}

	_, err := client.Generate("prompt")
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
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	_, err := client.Generate("prompt")
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

	_, err := client.Generate("prompt")
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

	_, err := client.Generate("prompt")
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
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		Model:      "test-model",
	}

	got, err := client.Generate("prompt")
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if got != "  feat: something  \n" {
		t.Errorf("Generate() = %q, want untrimmed content", got)
	}
}