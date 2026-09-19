package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This opt-in integration test uses only a local fake upstream and fresh child-process configuration.
func TestCodexCLIProtocol(t *testing.T) {
	if os.Getenv("SUBLANE_TEST_CODEX") != "1" {
		t.Skip("set SUBLANE_TEST_CODEX=1 to exercise an installed Codex CLI")
	}
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, websockets := range []bool{false, true} {
		t.Run(fmt.Sprintf("websockets_%v", websockets), func(t *testing.T) {
			fixture := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"slug": "synthetic-model", "visibility": "list"}}})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				part := map[string]any{"type": "output_text", "text": "SUBLANE_SYNTHETIC_OK", "annotations": []any{}}
				item := map[string]any{"type": "message", "id": "msg_synthetic", "role": "assistant", "status": "completed", "content": []any{part}}
				response := map[string]any{"id": "resp_synthetic", "object": "response", "created_at": 1900000000, "status": "in_progress", "model": "synthetic-model", "output": []any{}}
				event := func(payload map[string]any) {
					raw, _ := json.Marshal(payload)
					fmt.Fprintf(w, "data: %s\n\n", raw)
					w.(http.Flusher).Flush()
				}
				event(map[string]any{"type": "response.created", "response": response})
				event(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "message", "id": "msg_synthetic", "role": "assistant", "status": "in_progress", "content": []any{}}})
				event(map[string]any{"type": "response.content_part.added", "item_id": "msg_synthetic", "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
				event(map[string]any{"type": "response.output_text.delta", "item_id": "msg_synthetic", "output_index": 0, "content_index": 0, "delta": "SUBLANE_SYNTHETIC_OK"})
				event(map[string]any{"type": "response.output_text.done", "item_id": "msg_synthetic", "output_index": 0, "content_index": 0, "text": "SUBLANE_SYNTHETIC_OK"})
				event(map[string]any{"type": "response.content_part.done", "item_id": "msg_synthetic", "output_index": 0, "content_index": 0, "part": part})
				event(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
				response["status"] = "completed"
				response["output"] = []any{item}
				response["usage"] = map[string]int{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}
				event(map[string]any{"type": "response.completed", "response": response})
			})
			directory := t.TempDir()
			home := filepath.Join(directory, "client")
			workspace := filepath.Join(directory, "workspace")
			if err := os.MkdirAll(home, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(directory, "result.txt")
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, "exec", "--ignore-user-config", "--ignore-rules", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "--color", "never", "-C", workspace, "--model", "synthetic-model", "--output-last-message", output,
				"-c", `cli_auth_credentials_store="file"`, "-c", `model_provider="sublane"`, "-c", `model_providers.sublane.name="SubLane synthetic test"`, "-c", "model_providers.sublane.base_url="+strconv.Quote(fixture.server.URL+"/v1"), "-c", `model_providers.sublane.env_key="SUBLANE_TEST_API_KEY"`, "-c", `model_providers.sublane.wire_api="responses"`, "-c", "model_providers.sublane.supports_websockets="+strconv.FormatBool(websockets), "-c", "model_providers.sublane.request_max_retries=0", "Return the fixed synthetic response. Do not call tools.")
			// Isolate only this child; real config, credentials, keychains, skills, and proxy secrets are not inherited.
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "CODEX_HOME=" + home, "TMPDIR=" + directory, "LANG=C.UTF-8", "SUBLANE_TEST_API_KEY=" + fixture.secret, "HTTP_PROXY=http://127.0.0.1:9", "HTTPS_PROXY=http://127.0.0.1:9", "NO_PROXY=127.0.0.1,localhost"}
			logs, err := command.CombinedOutput()
			if err != nil {
				if len(logs) > 2500 {
					logs = logs[len(logs)-2500:]
				}
				t.Fatalf("isolated Codex CLI failed: %v\n%s", err, logs)
			}
			result, err := os.ReadFile(output)
			if err != nil || strings.TrimSpace(string(result)) != "SUBLANE_SYNTHETIC_OK" {
				t.Fatalf("CLI did not consume the synthetic response: %v", err)
			}
			if websockets && fixture.upgrades.Load() == 0 {
				t.Fatal("CLI did not exercise WebSocket transport")
			}
			if !websockets && fixture.upgrades.Load() != 0 {
				t.Fatal("HTTP test unexpectedly upgraded to WebSocket")
			}
		})
	}
}
