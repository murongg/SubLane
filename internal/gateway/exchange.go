package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/murongg/SubLane/internal/upstream"
)

// Exchange owns the account lease until body closure, including cancellation and partial streams.
type Exchange struct {
	*upstream.Stream
	mu                            sync.Mutex
	closeOnce                     sync.Once
	raw                           io.ReadCloser
	stop                          func() bool
	entry                         *observation
	closed                        bool
	outcome, code, penalty, retry string
}
type exchangeBody struct{ x *Exchange }

func (b *exchangeBody) Read(p []byte) (int, error) { return b.x.raw.Read(p) }
func (b *exchangeBody) Close() error               { return b.x.close() }
func trackExchange(stream *upstream.Stream, entry *observation) *Exchange {
	x := &Exchange{Stream: stream, entry: entry, raw: stream.Body, outcome: "canceled", code: "client_disconnected"}
	status := int64(stream.StatusCode)
	entry.record.UpstreamStatus = &status
	if status < 200 || status >= 300 {
		x.outcome, x.code, x.penalty = statusOutcome(int(status))
		x.retry = stream.Header.Get("Retry-After")
	}
	stream.Body = &exchangeBody{x: x}
	// Install the callback under the same mutex it acquires; canceled contexts may run it immediately.
	x.mu.Lock()
	x.stop = context.AfterFunc(entry.ctx, func() { x.close() })
	x.mu.Unlock()
	return x
}
func (x *Exchange) close() error {
	var err error
	x.closeOnce.Do(func() {
		err = x.raw.Close()
		x.mu.Lock()
		defer x.mu.Unlock()
		x.closed = true
		if x.stop != nil {
			x.stop()
		}
		if x.outcome == "canceled" && x.entry.ctx.Err() != nil {
			x.outcome, x.code, x.penalty = classify(x.entry.ctx, x.entry.ctx.Err())
		}
		x.entry.finish(x.outcome, x.code, x.penalty, x.retry)
	})
	return err
}
func (x *Exchange) Events(yield func([]byte) error) error {
	terminal := ""
	consumerError := false
	err := x.Stream.Events(func(raw []byte) error {
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		_ = json.Unmarshal(raw, &event)
		x.mu.Lock()
		if !x.closed {
			// Lifecycle events and empty deltas are not generated output. Missing observations
			// stay null, including compact or buffered responses without a measurable delta.
			if event.Delta != "" && x.entry.record.FirstTokenMs == nil {
				switch event.Type {
				case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta":
					elapsed := max(0, x.entry.service.now().Sub(x.entry.started).Milliseconds())
					x.entry.record.FirstTokenMs = &elapsed
				}
			}
			if event.Type == "response.completed" || event.Type == "response.incomplete" {
				terminal = event.Type
				x.entry.observe(raw)
			}
		}
		x.mu.Unlock()
		err := yield(raw)
		if err != nil {
			consumerError = true
		}
		return err
	})
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.closed {
		if err == nil {
			x.outcome = "success"
			if terminal == "response.incomplete" {
				x.outcome = "incomplete"
			}
			x.code = ""
			x.penalty = ""
		} else if consumerError {
			if x.entry.ctx.Err() != nil {
				x.outcome, x.code, x.penalty = classify(x.entry.ctx, x.entry.ctx.Err())
			} else if errors.Is(err, ErrContextLimit) {
				x.outcome, x.code, x.penalty = "rejected", "context_limit", ""
			} else {
				x.outcome, x.code, x.penalty = "canceled", "client_disconnected", ""
			}
		} else {
			x.outcome, x.code, x.penalty = classify(x.entry.ctx, err)
		}
	}
	return err
}
func (x *Exchange) Fail(err error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.closed {
		x.outcome, x.code, x.penalty = classify(x.entry.ctx, err)
	}
}
func (x *Exchange) Complete(ctx context.Context, event []byte) []byte {
	data := x.Stream.Complete(ctx, event)
	if !json.Valid(data) {
		x.Fail(upstream.ErrResponse)
	}
	return data
}
func (x *Exchange) AcceptCompact(raw []byte) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.closed {
		x.entry.observe(raw)
		x.outcome = "success"
		x.code = ""
		x.penalty = ""
	}
}
func statusOutcome(status int) (string, string, string) {
	switch {
	case status == 429:
		return "error", "rate_limited", "rate_limited"
	case status == 401:
		return "error", "auth_required", ""
	case status == 403:
		return "error", "upstream_forbidden", ""
	case status == 408:
		return "error", "timeout", "timeout"
	case status >= 500:
		return "error", "upstream_unavailable", "upstream_error"
	default:
		return "rejected", "upstream_rejected", ""
	}
}
