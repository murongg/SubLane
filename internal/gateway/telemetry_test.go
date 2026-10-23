package gateway

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/metrics"
)

func TestTelemetryCountsARequestOnceAndReadsLeasePressure(t *testing.T) {
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	c := &metrics.Collector{}
	s.SetTelemetry(c)
	r, err := s.Open(context.Background(), 1, 1, []byte(`{"model":"synthetic-model","input":"synthetic"}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	if active, _ := s.Pressure(); active != 1 {
		t.Fatal("active lease not observed", active)
	}
	if err := r.Events(func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	r.Body.Close()
	active, queued := s.Pressure()
	if active != 0 || queued != 0 {
		t.Fatal("closed lease retained", active, queued)
	}
	var body bytes.Buffer
	if err := c.Write(&body, active, queued); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), `sublane_gateway_requests_total{outcome="success"} 1`) {
		t.Fatal(body.String())
	}
}
