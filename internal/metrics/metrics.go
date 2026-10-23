// Package metrics collects bounded process-wide gateway telemetry without request content or identities.
package metrics

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

var outcomes = [...]string{"success", "incomplete", "error", "canceled", "rejected"}
var bounds = [...]float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300}

type histogram struct {
	Buckets [len(bounds)]uint64
	Count   uint64
	Sum     float64
}

func (h *histogram) observe(milliseconds int64) {
	seconds := float64(max(0, milliseconds)) / 1000
	h.Count++
	h.Sum += seconds
	for i, bound := range bounds {
		if seconds <= bound {
			h.Buckets[i]++
		}
	}
}

type Collector struct {
	mu              sync.Mutex
	requests        [len(outcomes)]uint64
	duration, first histogram
}

func (c *Collector) Observe(outcome string, durationMS int64, firstMS *int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	index := 2
	for i, value := range outcomes {
		if outcome == value {
			index = i
			break
		}
	}
	c.requests[index]++
	c.duration.observe(durationMS)
	// Unknown first-output latency is not a zero-latency sample.
	if firstMS != nil && *firstMS >= 0 {
		c.first.observe(*firstMS)
	}
}

func (c *Collector) Write(writer io.Writer, inFlight, queued int64) error {
	c.mu.Lock()
	requests, duration, first := c.requests, c.duration, c.first
	c.mu.Unlock()
	var body strings.Builder
	body.WriteString("# HELP sublane_gateway_requests_total Completed gateway observations, including local rejections.\n# TYPE sublane_gateway_requests_total counter\n")
	for i, outcome := range outcomes {
		fmt.Fprintf(&body, "sublane_gateway_requests_total{outcome=%q} %d\n", outcome, requests[i])
	}
	writeHistogram(&body, "sublane_gateway_duration_seconds", "Gateway observation duration in seconds.", duration)
	writeHistogram(&body, "sublane_gateway_first_output_seconds", "First output latency when observed, in seconds.", first)
	body.WriteString("# HELP sublane_gateway_in_flight Current account inference leases.\n# TYPE sublane_gateway_in_flight gauge\n")
	fmt.Fprintf(&body, "sublane_gateway_in_flight %d\n", inFlight)
	body.WriteString("# HELP sublane_gateway_queued Requests waiting for an account lease.\n# TYPE sublane_gateway_queued gauge\n")
	fmt.Fprintf(&body, "sublane_gateway_queued %d\n", queued)
	_, err := io.WriteString(writer, body.String())
	return err
}

func writeHistogram(body *strings.Builder, name, help string, h histogram) {
	fmt.Fprintf(body, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	for i, bound := range bounds {
		fmt.Fprintf(body, "%s_bucket{le=%q} %d\n", name, strconv.FormatFloat(bound, 'f', -1, 64), h.Buckets[i])
	}
	fmt.Fprintf(body, "%s_bucket{le=\"+Inf\"} %d\n%s_sum %s\n%s_count %d\n", name, h.Count, name, strconv.FormatFloat(h.Sum, 'f', -1, 64), name, h.Count)
}
