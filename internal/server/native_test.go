package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/groups"
)

func nativeTestRequest(provider string) (path, body, keyHeader string) {
	if provider == "claude" {
		return "/v1/messages", `{"model":"synthetic-model","max_tokens":128,"messages":[{"role":"user","content":"Synthetic input"}]}`, "X-Api-Key"
	}
	return "/v1beta/models/synthetic-model:generateContent", `{"contents":[{"parts":[{"text":"Synthetic input"}]}]}`, "X-Goog-Api-Key"
}

func TestNativeProtocolsEnforceMemberGroupModelAndKeyPolicies(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			f := newProviderFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if provider == "claude" {
					io.WriteString(w, syntheticMessage)
				} else {
					io.WriteString(w, `{"response":`+syntheticGemini+`}`)
				}
			})
			ctx := context.Background()
			if err := f.forwarding.SetMemberLimits(ctx, f.userID, 1, 1); err != nil {
				t.Fatal(err)
			}
			path, body, keyHeader := nativeTestRequest(provider)
			call := func(secret string) *http.Response {
				response, _ := messageRequest(t, f, "POST", path, body, http.Header{keyHeader: {secret}})
				return response
			}
			if response := call(f.secret); response.StatusCode != 200 {
				t.Fatal("initial call failed", response.StatusCode)
			}
			second, err := f.keys.Create(ctx, f.userID, "Synthetic second key")
			if err != nil {
				t.Fatal(err)
			}
			if response := call(second.Secret); response.StatusCode != 429 || response.Header.Get("Retry-After") == "" {
				t.Fatal("member limit bypassed", response.StatusCode)
			}
			if err := f.forwarding.SetMemberLimits(ctx, f.userID, 0, 0); err != nil {
				t.Fatal(err)
			}
			accounts, err := f.accounts.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.groups.Save(ctx, groups.DefaultID, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{accounts[0].ID}, ModelPolicy: &groups.ModelPolicy{Restricted: true, Models: []string{"other-model"}}}); err != nil {
				t.Fatal(err)
			}
			if response := call(f.secret); response.StatusCode != 403 {
				t.Fatal("model allowlist bypassed", response.StatusCode)
			}
			if err := f.groups.SetMemberGroups(ctx, f.userID, []int64{}); err != nil {
				t.Fatal(err)
			}
			if response := call(f.secret); response.StatusCode != 401 {
				t.Fatal("group revocation bypassed", response.StatusCode)
			}
			if _, err := f.keys.Revoke(ctx, f.userID, f.keyID); err != nil {
				t.Fatal(err)
			}
			if response := call(f.secret); response.StatusCode != 401 {
				t.Fatal("key revocation bypassed", response.StatusCode)
			}
			if calls.Load() != 1 {
				t.Fatal("rejected request reached upstream", calls.Load())
			}
		})
	}
}

func TestNativeStreamingCancellationReleasesAccountAndMemberSlots(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		t.Run(provider, func(t *testing.T) {
			canceled := make(chan struct{}, 1)
			f := newProviderFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if provider == "claude" {
					io.WriteString(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"synthetic\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
				} else {
					io.WriteString(w, `data: {"response":{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Synthetic delta"}]}}]}}`+"\n\n")
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				canceled <- struct{}{}
			})
			if err := f.forwarding.SetMemberLimits(context.Background(), f.userID, 0, 1); err != nil {
				t.Fatal(err)
			}
			path, body, header := nativeTestRequest(provider)
			if provider == "claude" {
				body = strings.TrimSuffix(body, "}") + `,"stream":true}`
			} else {
				path = strings.ReplaceAll(path, ":generateContent", ":streamGenerateContent") + "?alt=sse"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r, _ := http.NewRequestWithContext(ctx, "POST", f.server.URL+path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set(header, f.secret)
			response, err := http.DefaultClient.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			reader := bufio.NewReader(response.Body)
			if _, err := reader.ReadString('\n'); err != nil {
				t.Fatal(err)
			}
			cancel()
			response.Body.Close()
			select {
			case <-canceled:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream was not canceled")
			}
			deadline := time.Now().Add(2 * time.Second)
			for {
				page, err := f.forwarding.Requests(context.Background(), gateway.RequestFilter{})
				if err != nil {
					t.Fatal(err)
				}
				limits, err := f.forwarding.MemberLimits(context.Background(), f.userID)
				if err != nil {
					t.Fatal(err)
				}
				pool, err := f.forwarding.Runtime(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Requests) == 1 && page.Requests[0].Outcome == "canceled" && limits.InFlight == 0 && len(pool) == 1 && pool[0].InFlight == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("cancellation did not release lease: %+v %+v", page, limits)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestNativeMidstreamErrorsRemainProtocolCompatible(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		f := newProviderFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			if provider == "claude" {
				io.WriteString(w, `data: {"type":"message_start","message":{"id":"synthetic","type":"message","role":"assistant","content":[]}}`+"\n\n")
			} else {
				io.WriteString(w, `data: {"response":{"candidates":[{"index":0,"content":{"parts":[{"text":"Synthetic output"}]}}]}}`+"\n\n")
			}
			w.(http.Flusher).Flush()
			io.WriteString(w, `data: {"type":"error","error":{"code":529,"type":"overloaded_error","message":"synthetic-private-upstream-content"}}`+"\n\n")
		})
		path, body, header := nativeTestRequest(provider)
		if provider == "claude" {
			body = strings.TrimSuffix(body, "}") + `,"stream":true}`
		} else {
			path = strings.ReplaceAll(path, ":generateContent", ":streamGenerateContent") + "?alt=sse"
		}
		response, raw := messageRequest(t, f, "POST", path, body, http.Header{header: {f.secret}})
		if response.StatusCode != 200 || !strings.Contains(raw, `"error"`) || strings.Contains(raw, "private-upstream") || strings.Contains(raw, "[DONE]") {
			t.Fatalf("unsafe SSE failure: %d %s", response.StatusCode, raw)
		}
		if provider == "claude" && !strings.Contains(raw, "event: error\n") {
			t.Fatal("missing native error event", raw)
		}
	}
}

func TestNativeFormatsRouteAcrossProviderAccounts(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		f := newProviderFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if provider == "claude" {
				var request struct{ Stream bool }
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				// The SDK requests a Claude SSE stream when converting to another protocol, even for a buffered caller.
				if request.Stream {
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "data: "+syntheticMessageEvents()+"\n\n")
				} else {
					io.WriteString(w, syntheticMessage)
				}
			} else {
				io.WriteString(w, `{"response":`+syntheticGemini+`}`)
			}
		})
		other := "claude"
		if provider == "claude" {
			other = "antigravity"
		}
		path, body, header := nativeTestRequest(other)
		response, raw := messageRequest(t, f, "POST", path, body, http.Header{header: {f.secret}})
		if response.StatusCode != 200 || !strings.Contains(raw, "read_file") {
			t.Fatalf("cross-provider %s: %d %s", provider, response.StatusCode, raw)
		}
	}
}

func TestClientProtocolMatrix(t *testing.T) {
	for _, provider := range []string{"codex", "claude", "antigravity"} {
		for _, protocol := range []string{"responses", "chat", "claude", "gemini"} {
			for _, stream := range []bool{false, true} {
				mode := "json"
				if stream {
					mode = "sse"
				}
				t.Run(provider+"/"+protocol+"/"+mode, func(t *testing.T) {
					f := newProviderFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
						var input struct{ Stream bool }
						if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
							t.Error(err)
						}
						if provider == "codex" {
							w.Header().Set("Content-Type", "text/event-stream")
							io.WriteString(w, syntheticCodexToolEvents())
						} else if provider == "claude" {
							if input.Stream {
								w.Header().Set("Content-Type", "text/event-stream")
								io.WriteString(w, "data: "+syntheticMessageEvents()+"\n\n")
							} else {
								w.Header().Set("Content-Type", "application/json")
								io.WriteString(w, syntheticMessage)
							}
						} else if strings.Contains(r.URL.Path, "streamGenerateContent") {
							w.Header().Set("Content-Type", "text/event-stream")
							io.WriteString(w, "data: {\"response\":"+syntheticGemini+"}\n\n")
						} else {
							w.Header().Set("Content-Type", "application/json")
							io.WriteString(w, `{"response":`+syntheticGemini+`}`)
						}
					})
					client := protocol
					if protocol == "gemini" {
						client = "antigravity"
					}
					path, body, header := nativeTestRequest(client)
					secret := f.secret
					if protocol == "responses" {
						path, body, header, secret = "/v1/responses", `{"model":"synthetic-model","input":"Synthetic prompt"}`, "Authorization", "Bearer "+f.secret
					} else if protocol == "chat" {
						path, body, header, secret = "/v1/chat/completions", `{"model":"synthetic-model","messages":[{"role":"user","content":"Synthetic prompt"}]}`, "Authorization", "Bearer "+f.secret
					}
					if stream {
						if protocol != "gemini" {
							body = strings.TrimSuffix(body, "}") + `,"stream":true}`
						} else {
							path = strings.ReplaceAll(path, ":generateContent", ":streamGenerateContent") + "?alt=sse"
						}
					}
					response, raw := messageRequest(t, f, "POST", path, body, http.Header{header: {secret}})
					if response.StatusCode != 200 || !strings.Contains(raw, "read_file") {
						t.Fatalf("protocol conversion failed: %d %s", response.StatusCode, raw)
					}
					if !stream && !json.Valid([]byte(raw)) {
						t.Fatal("invalid buffered response", raw)
					}
					if stream {
						terminal := map[string]string{"responses": "event: response.completed", "chat": "data: [DONE]", "claude": "event: message_stop", "gemini": "finishReason"}[protocol]
						if !strings.Contains(raw, terminal) || protocol != "chat" && strings.Contains(raw, "[DONE]") {
							t.Fatal("invalid native stream", raw)
						}
					}
				})
			}
		}
	}
}

func syntheticCodexToolEvents() string {
	item := `{"type":"function_call","id":"fc_synthetic","call_id":"call_synthetic","name":"read_file","arguments":"{\"path\":\"example.txt\"}"}`
	events := []string{
		`{"type":"response.created","response":{"id":"resp_synthetic","model":"synthetic-model","status":"in_progress","output":[]}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_synthetic","call_id":"call_synthetic","name":"read_file","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_synthetic","delta":"{\"path\":\"example.txt\"}"}`,
		`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_synthetic","arguments":"{\"path\":\"example.txt\"}"}`,
		`{"type":"response.output_item.done","output_index":0,"item":` + item + `}`,
		`{"type":"response.completed","response":{"id":"resp_synthetic","model":"synthetic-model","status":"completed","output":[` + item + `],"usage":{"input_tokens":20,"output_tokens":7,"total_tokens":27,"input_tokens_details":{"cached_tokens":12}}}}`,
	}
	return "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
}

func TestNativeErrorsAreSanitizedAndPreserveRetryAfter(t *testing.T) {
	for _, provider := range []string{"claude", "antigravity"} {
		f := newProviderFixture(t, provider, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "4")
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"message":"synthetic-private-upstream-content"}}`)
		})
		path, body, header := nativeTestRequest(provider)
		response, raw := messageRequest(t, f, "POST", path, body, http.Header{header: {f.secret}})
		if response.StatusCode != 429 || response.Header.Get("Retry-After") != "4" || strings.Contains(raw, "private-upstream") {
			t.Fatalf("unsafe native error %s: %d %s", provider, response.StatusCode, raw)
		}
	}
}
