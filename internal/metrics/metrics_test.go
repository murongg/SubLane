package metrics

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestPrometheusMetricsKeepFixedLabelsAndUnknownLatency(t *testing.T) {
	c := &Collector{}
	first := int64(100)
	c.Observe("success", 250, &first)
	c.Observe("rejected", 10, nil)
	c.Observe("synthetic-client-controlled-label", 5, nil)
	var body bytes.Buffer
	if err := c.Write(&body, 2, 3); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`sublane_gateway_requests_total{outcome="success"} 1`, `sublane_gateway_requests_total{outcome="rejected"} 1`, `sublane_gateway_first_output_seconds_count 1`, `sublane_gateway_first_output_seconds_bucket{le="0.1"} 1`, `sublane_gateway_in_flight 2`, `sublane_gateway_queued 3`} {
		if !strings.Contains(body.String(), want) {
			t.Fatalf("missing %s in %s", want, body.String())
		}
	}
	if strings.Contains(body.String(), "synthetic-client") {
		t.Fatal("unbounded label exported")
	}
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			for range 20 {
				c.Observe("success", 25, nil)
				var out bytes.Buffer
				_ = c.Write(&out, 0, 0)
			}
		})
	}
	wg.Wait()
}
