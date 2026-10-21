package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/upstream"
)

const checkResponse = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}]}}\n\n"

func TestCheckModelUsesSelectedResourceWithoutAffinity(t *testing.T) {
	var calls atomic.Int32
	s, ids := providerGateway(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Header.Get("Chatgpt-Account-Id") != "synthetic-subject" || r.Header.Get("Authorization") != "Bearer synthetic-access" {
			t.Error("check did not use the selected account")
		}
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 30*time.Second {
			t.Error("unbounded check")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(checkResponse))}, nil
	}))
	result, err := s.CheckModel(t.Context(), ids["codex"], "synthetic-model")
	if err != nil || !result.Available || result.Model != "synthetic-model" || result.DurationMS < 0 || result.ErrorCode != "" || calls.Load() != 1 {
		t.Fatalf("check: %+v %v, calls %d", result, err, calls.Load())
	}
	var affinities, requests int
	if err := s.db.QueryRow("SELECT count(*) FROM account_affinity").Scan(&affinities); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM request_records").Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if affinities != 0 || requests != 0 || s.health[ids["codex"]].InFlight != 0 || len(s.slots) != 0 || len(s.modelHealth) != 0 {
		t.Fatal("management check retained leases or changed conversation/history state")
	}
}

func TestCheckModelRejectsFailuresAndEmptySuccess(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		status           int
	}{
		{"denied", `{"error":{"message":"synthetic-secret"}}`, "upstream_forbidden", 403},
		{"limited", `{"error":{"message":"synthetic-secret"}}`, "rate_limited", 429},
		{"empty", "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n", "stream_interrupted", 200},
		{"truncated", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n", "timeout", 200},
		{"html", "<html>synthetic-secret</html>", "timeout", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ids := providerGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}))
			result, err := s.CheckModel(t.Context(), ids["codex"], "synthetic-model")
			if err != nil || result.Available || result.ErrorCode != tc.code {
				t.Fatalf("failure: %+v %v", result, err)
			}
		})
	}
}

func TestCheckModelAdmissionAndCancellation(t *testing.T) {
	entered := make(chan struct{}, 1)
	s, ids := providerGateway(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	id := ids["codex"]
	if _, err := s.CheckModel(t.Context(), id, "invalid model"); !errors.Is(err, upstream.ErrInput) {
		t.Fatal("invalid input accepted", err)
	}
	if _, err := s.CheckModel(t.Context(), "missing-resource", "synthetic-model"); !errors.Is(err, accounts.ErrNotFound) {
		t.Fatal("unknown account accepted", err)
	}
	if result, err := s.CheckModel(t.Context(), id, "unsupported-model"); err != nil || result.ErrorCode != "model_not_available" {
		t.Fatal("unsupported model contacted upstream", result, err)
	}
	other := NewForTenant(t.Context(), s.db, accounts.NewForTenant(s.db, nil, 2), s.provider, 2)
	defer other.Close()
	if _, err := other.CheckModel(t.Context(), id, "synthetic-model"); !errors.Is(err, accounts.ErrNotFound) {
		t.Fatal("check crossed workspace boundaries", err)
	}
	t.Run("global capacity", func(t *testing.T) {
		for range cap(s.slots) {
			release, err := s.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			defer release()
		}
		if _, err := s.CheckModel(t.Context(), id, "synthetic-model"); !errors.Is(err, ErrBusy) {
			t.Fatal("check did not report global capacity", err)
		}
	})
	if err := s.SetConcurrency(t.Context(), id, 1); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan ModelCheck, 1)
	go func() { result, _ := s.CheckModel(ctx, id, "synthetic-model"); done <- result }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("check never reached upstream")
	}
	if result, err := s.CheckModel(t.Context(), id, "synthetic-model"); err != nil || result.ErrorCode != "account_busy" {
		t.Fatal("check bypassed account concurrency", result, err)
	}
	cancel()
	select {
	case result := <-done:
		if result.Available || result.ErrorCode != "client_disconnected" {
			t.Fatal("cancellation claimed success", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("check ignored cancellation")
	}
	if s.health[id].InFlight != 0 || len(s.slots) != 0 || len(s.modelHealth) != 0 {
		t.Fatal("canceled check leaked capacity")
	}
	if _, err := s.accounts.SetEnabled(t.Context(), id, false); err != nil {
		t.Fatal(err)
	}
	if result, err := s.CheckModel(t.Context(), id, "synthetic-model"); err != nil || result.ErrorCode != "account_unavailable" {
		t.Fatal("disabled resource contacted upstream", result, err)
	}
}

func TestCheckModelTranslatesProviderResponses(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity", "openai", "xai"} {
		t.Run(provider, func(t *testing.T) {
			s, ids := providerGateway(t, transportFunc(func(r *http.Request) (*http.Response, error) {
				body := checkResponse
				switch provider {
				case "claude":
					body = "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_synthetic\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n" +
						"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
						"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"OK\"}}\n\n" +
						"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
						"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n" +
						"data: {\"type\":\"message_stop\"}\n\n"
				case "antigravity":
					body = "data: {\"response\":{\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"OK\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}}\n\n"
				case "openai":
					if r.URL.Host != "relay.example.test" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-channel-key" {
						t.Error("channel endpoint or credential changed")
					}
					body = "data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"model\":\"synthetic-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"OK\"},\"finish_reason\":null}]}\n\n" +
						"data: {\"id\":\"synthetic-chat\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			if provider == "openai" || provider == "xai" {
				var row accounts.Account
				var err error
				if provider == "openai" {
					row, err = s.accounts.ImportProvider(t.Context(), provider, "Synthetic channel", []byte(`{"api_key":"synthetic-channel-key","base_url":"https://relay.example.test/v1"}`), "")
				} else {
					row, err = s.accounts.Authorize(t.Context(), "Synthetic Grok", accounts.Credential{Provider: provider, AccountID: "synthetic-subject", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
				}
				if err != nil {
					t.Fatal(err)
				}
				ids[provider] = row.ID
				if err := s.accounts.SaveCatalog(t.Context(), row.ID, 0, []string{"synthetic-model"}, s.now().Unix(), upstream.CatalogSource(provider)); err != nil {
					t.Fatal(err)
				}
			}
			result, err := s.CheckModel(t.Context(), ids[provider], "synthetic-model")
			if err != nil || !result.Available {
				t.Fatalf("provider check: %+v %v", result, err)
			}
		})
	}
}
