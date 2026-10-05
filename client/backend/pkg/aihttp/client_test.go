package aihttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChatCompletionsURL(t *testing.T) {
	got, err := ChatCompletionsURL(" https://api.example.com/v1/ ")
	if err != nil {
		t.Fatalf("ChatCompletionsURL returned error: %v", err)
	}
	if got != "https://api.example.com/v1/chat/completions" {
		t.Fatalf("unexpected URL: %q", got)
	}
	if _, err := ChatCompletionsURL("ftp://api.example.com/v1"); err == nil {
		t.Fatal("expected unsupported scheme to fail")
	}
}

func TestChatCompletionsURLDoesNotDuplicateEndpoint(t *testing.T) {
	for _, testCase := range []struct {
		name string
		base string
		want string
	}{
		{name: "version base", base: "https://api.example.com/v1", want: "https://api.example.com/v1/chat/completions"},
		{name: "custom base", base: "https://api.example.com/openai/v1/", want: "https://api.example.com/openai/v1/chat/completions"},
		{name: "complete endpoint", base: "https://api.example.com/custom/chat/completions", want: "https://api.example.com/custom/chat/completions"},
		{name: "complete endpoint with query", base: "https://api.example.com/custom/chat/completions?api-version=2026-01-01", want: "https://api.example.com/custom/chat/completions?api-version=2026-01-01"},
		{name: "base with query", base: "https://api.example.com/custom?api-version=2026-01-01", want: "https://api.example.com/custom/chat/completions?api-version=2026-01-01"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := ChatCompletionsURL(testCase.base)
			if err != nil {
				t.Fatal(err)
			}
			if got != testCase.want {
				t.Fatalf("ChatCompletionsURL(%q) = %q, want %q", testCase.base, got, testCase.want)
			}
		})
	}
}

func TestModelsUsesOpenAICompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing bearer token")
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client := New(DiagnosticPolicy())
	result, err := client.Models(context.Background(), server.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("Models returned error: %v", err)
	}
	if result.StatusCode != http.StatusOK || result.Attempts != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestModelsURLUsesSiblingOfCompleteChatEndpoint(t *testing.T) {
	got, err := ModelsURL("https://api.example.com/custom/chat/completions?api-version=2026-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.example.com/custom/models?api-version=2026-01-01" {
		t.Fatalf("ModelsURL returned %q", got)
	}
}

func TestStandardPoliciesKeepUtilityAndAgentBudgetsSeparate(t *testing.T) {
	diagnostic := DiagnosticPolicy()
	utility := UtilityPolicy()
	agent := AgentPolicy()
	if diagnostic.MaxRetries != 0 || diagnostic.TotalTimeout != 15*time.Second {
		t.Fatalf("unexpected diagnostic policy: %+v", diagnostic)
	}
	if utility.MaxRetries != 1 || utility.ResponseHeaderTimeout != 60*time.Second || utility.TotalTimeout != 90*time.Second {
		t.Fatalf("unexpected utility policy: %+v", utility)
	}
	if agent.MaxRetries != 1 || agent.ResponseHeaderTimeout != 90*time.Second || agent.TotalTimeout != 180*time.Second {
		t.Fatalf("unexpected agent policy: %+v", agent)
	}
}

func TestClientsWithSamePolicyShareTransport(t *testing.T) {
	first := New(UtilityPolicy())
	second := New(UtilityPolicy())
	if first.httpClient.Transport != second.httpClient.Transport {
		t.Fatal("clients with the same response-header policy should share a connection pool")
	}
}

func TestClientCapturesTraceAndRequestMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected authorization header")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["model"] != "test-model" {
			t.Fatalf("unexpected model: %#v", payload["model"])
		}
		w.Header().Set("x-siliconcloud-trace-id", "trace-123")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client := New(Policy{TotalTimeout: time.Second, ResponseHeaderTimeout: time.Second})
	result, err := client.ChatCompletions(context.Background(), server.URL+"/v1", "test-key", map[string]any{"model": "test-model"})
	if err != nil {
		t.Fatalf("ChatCompletions returned error: %v", err)
	}
	if result.TraceID != "trace-123" || result.Attempts != 1 || result.Duration <= 0 {
		t.Fatalf("unexpected metadata: %+v", result)
	}
}

func TestClientRetriesTransientStatusOnce(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"busy"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client := New(Policy{
		TotalTimeout:          time.Second,
		ResponseHeaderTimeout: time.Second,
		MaxRetries:            1,
	})
	result, err := client.ChatCompletions(context.Background(), server.URL, "test-key", map[string]any{"model": "test-model"})
	if err != nil {
		t.Fatalf("ChatCompletions returned error: %v", err)
	}
	if result.Attempts != 2 || attempts != 2 {
		t.Fatalf("expected two attempts, result=%+v attempts=%d", result, attempts)
	}
}

func TestClientDoesNotRetryAuthenticationFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	defer server.Close()

	client := New(Policy{
		TotalTimeout:          time.Second,
		ResponseHeaderTimeout: time.Second,
		MaxRetries:            1,
	})
	_, err := client.ChatCompletions(context.Background(), server.URL, "bad-key", map[string]any{"model": "test-model"})
	var providerErr *Error
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider Error, got %T: %v", err, err)
	}
	if providerErr.Kind != ErrorAuthentication || providerErr.Attempts != 1 || attempts != 1 {
		t.Fatalf("unexpected error metadata: %+v attempts=%d", providerErr, attempts)
	}
}

func TestClientClassifiesResponseHeaderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	client := New(Policy{
		TotalTimeout:          500 * time.Millisecond,
		ResponseHeaderTimeout: 20 * time.Millisecond,
	})
	_, err := client.ChatCompletions(context.Background(), server.URL, "test-key", map[string]any{"model": "test-model"})
	var providerErr *Error
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider Error, got %T: %v", err, err)
	}
	if providerErr.Kind != ErrorTimeout {
		t.Fatalf("expected timeout, got %+v", providerErr)
	}
}

func TestOpenChatCompletionsKeepsStreamReadableAndCapturesTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("x-request-id", "stream-trace")
		_, _ = w.Write([]byte("data: {\"choices\":[]}\n\n"))
	}))
	defer server.Close()

	client := New(Policy{TotalTimeout: time.Second, ResponseHeaderTimeout: time.Second})
	result, err := client.OpenChatCompletions(context.Background(), server.URL, "test-key", map[string]any{
		"model":  "test-model",
		"stream": true,
	})
	if err != nil {
		t.Fatalf("OpenChatCompletions returned error: %v", err)
	}
	defer result.Close()
	body, err := io.ReadAll(result.Response.Body)
	if err != nil {
		t.Fatalf("stream became unreadable after OpenChatCompletions returned: %v", err)
	}
	if string(body) != "data: {\"choices\":[]}\n\n" || result.TraceID != "stream-trace" {
		t.Fatalf("unexpected stream result: body=%q result=%+v", body, result)
	}
}

func TestOpenChatCompletionsRetriesBeforeReturningStream(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := New(Policy{
		TotalTimeout:          time.Second,
		ResponseHeaderTimeout: time.Second,
		MaxRetries:            1,
	})
	result, err := client.OpenChatCompletions(context.Background(), server.URL, "test-key", map[string]any{"stream": true})
	if err != nil {
		t.Fatalf("OpenChatCompletions returned error: %v", err)
	}
	defer result.Close()
	if result.Attempts != 2 || attempts != 2 {
		t.Fatalf("expected one retry, result=%+v attempts=%d", result, attempts)
	}
}
