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

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/timezone"
)

func TestPersonalKeyStatisticsFilterEveryViewAndKeepRevokedUsage(t *testing.T) {
	f := newForwardFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n")
	})
	ctx := context.Background()
	second, err := f.keys.CreateInGroup(ctx, f.userID, 1, "Synthetic second client")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := f.keys.CreateInGroup(ctx, 1, 1, "Synthetic owner client")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{f.secret, f.secret, second.Secret, owner.Secret} {
		r, err := http.NewRequest("POST", f.server.URL+"/v1/responses", strings.NewReader(`{"model":"synthetic-model","input":"synthetic"}`))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("synthetic inference failed", response.StatusCode)
		}
	}
	if _, err := f.keys.Revoke(ctx, f.userID, second.Key.ID); err != nil {
		t.Fatal(err)
	}
	login, err := f.identity.Login(ctx, "member-test", "member pass 42")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: login.Token}
	for _, test := range []struct {
		keyID, requests int64
	}{{f.keyID, 2}, {second.Key.ID, 1}, {0, 3}} {
		suffix := ""
		if test.keyID != 0 {
			suffix = fmt.Sprintf("&key_id=%d", test.keyID)
		}
		response := request(f.server.Config.Handler, "GET", "/api/me/usage?days=7"+suffix, "", nil, cookie)
		var page gateway.Statistics
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatal("key statistics unavailable", response.Code, response.Body.String())
		}
		if page.Totals.Requests != test.requests || page.Totals.InputTokens != 3*test.requests || page.Totals.OutputTokens != 2*test.requests || page.Totals.CachedTokens != test.requests {
			t.Fatalf("key %d totals escaped filter: %+v", test.keyID, page.Totals)
		}
		for _, rows := range [][]gateway.Statistic{page.Days, page.Models, page.Groups} {
			var requests, input int64
			for _, row := range rows {
				requests += row.Requests
				input += row.InputTokens
			}
			if requests != test.requests || input != 3*test.requests {
				t.Fatal("breakdown escaped key filter", rows)
			}
		}
		var requests, input int64
		for _, cell := range page.Activity.Cells {
			requests += cell.Requests
			input += cell.InputTokens
		}
		if requests != test.requests || input != 3*test.requests {
			t.Fatal("heatmap escaped key filter", requests, input)
		}
	}
	for _, test := range []struct {
		suffix string
		status int
	}{{"-1", 400}, {"0", 400}, {"invalid", 400}, {"9223372036854775808", 400}, {"%zz", 400}, {"1;ignored", 400}, {"1&key_id=2", 400}, {"", 400}, {fmt.Sprint(owner.Key.ID), 404}, {"999999", 404}} {
		response := request(f.server.Config.Handler, "GET", "/api/me/usage?days=7&key_id="+test.suffix, "", nil, cookie)
		if response.Code != test.status {
			t.Fatal("key filter validation/ownership", test.suffix, response.Code, response.Body.String())
		}
	}
	ownerLogin, err := f.identity.Login(ctx, "owner-test", "owner pass 42")
	if err != nil {
		t.Fatal(err)
	}
	for _, filter := range []string{"1", "%zz", "1;ignored"} {
		response := request(f.server.Config.Handler, "GET", "/api/usage?key_id="+filter, "", nil, &http.Cookie{Name: sessionCookie, Value: ownerLogin.Token})
		if response.Code != 400 {
			t.Fatal("team statistics accepted unsupported key filter", filter, response.Code)
		}
	}
}

func TestPersonalUsageDateRangeAndKeyExport(t *testing.T) {
	for _, zone := range []string{"UTC", "Asia/Kathmandu"} {
		t.Run(zone, func(t *testing.T) {
			f := newForwardFixture(t, func(http.ResponseWriter, *http.Request) {
				t.Error("usage report contacted upstream")
			})
			ctx := context.Background()
			zones, err := timezone.New(ctx, f.connection)
			if err != nil {
				t.Fatal(err)
			}
			actor := audit.WithActor(ctx, audit.Actor{TenantID: 1, ID: 1, Username: "owner-test", Role: "admin", Source: "user"})
			if _, err := zones.Save(actor, zone); err != nil {
				t.Fatal(err)
			}
			f.forwarding.SetTimeZone(zones)
			location := zones.Location()
			local := time.Now().In(location)
			today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
			day := today.AddDate(0, 0, -2)
			end := day.AddDate(0, 0, 1)
			for _, setting := range []string{"usage.daily.started_at", "usage.hourly.started_at", "usage.minutes.started_at", "usage.keys.started_at"} {
				if _, err := f.connection.Exec("UPDATE settings SET value=? WHERE key=?", day.Add(-time.Hour).Unix(), setting); err != nil {
					t.Fatal(err)
				}
			}
			second, err := f.keys.CreateInGroup(ctx, f.userID, 1, "Synthetic client")
			if err != nil {
				t.Fatal(err)
			}
			foreign, err := f.keys.CreateInGroup(ctx, 1, 1, "Synthetic foreign client")
			if err != nil {
				t.Fatal(err)
			}
			noUsage, err := f.keys.CreateInGroup(ctx, f.userID, 1, "Synthetic idle client")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.keys.Revoke(ctx, f.userID, second.Key.ID); err != nil {
				t.Fatal(err)
			}
			for _, row := range []struct {
				at                      time.Time
				user, key, calls, input int64
			}{
				{day.Add(-time.Minute), f.userID, f.keyID, 8, 0},
				{day, f.userID, f.keyID, 2, 5},
				{end.Add(-time.Minute), f.userID, second.Key.ID, 1, 0},
				{day, f.userID, 0, 4, 0},
				{day, 1, foreign.Key.ID, 8, 0},
				{end, f.userID, f.keyID, 8, 0},
			} {
				reported := 0
				if row.input > 0 {
					reported = 1
				}
				if zone == "UTC" {
					if _, err := f.connection.Exec(`INSERT INTO usage_daily(day,user_id,group_id,provider,model,key_id,requests,input_tokens,input_reported) VALUES(?,?,1,'codex','synthetic-model',?,?,?,?)`, row.at.UTC().Truncate(24*time.Hour).Unix(), row.user, row.key, row.calls, row.input, reported); err != nil {
						t.Fatal(err)
					}
				} else if _, err := f.connection.Exec(`INSERT INTO usage_minutes(minute,user_id,group_id,provider,model,key_id,requests,input_tokens,input_reported,tenant_id) VALUES(?,?,1,'codex','synthetic-model',?,?,?,?,1)`, row.at.Unix(), row.user, row.key, row.calls, row.input, reported); err != nil {
					t.Fatal(err)
				}
			}
			login, err := f.identity.Login(ctx, "member-test", "member pass 42")
			if err != nil {
				t.Fatal(err)
			}
			owner, err := f.identity.Login(ctx, "owner-test", "owner pass 42")
			if err != nil {
				t.Fatal(err)
			}
			cookie := &http.Cookie{Name: sessionCookie, Value: login.Token}
			date := day.Format(time.DateOnly)
			rangeQuery := "start_date=" + date + "&end_date=" + date
			read := func(query string) (gateway.Statistics, []gateway.Statistic) {
				t.Helper()
				response := request(f.server.Config.Handler, "GET", "/api/me/usage?"+query, "", nil, cookie)
				var body struct {
					gateway.Statistics
					Keys []gateway.Statistic `json:"keys"`
				}
				if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &body) != nil {
					t.Fatal("usage report unavailable", response.Code, response.Body.String())
				}
				return body.Statistics, body.Keys
			}
			page, keys := read(rangeQuery)
			if page.TimeZone != zone || page.FromDay != day.Unix() || page.ToDay != end.Unix() || page.Totals.Requests != 7 || len(page.Days) != 1 || page.Days[0].Name != date || page.Days[0].Requests != 7 || len(keys) != 0 {
				t.Fatalf("incorrect single-day report: bounds=%d..%d, totals=%+v, days=%+v, keys=%+v", page.FromDay, page.ToDay, page.Totals, page.Days, keys)
			}
			exported, keys := read(rangeQuery + "&include_keys=1")
			byID := make(map[string]gateway.Statistic, len(keys))
			for _, key := range keys {
				byID[key.ID] = key
			}
			first := byID[fmt.Sprint(f.keyID)]
			archived := byID[fmt.Sprint(second.Key.ID)]
			if exported.Totals != page.Totals || len(keys) != 3 ||
				first.Name != "Synthetic client" || first.Requests != 2 || first.InputTokens != 5 || first.InputReported != 1 ||
				archived.Name != first.Name || archived.Requests != 1 || archived.InputReported != 0 || byID["0"].Requests != 4 {
				t.Fatalf("incorrect key export: %+v", keys)
			}
			selected, keys := read(rangeQuery + fmt.Sprintf("&include_keys=1&key_id=%d", second.Key.ID))
			if selected.Totals.Requests != 1 || len(keys) != 1 || keys[0] != archived {
				t.Fatalf("selected export did not retain the revoked key: %+v", keys)
			}
			empty, keys := read(rangeQuery + fmt.Sprintf("&include_keys=1&key_id=%d", noUsage.Key.ID))
			if empty.Totals.Requests != 0 || len(keys) != 1 || keys[0].Name != noUsage.Key.Name || keys[0].Requests != 0 {
				t.Fatalf("selected export lost an idle key: %+v", keys)
			}
			for _, query := range []string{
				"start_date=" + date,
				"end_date=" + date,
				rangeQuery + "&days=7",
				rangeQuery + "&start_date=" + date,
				"start_date=" + date + "&end_date=2030-02-30",
				"start_date=" + end.Format(time.DateOnly) + "&end_date=" + date,
				"start_date=" + today.AddDate(0, 0, -90).Format(time.DateOnly) + "&end_date=" + date,
				"start_date=" + date + "&end_date=" + today.AddDate(0, 0, 1).Format(time.DateOnly),
				rangeQuery + "&include_keys=0",
			} {
				response := request(f.server.Config.Handler, "GET", "/api/me/usage?"+query, "", nil, cookie)
				if response.Code != 400 {
					t.Fatalf("invalid report query %q returned %d", query, response.Code)
				}
			}
			for _, query := range []string{rangeQuery, "include_keys=1"} {
				response := request(f.server.Config.Handler, "GET", "/api/usage?"+query, "", nil, &http.Cookie{Name: sessionCookie, Value: owner.Token})
				if response.Code != 400 {
					t.Fatalf("team usage accepted personal report query %q", query)
				}
			}
		})
	}
}
