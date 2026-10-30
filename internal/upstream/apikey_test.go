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

func channelCredential(t *testing.T, platform, protocol, endpoint string) accounts.Credential {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"api_key": "synthetic-api-key", "base_url": endpoint, "platform": platform, "protocol": protocol})
	credential, err := accounts.ParseFor("openai", raw)
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func TestAPIChannelNativeProtocols(t *testing.T) {
	for _, protocol := range []string{"anthropic", "gemini", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				var payload map[string]any
				if json.NewDecoder(r.Body).Decode(&payload) != nil {
					t.Fatal("invalid upstream JSON")
				}
				if r.Header.Get("Cookie") != "" {
					t.Error("client cookie leaked")
				}
				body, contentType := "", "application/json"
				switch protocol {
				case "anthropic":
					if r.URL.Path != "/edge/v1/messages" || r.Header.Get("X-Api-Key") != "synthetic-api-key" || r.Header.Get("Authorization") != "" || r.Header.Get("Anthropic-Version") != "2023-06-01" {
						t.Errorf("incorrect Anthropic request: %s %#v", r.URL.Path, r.Header)
					}
					body = `{"id":"msg_synthetic","type":"message","role":"assistant","model":"synthetic-model","content":[{"type":"text","text":"Synthetic output"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`
				case "gemini":
					if r.URL.Path != "/edge/v1beta/models/synthetic-model:generateContent" || r.Header.Get("X-Goog-Api-Key") != "synthetic-api-key" || r.Header.Get("Authorization") != "" {
						t.Errorf("incorrect Gemini request: %s %#v", r.URL.Path, r.Header)
					}
					body = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Synthetic output"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2}}`
				case "responses":
					if r.URL.Path != "/edge/v1/responses" || r.Header.Get("Authorization") != "Bearer synthetic-api-key" {
						t.Errorf("incorrect Responses request: %s", r.URL.Path)
					}
					body, contentType = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"synthetic-model\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Synthetic output\"}]}],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n", "text/event-stream"
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			defer client.Close()
			endpoint := "https://channel.example.test/edge/v1"
			if protocol == "gemini" {
				endpoint = "https://channel.example.test/edge/v1beta"
			}
			credential := channelCredential(t, "custom", protocol, endpoint)
			headers := http.Header{"Cookie": {"synthetic-cookie"}}
			var stream *Stream
			var err error
			switch protocol {
			case "anthropic":
				stream, err = client.Messages(t.Context(), credential, []byte(`{"model":"synthetic-model","max_tokens":16,"messages":[{"role":"user","content":"Synthetic input"}]}`), headers)
			case "gemini":
				stream, err = client.Gemini(t.Context(), credential, []byte(`{"model":"synthetic-model","contents":[{"role":"user","parts":[{"text":"Synthetic input"}]}]}`), headers, false)
			case "responses":
				stream, err = client.Responses(t.Context(), credential, []byte(`{"model":"synthetic-model","input":"Synthetic input"}`), headers, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Body.Close()
			var output strings.Builder
			if protocol == "responses" {
				err = stream.Events(func(raw []byte) error { output.Write(raw); return nil })
			} else {
				_, err = io.Copy(&output, stream.Body)
			}
			if err != nil || !strings.Contains(output.String(), "Synthetic output") {
				t.Fatal("native protocol response", output.String(), err)
			}
			if len(client.runtime.manager.List()) != 0 {
				t.Fatal("API credentials entered SDK store")
			}
		})
	}
}

func TestOpenCodeChannelSessionHeaders(t *testing.T) {
	var sessions []string
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		sessions = append(sessions, r.Header.Get("X-Opencode-Session"))
		if r.Header.Get("User-Agent") != "SubLane" {
			t.Error("missing gateway client identity")
		}
		if r.URL.Path == "/inference/go/v1/models" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"synthetic-model"}]}`))}, nil
		}
		if r.URL.Path != "/inference/go/openai/v1/chat/completions" {
			t.Error("incorrect OpenCode endpoint", r.URL.Path)
		}
		body := "data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"model\":\"synthetic-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Synthetic output\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	defer client.Close()
	credential := channelCredential(t, "opencode-go", "openai", "https://channel.example.test/inference/go/openai/v1")
	if _, err := client.Discover(t.Context(), credential); err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{"synthetic-conversation-a", "synthetic-conversation-a", "synthetic-conversation-b", ""} {
		stream, err := client.Responses(t.Context(), credential, []byte(`{"model":"synthetic-model","input":"Synthetic input"}`), http.Header{"Session_id": {session}}, false)
		if err != nil {
			t.Fatal(err)
		}
		err = stream.Events(func([]byte) error { return nil })
		stream.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(sessions) != 5 || sessions[0] == "" || sessions[1] == "" || sessions[1] != sessions[2] || sessions[1] == sessions[3] || sessions[4] == "" || sessions[4] == sessions[1] {
		t.Fatal("invalid OpenCode session isolation", sessions)
	}
}

func TestAPIChannelNativeModelDiscovery(t *testing.T) {
	for _, protocol := range []string{"anthropic", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			calls := 0
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body := `{"data":[{"id":"synthetic-b"}],"has_more":true,"last_id":"synthetic-b"}`
				if protocol == "anthropic" {
					if r.URL.Path != "/edge/v1/models" || r.Header.Get("X-Api-Key") != "synthetic-api-key" || r.URL.Query().Get("limit") != "100" {
						t.Error("incorrect Anthropic catalog request")
					}
					if calls == 2 {
						if r.URL.Query().Get("after_id") != "synthetic-b" {
							t.Error("missing Anthropic pagination")
						}
						body = `{"data":[{"id":"synthetic-a"}],"has_more":false}`
					}
				} else {
					if r.URL.Path != "/edge/v1beta/models" || r.Header.Get("X-Goog-Api-Key") != "synthetic-api-key" {
						t.Error("incorrect Gemini catalog request")
					}
					body = `{"models":[{"name":"models/synthetic-b","supportedGenerationMethods":["generateContent"]},{"name":"models/synthetic-embedding","supportedGenerationMethods":["embedContent"]}],"nextPageToken":"synthetic-next"}`
					if calls == 2 {
						if r.URL.Query().Get("pageToken") != "synthetic-next" {
							t.Error("missing Gemini pagination")
						}
						body = `{"models":[{"name":"models/synthetic-a","supportedGenerationMethods":["generateContent"]}]}`
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			defer client.Close()
			base := "https://channel.example.test/edge/v1"
			if protocol == "gemini" {
				base = "https://channel.example.test/edge/v1beta"
			}
			catalog, err := client.Discover(t.Context(), channelCredential(t, "custom", protocol, base))
			if err != nil || calls != 2 || len(catalog.Models) != 2 || catalog.Models[0].ID != "synthetic-a" || catalog.Models[1].ID != "synthetic-b" {
				t.Fatal("native catalog", catalog, err, calls)
			}
		})
	}
}

func TestAPIChannelResponsesTranslatesNativeClients(t *testing.T) {
	for _, protocol := range []string{"messages", "messages-stream", "gemini", "gemini-stream"} {
		t.Run(protocol, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
				var request struct {
					Stream    bool            `json:"stream"`
					Input     json.RawMessage `json:"input"`
					MaxOutput int64           `json:"max_output_tokens"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request")
				}
				if len(request.Input) == 0 {
					t.Error("native client was not translated to Responses input")
				}
				if request.MaxOutput != 16 {
					t.Error("native client output limit was not preserved", request.MaxOutput)
				}
				message := map[string]any{"type": "message", "id": "msg_synthetic", "role": "assistant", "status": "completed", "content": []map[string]any{{"type": "output_text", "text": "Synthetic output", "annotations": []any{}}}}
				response := map[string]any{"id": "resp_synthetic", "object": "response", "model": "synthetic-model", "status": "completed", "output": []any{message}, "usage": map[string]int{"input_tokens": 3, "output_tokens": 2}}
				contentType := "application/json"
				var body strings.Builder
				if request.Stream {
					contentType = "text/event-stream"
					for _, event := range []map[string]any{
						{"type": "response.created", "response": map[string]any{"id": "resp_synthetic", "model": "synthetic-model", "status": "in_progress", "output": []any{}}},
						{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "message", "id": "msg_synthetic", "role": "assistant", "content": []any{}}},
						{"type": "response.content_part.added", "output_index": 0, "content_index": 0, "part": map[string]string{"type": "output_text", "text": ""}},
						{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "Synthetic output"},
						{"type": "response.output_item.done", "output_index": 0, "item": message},
						{"type": "response.completed", "response": response},
					} {
						raw, _ := json.Marshal(event)
						body.WriteString("data: " + string(raw) + "\n\n")
					}
				} else {
					raw, _ := json.Marshal(response)
					body.Write(raw)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body.String()))}, nil
			}))
			defer client.Close()
			credential := channelCredential(t, "custom", "responses", "https://channel.example.test/v1")
			streaming := strings.HasSuffix(protocol, "-stream")
			var stream *Stream
			var err error
			if strings.HasPrefix(protocol, "messages") {
				raw, _ := json.Marshal(map[string]any{"model": "synthetic-model", "max_tokens": 16, "stream": streaming, "messages": []map[string]string{{"role": "user", "content": "Synthetic input"}}})
				stream, err = client.Messages(t.Context(), credential, raw, nil)
			} else {
				stream, err = client.Gemini(t.Context(), credential, []byte(`{"model":"synthetic-model","generationConfig":{"maxOutputTokens":16},"contents":[{"role":"user","parts":[{"text":"Synthetic input"}]}]}`), nil, streaming)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Body.Close()
			var output strings.Builder
			if streaming {
				err = stream.Events(func(raw []byte) error { output.Write(raw); return nil })
			} else {
				_, err = io.Copy(&output, stream.Body)
			}
			if err != nil || !strings.Contains(output.String(), "Synthetic output") {
				t.Fatal("Responses client translation", output.String(), err)
			}
			if !streaming {
				if protocol == "messages" {
					err = ValidateMessageResponse([]byte(output.String()))
				} else {
					err = ValidateGeminiResponse([]byte(output.String()))
				}
				if err != nil {
					t.Fatal("invalid native response", output.String(), err)
				}
			}
		})
	}
}

func TestAzureChannelDiscoveryUsesAPIKey(t *testing.T) {
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/openai/v1/models" || r.Header.Get("Api-Key") != "synthetic-api-key" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect Azure API-key authentication")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"synthetic-deployment"}]}`))}, nil
	}))
	defer client.Close()
	if _, err := client.Discover(t.Context(), channelCredential(t, "azure", "responses", "https://resource.example.test/openai/v1")); err != nil {
		t.Fatal(err)
	}
}

func TestAPIChannelResponsesPreservesIncompleteStatus(t *testing.T) {
	client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"resp_synthetic","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Synthetic partial"}]}]}`))}, nil
	}))
	defer client.Close()
	stream, err := client.Messages(t.Context(), channelCredential(t, "custom", "responses", "https://channel.example.test/v1"), []byte(`{"model":"synthetic-model","max_tokens":16,"messages":[{"role":"user","content":"Synthetic input"}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	raw, err := io.ReadAll(stream.Body)
	var result struct {
		StopReason string `json:"stop_reason"`
	}
	if err != nil || json.Unmarshal(raw, &result) != nil || result.StopReason != "max_tokens" {
		t.Fatal("incomplete response reported as completed", string(raw), err)
	}
	gemini, err := client.Gemini(t.Context(), channelCredential(t, "custom", "responses", "https://channel.example.test/v1"), []byte(`{"model":"synthetic-model","contents":[{"role":"user","parts":[{"text":"Synthetic input"}]}]}`), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer gemini.Body.Close()
	raw, err = io.ReadAll(gemini.Body)
	var native struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}
	if err != nil || json.Unmarshal(raw, &native) != nil || len(native.Candidates) != 1 || native.Candidates[0].FinishReason != "MAX_TOKENS" {
		t.Fatal("Gemini incomplete response reported as completed", string(raw), err)
	}
}

func TestQwenChannelModelDiscovery(t *testing.T) {
	calls := 0
	client := NewWithTransport(usageTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/v1/models" || r.URL.Query().Get("capabilities") != "TG" || r.URL.Query().Get("page_size") != "100" {
			t.Error("incorrect Qwen model discovery", r.URL.String())
		}
		id := "synthetic-b"
		if calls == 2 {
			id = "synthetic-a"
			if r.URL.Query().Get("page_no") != "2" {
				t.Error("Qwen models were not paged")
			}
		}
		body := `{"success":true,"output":{"total":2,"models":[{"model":"` + id + `"}]}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	defer client.Close()
	catalog, err := client.Discover(t.Context(), channelCredential(t, "qwen", "openai", "https://channel.example.test/compatible-mode/v1"))
	if err != nil || calls != 2 || len(catalog.Models) != 2 || catalog.Models[0].ID != "synthetic-a" {
		t.Fatal("Qwen model discovery", catalog, err, calls)
	}
}

func TestAPIChannelTransportRejectsCredentialEscape(t *testing.T) {
	for _, target := range []string{"https://other.example.test/v1/models", "https://channel.example.test/admin", "https://channel.example.test/v1/../admin"} {
		t.Run(target, func(t *testing.T) {
			called := false
			transport := channelTransport{credential: apiCredential(t, "https://channel.example.test/v1"), base: usageTransport(func(*http.Request) (*http.Response, error) {
				called = true
				return &http.Response{StatusCode: 200}, nil
			})}
			req, _ := http.NewRequest(http.MethodGet, target, nil)
			if _, err := transport.RoundTrip(req); !errors.Is(err, ErrUpstream) || called {
				t.Fatal("channel credential left its configured boundary", err, called)
			}
		})
	}
}

func TestAPIChannelResponsesRejectsCompaction(t *testing.T) {
	calls := 0
	client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"resp_synthetic","status":"completed","output":[]}`))}, nil
	}))
	defer client.Close()
	stream, err := client.Responses(t.Context(), channelCredential(t, "custom", "responses", "https://channel.example.test/v1"), []byte(`{"model":"synthetic-model","input":"Synthetic input"}`), nil, true)
	if stream != nil {
		stream.Body.Close()
	}
	if !errors.Is(err, ErrInput) || calls != 0 {
		t.Fatal("API compaction reached upstream", calls, err)
	}
}
