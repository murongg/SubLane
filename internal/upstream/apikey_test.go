package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/accounts"
)

func apiCredential(t *testing.T, endpoint string) accounts.Credential {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"api_key": "synthetic-api-key", "base_url": endpoint})
	credential, err := accounts.ParseFor("openai", raw)
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func TestAPIKeyDiscoveryAndProtocolForwarding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-api-key" {
			t.Error("API key authentication missing")
		}
		for _, header := range []string{"Cookie", "Chatgpt-Account-Id", "X-Api-Key", "X-Goog-Api-Key"} {
			if r.Header.Get(header) != "" {
				t.Error("client or subscription credential leaked", header)
			}
		}
		if r.URL.Path == "/relay/v1/models" {
			if r.Method != http.MethodGet || r.ContentLength != 0 {
				t.Error("model discovery must use a bodyless GET")
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"openai/synthetic-model"}]}`)
			return
		}
		if r.URL.Path != "/relay/v1/chat/completions" {
			t.Errorf("wrong API endpoint: %s", r.URL.Path)
		}
		var request struct {
			Model  string
			Stream bool
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != "openai/synthetic-model" {
			t.Error("native model namespace lost")
		}
		if !request.Stream {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"synthetic-chat","object":"chat.completion","model":"openai/synthetic-model","choices":[{"index":0,"message":{"role":"assistant","content":"Synthetic output"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"model\":\"openai/synthetic-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Synthetic output\"},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"model\":\"openai/synthetic-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := New()
	defer client.Close()
	credential := apiCredential(t, server.URL+"/relay/v1")
	discovery, err := client.Discover(context.Background(), credential)
	if err != nil || len(discovery.Models) != 1 || discovery.Models[0].ID != "openai/synthetic-model" {
		t.Fatal("API discovery", discovery, err)
	}
	for _, protocol := range []string{"responses", "chat", "messages", "gemini", "messages-stream", "gemini-stream"} {
		t.Run(protocol, func(t *testing.T) {
			headers := http.Header{"Authorization": {"Bearer synthetic-gateway-key"}, "Cookie": {"synthetic-cookie"}}
			var stream *Stream
			var err error
			switch protocol {
			case "responses":
				stream, err = client.Responses(context.Background(), credential, []byte(`{"model":"openai/synthetic-model","input":"Synthetic input"}`), headers, false)
			case "chat":
				stream, err = client.Chat(context.Background(), credential, []byte(`{"model":"openai/synthetic-model","messages":[{"role":"user","content":"Synthetic input"}]}`), headers)
			case "messages":
				stream, err = client.Messages(context.Background(), credential, []byte(`{"model":"openai/synthetic-model","max_tokens":16,"messages":[{"role":"user","content":"Synthetic input"}]}`), headers)
			case "messages-stream":
				stream, err = client.Messages(context.Background(), credential, []byte(`{"model":"openai/synthetic-model","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"Synthetic input"}]}`), headers)
			case "gemini":
				stream, err = client.Gemini(context.Background(), credential, []byte(`{"model":"openai/synthetic-model","contents":[{"role":"user","parts":[{"text":"Synthetic input"}]}]}`), headers, false)
			case "gemini-stream":
				stream, err = client.Gemini(context.Background(), credential, []byte(`{"model":"openai/synthetic-model","contents":[{"role":"user","parts":[{"text":"Synthetic input"}]}]}`), headers, true)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Body.Close()
			if stream.StatusCode != 200 {
				t.Fatal("API forwarding status", stream.StatusCode)
			}
			var output strings.Builder
			if protocol == "responses" || protocol == "chat" || strings.HasSuffix(protocol, "-stream") {
				err = stream.Events(func(raw []byte) error { output.Write(raw); return nil })
			} else {
				raw, readErr := io.ReadAll(stream.Body)
				err = readErr
				output.Write(raw)
			}
			if err != nil || !strings.Contains(output.String(), "Synthetic output") {
				t.Fatal("API protocol output", output.String(), err)
			}
		})
	}
	if len(client.runtime.manager.List()) != 0 {
		t.Fatal("API credentials mirrored into SDK")
	}
	if _, err := client.Refresh(context.Background(), credential); !errors.Is(err, accounts.ErrReauthorize) {
		t.Fatal("static key entered refresh", err)
	}
	if _, err := client.Usage(context.Background(), credential); !errors.Is(err, ErrUsageUnsupported) {
		t.Fatal("invented API subscription quota", err)
	}
}

func TestAPIKeyDiscoveryRejectsRedirectsAndSanitizesErrors(t *testing.T) {
	for _, status := range []int{302, 401, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://other.example.test/models")
				w.Header().Set("Retry-After", "23")
				w.WriteHeader(status)
				io.WriteString(w, "synthetic-sensitive-error")
			}))
			defer server.Close()
			client := New()
			defer client.Close()
			_, err := client.Discover(context.Background(), apiCredential(t, server.URL+"/v1"))
			if err == nil || strings.Contains(err.Error(), "synthetic-sensitive-error") {
				t.Fatal("unsafe API failure", err)
			}
			if status == 429 {
				var rejected *UpstreamError
				if !errors.As(err, &rejected) || rejected.RetryAfter != "23" {
					t.Fatal("retry deadline lost", err)
				}
			}
		})
	}
}
