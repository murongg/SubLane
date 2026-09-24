package upstream

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const proxyLocationURL = "https://ipinfo.io/json"

var proxyProbeSlots = make(chan struct{}, 4)

type ProxyCheck struct {
	Reachable bool
	ExitIP    string
	Country   string
	Region    string
	City      string
	LatencyMS int64
	ErrorCode string
}

// CheckProxy sends one bounded request to a fixed destination through the
// selected proxy. It never retries through the host's default network route.
func CheckProxy(ctx context.Context, address string) (ProxyCheck, error) {
	return checkProxy(ctx, address, proxyLocationURL)
}

func checkProxy(parent context.Context, address, target string) (ProxyCheck, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	select {
	case proxyProbeSlots <- struct{}{}:
		defer func() { <-proxyProbeSlots }()
	case <-ctx.Done():
		return ProxyCheck{}, ctx.Err()
	}
	u, err := url.Parse(address)
	if err != nil || u == nil || u.Host == "" {
		return ProxyCheck{}, ErrInput
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(u)
	transport.ResponseHeaderTimeout = 7 * time.Second
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 9 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ProxyCheck{}, ErrInput
	}
	request.Header.Set("Accept", "application/json")
	started := time.Now()
	response, err := client.Do(request)
	result := ProxyCheck{LatencyMS: min(30000, max(0, time.Since(started).Milliseconds()))}
	if parent.Err() != nil {
		return ProxyCheck{}, parent.Err()
	}
	if err != nil {
		result.ErrorCode = "connection_failed"
		return result, nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusProxyAuthRequired {
			result.ErrorCode = "connection_failed"
		} else if response.StatusCode == http.StatusTooManyRequests {
			result.ErrorCode = "lookup_rate_limited"
		} else {
			result.ErrorCode = "lookup_unavailable"
		}
		return result, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	result.LatencyMS = min(30000, max(0, time.Since(started).Milliseconds()))
	if err != nil || len(body) > 8192 {
		result.ErrorCode = "invalid_response"
		return result, nil
	}
	var value struct {
		IP      string `json:"ip"`
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
	}
	if json.Unmarshal(body, &value) != nil || net.ParseIP(value.IP) == nil {
		result.ErrorCode = "invalid_response"
		return result, nil
	}
	result.Reachable = true
	result.ExitIP = value.IP
	result.Country = strings.ToUpper(cleanLocation(value.Country, 2))
	if result.Country != "" && (len(result.Country) != 2 || result.Country[0] < 'A' || result.Country[0] > 'Z' || result.Country[1] < 'A' || result.Country[1] > 'Z') {
		result.Country = ""
	}
	result.Region = cleanLocation(value.Region, 128)
	result.City = cleanLocation(value.City, 128)
	return result, nil
}

func cleanLocation(value string, limit int) string {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || len(value) > limit || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return ""
	}
	return value
}
