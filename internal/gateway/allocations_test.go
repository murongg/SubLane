package gateway

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/murongg/SubLane/internal/allocations"
	"github.com/murongg/SubLane/internal/groups"
)

func TestAllocationKeyUsesOneModeAndSharedLedger(t *testing.T) {
	ctx := context.Background()
	s, _ := codexGateway(t, transportFunc(func(*http.Request) (*http.Response, error) { return syntheticStream(), nil }))
	user := runtimeMember(t, s)
	ids, err := s.queries.ListGroupAccounts(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := groups.New(s.db).Save(ctx, 0, groups.Input{Name: "Synthetic pool", Enabled: true, AccountIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = groups.New(s.db).Save(ctx, 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	manager := allocations.New(s.db)
	team, err := manager.SaveTeam(ctx, 0, allocations.TeamInput{Name: "Synthetic team", Enabled: true, MemberIDs: []int64{user}})
	if err != nil {
		t.Fatal(err)
	}
	scheme, err := manager.SaveScheme(ctx, 0, allocations.SchemeInput{Name: "Synthetic amount", TeamID: team.ID, GroupID: pool.ID, Enabled: true, Config: allocations.Config{Mode: "amount", Period: "day", Members: []allocations.Share{{UserID: user, Limit: 16}}, Rates: []allocations.Rate{{Model: "synthetic-model", Input: 2000000, Cached: 1000000, Output: 6000000}}}})
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic stream has 3 input (2 cached) and 2 output = 16 micro-USD.
	budgetRule(t, s, user, 0, "", "day", 1)
	// Bind synthetic hashed key rows through the same durable relation as personal keys.
	result, err := s.db.Exec("INSERT INTO api_keys(user_id,group_id,name,prefix,token_hash,created_at) VALUES(?,2,'synthetic','sl_fake',randomblob(32),1)", user)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := result.LastInsertId()
	if _, err = s.db.Exec("INSERT INTO allocation_keys(key_id,scheme_id) VALUES(?,?)", key, scheme.ID); err != nil {
		t.Fatal(err)
	}
	callctx := WithRequestIdentity(ctx, key, "http")
	x, err := s.Open(callctx, user, pool.ID, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses)
	if err != nil {
		t.Fatal(err)
	}
	budgetFinish(t, x)
	if _, err = s.Open(WithRequestIdentity(ctx, key, "websocket"), user, pool.ID, []byte(`{"model":"codex/synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, allocations.ErrQuota) {
		t.Fatal("scheme did not enforce amount or stacked legacy token limit", err)
	}
	if _, err = s.Open(ctx, user, pool.ID, []byte(`{"model":"synthetic-model","input":[]}`), nil, Responses); !errors.Is(err, allocations.ErrUnavailable) {
		t.Fatal("unbound key bypassed scheme", err)
	}
}
