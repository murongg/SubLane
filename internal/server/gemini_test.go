package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/gateway"
)

const syntheticGemini = `{"responseId":"synthetic-response","modelVersion":"synthetic-model","candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"example.txt"}},"thoughtSignature":"synthetic-signature"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":5,"thoughtsTokenCount":2,"cachedContentTokenCount":12,"totalTokenCount":27}}`

func TestGeminiNativeJSONAndSSE(t *testing.T) {
	f := newProviderFixture(t, "antigravity", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model   string
			Request map[string]json.RawMessage
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != "synthetic-model" {
			t.Error("wrong upstream model/body")
		}
		for _, field := range []string{"contents", "systemInstruction", "tools", "generationConfig"} {
			if len(request.Request[field]) == 0 {
				t.Errorf("lost Gemini field %s", field)
			}
		}
		if r.URL.Query().Has("key") || r.Header.Get("X-Goog-Api-Key") != "" || r.Header.Get("Authorization") != "Bearer synthetic-upstream-access" {
			t.Error("client key forwarded upstream")
		}
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			var response map[string]json.RawMessage
			_ = json.Unmarshal([]byte(syntheticGemini), &response)
			usage := response["usageMetadata"]
			delete(response, "usageMetadata")
			chunk, _ := json.Marshal(response)
			io.WriteString(w, "data: {\"response\":"+string(chunk)+"}\n\n")
			io.WriteString(w, "data: {\"response\":{\"usageMetadata\":"+string(usage)+"}}\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"response":`+syntheticGemini+`}`)
		}
	})
	request := `{"model":"untrusted-body-model","stream":true,"contents":[{"role":"user","parts":[{"text":"Synthetic prompt"}]}],"systemInstruction":{"parts":[{"text":"Synthetic system"}]},"tools":[{"functionDeclarations":[{"name":"read_file","parameters":{"type":"OBJECT","properties":{"path":{"type":"STRING"}}}}]}],"generationConfig":{"maxOutputTokens":1024}}`
	for _, stream := range []bool{false, true} {
		path := "/v1beta/models/synthetic-model:generateContent"
		headers := http.Header{"X-Goog-Api-Key": {f.secret}}
		if stream {
			path = "/v1beta/models/synthetic-model:streamGenerateContent?alt=sse&key=" + f.secret
			headers = nil
		}
		response, raw := messageRequest(t, f, "POST", path, request, headers)
		if response.StatusCode != 200 || !strings.Contains(raw, "functionCall") || !strings.Contains(raw, "synthetic-signature") {
			t.Fatalf("Gemini failed: %d %s", response.StatusCode, raw)
		}
		if stream && (!strings.Contains(raw, "data: ") || strings.Contains(raw, "event:") || strings.Contains(raw, "[DONE]")) {
			t.Fatalf("invalid Gemini stream: %s", raw)
		}
		if !stream && (!json.Valid([]byte(raw)) || strings.Contains(raw, `"response":`)) {
			t.Fatalf("upstream envelope not unwrapped: %s", raw)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		page, err := f.forwarding.Requests(context.Background(), gateway.RequestFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Requests) == 2 {
			for _, record := range page.Requests {
				if record.Operation != "gemini" || record.InputTokens == nil || *record.InputTokens != 20 || record.OutputTokens == nil || *record.OutputTokens != 7 || record.CachedTokens == nil || *record.CachedTokens != 12 {
					t.Fatalf("wrong Gemini observation: %+v", record)
				}
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("missing Gemini request records")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestGeminiAuthenticationAndActionRouting(t *testing.T) {
	f := newProviderFixture(t, "antigravity", func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected upstream request") })
	for _, tc := range []struct {
		method, path, key, bearer string
		status                    int
		code                      string
	}{
		{"POST", "/v1beta/models/synthetic-model:generateContent", "", "", 401, "UNAUTHENTICATED"},
		{"POST", "/v1beta/models/synthetic-model:countTokens", f.secret, "", 404, "NOT_FOUND"},
		{"POST", "/v1beta/models/synthetic-model:delete", f.secret, "", 404, "NOT_FOUND"},
		{"GET", "/v1beta/models/synthetic-model:generateContent", f.secret, "", 405, "INVALID_ARGUMENT"},
		{"POST", "/v1beta/models/synthetic-model:generateContent", f.secret, "Bearer invalid", 401, "UNAUTHENTICATED"},
		{"POST", "/v1beta/models/synthetic-model:generateContent?key=invalid", f.secret, "", 401, "UNAUTHENTICATED"},
		{"POST", "/v1beta/models/synthetic-model:streamGenerateContent?alt=json", f.secret, "", 400, "INVALID_ARGUMENT"},
		{"POST", "/v1beta/missing", "", "", 401, "UNAUTHENTICATED"},
	} {
		response, raw := messageRequest(t, f, tc.method, tc.path, `{}`, http.Header{"X-Goog-Api-Key": {tc.key}, "Authorization": {tc.bearer}})
		var value struct {
			Error struct {
				Code   int
				Status string
			}
		}
		if json.Unmarshal([]byte(raw), &value) != nil || response.StatusCode != tc.status || value.Error.Code != tc.status || value.Error.Status != tc.code {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, response.StatusCode, raw)
		}
	}
}
