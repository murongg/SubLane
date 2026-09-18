package gateway

import (
	"encoding/json"
	"errors"
	"github.com/murongg/SubLane/internal/codex"
	"testing"
)

func TestConversationReconstructsIncrementalInputAndCompaction(t *testing.T) {
	conversation := Conversation{}
	initial, prewarm, err := conversation.Normalize([]byte(`{"type":"response.create","model":"synthetic-model","input":[{"role":"user","content":"first"}]}`))
	if err != nil || prewarm {
		t.Fatal(err)
	}
	if err := conversation.Accept(initial, []byte(`{"type":"response.completed","response":{"id":"resp_one","output":[{"type":"function_call","call_id":"call_test","name":"synthetic","arguments":"{}"}]}}`)); err != nil {
		t.Fatal(err)
	}
	next, _, err := conversation.Normalize([]byte(`{"type":"response.create","previous_response_id":"resp_one","input":[{"type":"function_call_output","call_id":"call_test","output":"synthetic output"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Model    string            `json:"model"`
		Input    []json.RawMessage `json:"input"`
		Previous string            `json:"previous_response_id"`
	}
	if json.Unmarshal(next, &request) != nil || len(request.Input) != 3 || request.Previous != "" || request.Model != "synthetic-model" {
		t.Fatal("incremental context lost")
	}
	if _, _, err := conversation.Normalize([]byte(`{"type":"response.create","previous_response_id":"resp_other","input":[]}`)); !errors.Is(err, codex.ErrContinuation) {
		t.Fatal("unknown continuation accepted", err)
	}
	replaced, _, err := conversation.Normalize([]byte(`{"type":"response.create","model":"synthetic-model","input":[{"type":"compaction","encrypted_content":"synthetic-compaction"},{"role":"user","content":"after compaction"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(replaced, &request) != nil || len(request.Input) != 2 {
		t.Fatal("stale transcript merged into compact replay")
	}
}
