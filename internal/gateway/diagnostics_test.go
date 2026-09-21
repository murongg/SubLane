package gateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage/db"
)

func TestRequestIdentityAttachesAuthenticationButDoesNotReuseCompletedIdentity(t *testing.T) {
	pending := WithRequestIdentity(context.Background(), 0, "http")
	admitted := WithRequestIdentity(pending, 42, "http")
	if RequestID(pending) != RequestID(admitted) {
		t.Fatal("native header and record IDs diverged")
	}
	next := WithRequestIdentity(admitted, 42, "http")
	if RequestID(admitted) == RequestID(next) {
		t.Fatal("new request reused an admitted request ID")
	}
}

func TestDiagnosticsMeasuresFirstOutputNotCreatedOrCompletion(t *testing.T) {
	for _, delta := range []string{"response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta", ""} {
		t.Run(delta, func(t *testing.T) {
			stream := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"synthetic\"}}\n\n"
			if delta != "" {
				stream += "data: {\"type\":\"" + delta + "\",\"delta\":\"synthetic\"}\n\n"
			}
			stream += "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[]}}\n\n"
			s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
			}))
			clock := time.Now()
			s.now = func() time.Time { return clock }
			ctx := WithRequestIdentity(context.Background(), 42, "http")
			x, err := s.Open(ctx, 1, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
			if err != nil {
				t.Fatal(err)
			}
			if err := x.Events(func([]byte) error { clock = clock.Add(30 * time.Millisecond); return nil }); err != nil {
				t.Fatal(err)
			}
			x.Body.Close()
			page, err := s.Requests(ctx, RequestFilter{})
			if err != nil || len(page.Requests) != 1 {
				t.Fatal(err)
			}
			r := page.Requests[0]
			if r.RequestID == "" || r.RequestID != RequestID(ctx) {
				t.Fatal("correlation lost", r.RequestID)
			}
			if delta == "" {
				if r.FirstTokenMs != nil {
					t.Fatal("completion fabricated first token")
				}
			} else if r.FirstTokenMs == nil || *r.FirstTokenMs != 30 {
				t.Fatal("wrong first output latency", r.FirstTokenMs)
			}
		})
	}
}

func TestDiagnosticFiltersComposeBeforePaginationAndPreserveOwner(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	now := time.Now().Unix()
	for i := int64(1); i <= 120; i++ {
		model := "synthetic-a"
		if i%2 == 0 {
			model = "synthetic-b"
		}
		if err := s.queries.RecordRequest(ctx, db.RecordRequestParams{UserID: i%3 + 1, KeyID: i%4 + 1, GroupID: 1, Model: model, Transport: "http", Operation: "responses", Outcome: "success", StartedAt: now - i, RequestID: "synthetic-id"}); err != nil {
			t.Fatal(err)
		}
	}
	filter := RequestFilter{Model: "synthetic-a", KeyID: 2, From: now - 90, Until: now - 10, MemberID: 2, RequestID: "synthetic-id"}
	page, err := s.Requests(ctx, filter)
	if err != nil || len(page.Requests) != 7 {
		t.Fatal("composed filters incorrect", len(page.Requests), err)
	}
	for _, r := range page.Requests {
		if r.UserID != 2 || r.KeyID != 2 || r.Model != "synthetic-a" || r.StartedAt < filter.From || r.StartedAt >= filter.Until {
			t.Fatal("filter escaped", r)
		}
	}
	filter.MemberID = 1
	filter.AccountID = "another-account"
	personal, err := s.UserRequests(ctx, 2, filter)
	if err != nil || len(personal.Requests) != len(page.Requests) {
		t.Fatal("personal filters changed SQL ownership", err, len(personal.Requests))
	}
	for _, bad := range []RequestFilter{{From: now, Until: now - 1}, {KeyID: -1}, {MemberID: -1}, {Model: strings.Repeat("x", 161)}, {RequestID: "unsafe\nidentifier"}} {
		if _, err := s.Requests(ctx, bad); err == nil {
			t.Fatal("invalid filter admitted", bad)
		}
	}
}
