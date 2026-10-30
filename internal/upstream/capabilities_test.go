package upstream

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/accounts"
)

func TestDiscoveryRetainsOnlyAdvertisedCapabilities(t *testing.T) {
	tests := []struct {
		name, body string
		credential accounts.Credential
		expected   string
	}{
		{"codex", `{"models":[{"slug":"synthetic-capable","context_window":200000,"max_output_tokens":16000,"supported_reasoning_levels":[{"effort":"low","description":"ignored"},{"effort":"high"}],"input_modalities":["text","image"],"supports_tools":false},{"slug":"synthetic-unknown"}]}`, accounts.Credential{AccessToken: "synthetic-access", AccountID: "synthetic-account"}, `{"context_window":200000,"max_output_tokens":16000,"supported_reasoning_levels":["low","high"],"supports_images":true,"supports_tools":false}`},
		{"openai", `{"data":[{"id":"synthetic-capable","context_window":200000,"max_output_tokens":16000,"supported_reasoning_levels":["low","high"],"supports_images":false,"supports_tools":true},{"id":"synthetic-unknown"}]}`, channelCredential(t, "custom", "openai", "https://channel.example.test/v1"), `{"context_window":200000,"max_output_tokens":16000,"supported_reasoning_levels":["low","high"],"supports_images":false,"supports_tools":true}`},
		{"gemini", `{"models":[{"name":"models/synthetic-capable","supportedGenerationMethods":["generateContent"],"inputTokenLimit":200000,"outputTokenLimit":16000},{"name":"models/synthetic-unknown","supportedGenerationMethods":["generateContent"]}]}`, channelCredential(t, "custom", "gemini", "https://channel.example.test/v1beta"), `{"context_window":200000,"max_output_tokens":16000}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}))
			defer client.Close()
			value, err := client.Discover(t.Context(), test.credential)
			if err != nil || len(value.Models) != 2 {
				t.Fatal(value, err)
			}
			var expected map[string]any
			json.Unmarshal([]byte(test.expected), &expected)
			for _, model := range value.Models {
				raw, _ := json.Marshal(model)
				var actual map[string]any
				json.Unmarshal(raw, &actual)
				for _, key := range []string{"context_window", "max_output_tokens", "supported_reasoning_levels", "supports_images", "supports_tools"} {
					want, known := expected[key]
					if model.ID == "synthetic-unknown" {
						known = false
					}
					got, present := actual[key]
					if present != known || known && !jsonEqual(got, want) {
						t.Fatalf("%s %s: got %s, want %s", model.ID, key, raw, test.expected)
					}
				}
			}
		})
	}
}

func TestDiscoveryIgnoresMalformedOptionalCapabilities(t *testing.T) {
	client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":"synthetic-model","context_window":-1,"max_output_tokens":1000000001,"supported_reasoning_levels":[{"effort":"bad\nlevel"}],"input_modalities":"image","supports_tools":"true"}]}`))}, nil
	}))
	defer client.Close()
	value, err := client.Discover(t.Context(), accounts.Credential{AccessToken: "synthetic-access", AccountID: "synthetic-account"})
	if err != nil || len(value.Models) != 1 {
		t.Fatal(value, err)
	}
	raw, _ := json.Marshal(value.Models[0])
	for _, field := range []string{"context_window", "max_output_tokens", "supported_reasoning_levels", "supports_images", "supports_tools"} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("invalid optional metadata retained: %s", raw)
		}
	}
}

func jsonEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func TestDiscoveryReadsProviderCapabilityContracts(t *testing.T) {
	tests := []struct {
		name, body string
		credential accounts.Credential
		expected   string
	}{
		{"codex-tools", `{"models":[{"slug":"synthetic-model","shell_type":"shell_command"}]}`, accounts.Credential{AccessToken: "synthetic-access", AccountID: "synthetic-account"}, `{"supports_tools":true}`},
		{"anthropic", `{"data":[{"id":"synthetic-model","max_input_tokens":200000,"max_tokens":16000,"capabilities":{"image_input":{"supported":false},"effort":{"low":{"supported":true},"high":{"supported":true},"max":{"supported":false}},"server_tools":{"supported":true}}}]}`, channelCredential(t, "custom", "anthropic", "https://channel.example.test/v1"), `{"context_window":200000,"max_output_tokens":16000,"supported_reasoning_levels":["high","low"],"supports_images":false,"supports_tools":true}`},
		{"openrouter", `{"data":[{"id":"synthetic-model","context_length":200000,"top_provider":{"max_completion_tokens":16000},"architecture":{"input_modalities":["text","image"]},"supported_parameters":["temperature","tools"]}]}`, channelCredential(t, "openrouter", "openai", "https://channel.example.test/v1"), `{"context_window":200000,"max_output_tokens":16000,"supports_images":true,"supports_tools":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewWithTransport(usageTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}))
			defer client.Close()
			value, err := client.Discover(t.Context(), test.credential)
			if err != nil || len(value.Models) != 1 {
				t.Fatal(value, err)
			}
			raw, _ := json.Marshal(value.Models[0])
			var got, want map[string]any
			json.Unmarshal(raw, &got)
			json.Unmarshal([]byte(test.expected), &want)
			for key, expected := range want {
				if !jsonEqual(got[key], expected) {
					t.Fatalf("%s: got %s, expected %s", key, raw, test.expected)
				}
			}
		})
	}
}
