package versions

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func releaseResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestOfficialReleaseSelectionAndHeaders(t *testing.T) {
	calls := 0
	source := NewReleases(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.github.com" || r.URL.Path != "/repos/openai/codex/releases/latest" || r.Method != "GET" {
			t.Fatal("unexpected release source", r.URL)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") == "" {
			t.Fatal("release request leaked credentials or omitted identity")
		}
		return releaseResponse(200, `{"tag_name":"rust-v0.200.1","draft":false,"prerelease":false}`), nil
	}))
	defer source.Close()
	value, err := source.Latest(context.Background())
	if err != nil || value != "0.200.1" || calls != 1 {
		t.Fatal(value, err, calls)
	}
}

func TestReleaseFallbackRejectsOtherComponentsAndPrereleases(t *testing.T) {
	calls := 0
	source := NewReleases(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/latest") {
			return releaseResponse(200, `{"tag_name":"rusty-v8-99.0.0","draft":false,"prerelease":false}`), nil
		}
		if r.URL.Query().Get("per_page") != "30" {
			t.Fatal("unbounded fallback")
		}
		return releaseResponse(200, `[{"tag_name":"rust-v0.900.0-alpha.1","draft":false,"prerelease":true},{"tag_name":"rust-v0.800.0","draft":true,"prerelease":false},{"tag_name":"rust-v0.199.0","draft":false,"prerelease":false},{"tag_name":"rust-v0.200.1","draft":false,"prerelease":false}]`), nil
	}))
	defer source.Close()
	value, err := source.Latest(context.Background())
	if err != nil || value != "0.200.1" || calls != 2 {
		t.Fatal(value, err, calls)
	}
}

func TestReleaseErrorsAreBoundedAndDoNotFollowRedirects(t *testing.T) {
	for _, test := range []struct {
		name     string
		response *http.Response
	}{
		{"rate limit", releaseResponse(429, `{"message":"synthetic private upstream detail"}`)},
		{"malformed", releaseResponse(200, `{"tag_name":`)},
		{"oversized", releaseResponse(200, `{"body":"`+strings.Repeat("x", 2<<20)+`","tag_name":"rust-v0.200.0"}`)},
		{"redirect", &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://other.example.test/releases"}}, Body: io.NopCloser(strings.NewReader(""))}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			source := NewReleases(transportFunc(func(*http.Request) (*http.Response, error) { calls++; return test.response, nil }))
			defer source.Close()
			if _, err := source.Latest(context.Background()); err == nil || strings.Contains(err.Error(), "synthetic private") {
				t.Fatal("unsafe release result", err)
			}
			if calls != 1 {
				t.Fatal("failed response caused an extra request", calls)
			}
		})
	}
}
