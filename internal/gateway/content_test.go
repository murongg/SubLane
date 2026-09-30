package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/murongg/SubLane/internal/content"
	"github.com/murongg/SubLane/internal/logging"
	"github.com/murongg/SubLane/internal/vault"
)

func TestContentDoesNotDiscloseAMatchedModelInDiagnostics(t *testing.T) {
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("blocked request reached provider")
		return nil, nil
	}))
	v, err := vault.Open(filepath.Join(t.TempDir(), "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	rules := content.New(s.db, v, 1)
	secret := "SYNTHETIC_SECRET_MODEL"
	if _, err = rules.Update(context.Background(), content.Input{Mode: "block", Rules: []content.RuleInput{{Name: "Synthetic", Kind: "text", Pattern: &secret, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	s.SetContent(rules)
	var buffer bytes.Buffer
	ctx := logging.WithLogger(context.Background(), slog.New(slog.NewJSONHandler(&buffer, nil)))
	if _, err = s.Open(ctx, 1, 1, []byte(`{"model":"SYNTHETIC_SECRET_MODEL","input":"safe"}`), nil, Responses); !errors.Is(err, content.ErrBlocked) {
		t.Fatal(err)
	}
	if _, err = s.Open(ctx, 1, 1, []byte(`{"model":"SYNTHETIC_SECRET_MODEL","input":"safe","prompt_cache_key":{}}`), nil, Responses); !errors.Is(err, content.ErrBlocked) {
		t.Fatal("unsafe model was recorded before an unrelated input failure", err)
	}
	page, err := s.Requests(context.Background(), RequestFilter{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), secret) || strings.Contains(buffer.String(), secret) {
		t.Fatal("matched model disclosed through metadata")
	}
}

func TestContentBlockBeforeProviderIOAcrossProtocols(t *testing.T) {
	for _, tc := range []struct {
		kind Kind
		raw  string
	}{
		{Responses, `{"model":"synthetic-model","input":"SYNTHETIC_SECRET"}`},
		{Chat, `{"model":"synthetic-model","messages":[{"role":"user","content":"SYNTHETIC_SECRET"}]}`},
		{Compact, `{"model":"synthetic-model","input":"SYNTHETIC_SECRET"}`},
		{Messages, `{"model":"synthetic-model","max_tokens":10,"messages":[{"role":"user","content":"SYNTHETIC_SECRET"}]}`},
		{Gemini, `{"model":"synthetic-model","contents":[{"parts":[{"text":"SYNTHETIC_SECRET"}]}]}`},
		{GeminiStream, `{"model":"synthetic-model","contents":[{"parts":[{"text":"SYNTHETIC_SECRET"}]}]}`},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var calls atomic.Int32
			s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("unexpected upstream")
			}))
			v, err := vault.Open(filepath.Join(t.TempDir(), "content-key"), true)
			if err != nil {
				t.Fatal(err)
			}
			rules := content.New(s.db, v, 1)
			secret := "SYNTHETIC_SECRET"
			saved, err := rules.Update(context.Background(), content.Input{Mode: "block", Rules: []content.RuleInput{{Name: "Synthetic", Kind: "text", Pattern: &secret, Enabled: true}}})
			if err != nil {
				t.Fatal(err)
			}
			s.SetContent(rules)
			_, err = s.Open(context.Background(), 1, 1, []byte(tc.raw), nil, tc.kind)
			if !errors.Is(err, content.ErrBlocked) || calls.Load() != 0 {
				t.Fatalf("rejection reached provider: %v calls=%d", err, calls.Load())
			}
			page, err := s.Requests(context.Background(), RequestFilter{})
			if err != nil || len(page.Requests) != 1 {
				t.Fatal(page, err)
			}
			r := page.Requests[0]
			if r.Outcome != "rejected" || r.ErrorCode != "content_policy_blocked" || r.AccountID != "" || r.UpstreamStatus != nil || r.Content == nil || r.Content.Revision != saved.Revision || len(r.Content.RuleIDs) != 1 {
				t.Fatalf("unsafe rejection: %+v", r)
			}
			raw, _ := json.Marshal(page)
			if strings.Contains(string(raw), secret) {
				t.Fatal("history contains secret")
			}
		})
	}
}

func TestContentObservationPreservesRequestAndHistory(t *testing.T) {
	var sent string
	s, _ := codexGateway(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		sent = string(raw)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"output\":[]}}\n\n"))}, nil
	}))
	v, _ := vault.Open(filepath.Join(t.TempDir(), "key"), true)
	rules := content.New(s.db, v, 1)
	secret := "SYNTHETIC_SECRET"
	if _, err := rules.Update(context.Background(), content.Input{Mode: "observe", Rules: []content.RuleInput{{Name: "Synthetic", Kind: "text", Pattern: &secret, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	s.SetContent(rules)
	x, err := s.Open(context.Background(), 1, 1, []byte(`{"model":"synthetic-model","input":"SYNTHETIC_SECRET"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if err = x.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	x.Body.Close()
	if !strings.Contains(sent, secret) {
		t.Fatal("observation changed content")
	}
	page, err := s.UserRequests(context.Background(), 1, RequestFilter{})
	if err != nil || len(page.Requests) != 1 || page.Requests[0].Content == nil || page.Requests[0].Content.Mode != "observe" || len(page.Requests[0].Content.RuleIDs) != 1 {
		t.Fatal(page, err)
	}
}
