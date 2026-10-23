package gateway

import (
	"context"
	"testing"
)

func TestRuntimeUsesBatchedReads(t *testing.T) {
	s, meter := measuredGateway(t, 20)
	states, err := s.Runtime(context.Background())
	if err != nil || len(states) != 20 {
		t.Fatal("incomplete runtime snapshot", len(states), err)
	}
	if count := len(meter.take()); count > 3 {
		t.Fatalf("runtime required %d SQL calls for 20 accounts; budget is 3", count)
	}
}
