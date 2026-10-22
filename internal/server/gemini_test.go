package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/groups"
)

const syntheticGemini = `{"responseId":"synthetic-response","modelVersion":"synthetic-model","candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"example.txt"}},"thoughtSignature":"synthetic-signature"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":5,"thoughtsTokenCount":2,"cachedContentTokenCount":12,"totalTokenCount":27}}`

func TestHiddenGeminiModelsForwardWithoutListingOrBypassingPolicy(t *testing.T) {
	forwarded := 0
	f := newProviderFixture(t, "antigravity", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1internal:fetchAvailableModels" {
			io.WriteString(w, `{"models":{"synthetic-model":{},"synthetic-internal":{"isInternal":true}}}`)
			return
		}
		var input struct{ Model string }
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Model != "synthetic-internal" {
			t.Error("internal model ID changed", input.Model)
		}
		forwarded++
		io.WriteString(w, `{"response":`+syntheticGemini+`}`)
	})
	ctx := context.Background()
	rows, err := f.accounts.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	id := rows[0].ID
	saved, err := f.accounts.Catalog(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.accounts.SaveCatalog(ctx, id, saved.Revision, saved.Models, time.Now().Unix(), "antigravity:v1"); err != nil {
		t.Fatal(err)
	}
	if catalog, err := f.forwarding.AccountCatalog(ctx, id, true); err != nil || !catalog.Known || len(catalog.Models) != 1 || catalog.RefreshFailed {
		t.Fatal(err)
	}
	h := f.server.Config.Handler
	login := func(name, password string) *http.Cookie {
		response := request(h, "POST", "/api/auth/login", "http://example.test", map[string]string{"username": name, "password": password}, nil)
		if response.Code != 200 {
			t.Fatal("login failed", response.Code)
		}
		return response.Result().Cookies()[0]
	}
	admin := login("owner-test", "owner pass 42")
	member := login("member-test", "member pass 42")
	assertListings := func(hidden ...string) {
		t.Helper()
		response, raw := messageRequest(t, f, "GET", "/v1/models", "", http.Header{"Authorization": {"Bearer " + f.secret}})
		if response.StatusCode != 200 || strings.Contains(raw, "synthetic-internal") {
			t.Fatal("internal model entered public list", response.StatusCode, raw)
		}
		for _, test := range []struct {
			path   string
			cookie *http.Cookie
		}{{"/api/accounts/" + id + "/models", admin}, {"/api/groups/1/models", admin}, {fmt.Sprintf("/api/keys/%d/models", f.keyID), member}} {
			response := request(h, "GET", test.path, "", nil, test.cookie)
			want := hidden
			if strings.HasPrefix(test.path, "/api/accounts/") {
				want = []string{"synthetic-internal"}
			}
			assertHiddenCatalog(t, response, want...)
		}
	}
	assertListings("synthetic-internal")
	call := func(model string) int {
		response, _ := messageRequest(t, f, "POST", "/v1beta/models/"+model+":generateContent", `{"contents":[{"parts":[{"text":"Synthetic prompt"}]}]}`, http.Header{"X-Goog-Api-Key": {f.secret}})
		return response.StatusCode
	}
	if status := call("synthetic-internal"); status != 200 || forwarded != 1 {
		t.Fatal("hidden model did not forward", status, forwarded)
	}
	if status := call("synthetic-unknown"); status != 404 || forwarded != 1 {
		t.Fatal("unknown model forwarded", status, forwarded)
	}
	input := groups.Input{Name: "Synthetic Gemini", Enabled: true, AccountIDs: []string{id}, ModelPolicy: &groups.ModelPolicy{Restricted: true, Models: []string{"synthetic-model"}}}
	if _, err := f.groups.Save(ctx, 1, input); err != nil {
		t.Fatal(err)
	}
	assertListings()
	if status := call("synthetic-internal"); status != 403 || forwarded != 1 {
		t.Fatal("hidden model bypassed model policy", status, forwarded)
	}
	input.ModelPolicy.Models = []string{"synthetic-internal"}
	if _, err := f.groups.Save(ctx, 1, input); err != nil {
		t.Fatal(err)
	}
	assertListings("synthetic-internal")
	if status := call("synthetic-internal"); status != 200 || forwarded != 2 {
		t.Fatal("explicit permission could not call a hidden model", status, forwarded)
	}
	if err := f.groups.SetMemberGroups(ctx, f.userID, []int64{}); err != nil {
		t.Fatal(err)
	}
	if status := call("synthetic-internal"); status != 401 || forwarded != 2 {
		t.Fatal("hidden model bypassed group access", status, forwarded)
	}
	if err := f.groups.SetMemberGroups(ctx, f.userID, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.keys.Update(ctx, f.userID, f.keyID, apikey.UpdateInput{Name: "Synthetic key", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if status := call("synthetic-internal"); status != 401 || forwarded != 2 {
		t.Fatal("hidden model bypassed paused key", status, forwarded)
	}
}

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
