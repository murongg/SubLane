package gateway

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/upstream"
	"github.com/murongg/SubLane/internal/vault"
	"modernc.org/sqlite"
)

type measuredQuery struct {
	sql  string
	args []any
}

type queryMeter struct {
	mu      sync.Mutex
	queries []measuredQuery
}

func (m *queryMeter) record(query string, args []driver.NamedValue) {
	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries = append(m.queries, measuredQuery{query, values})
}

func (m *queryMeter) take() []measuredQuery {
	m.mu.Lock()
	defer m.mu.Unlock()
	queries := m.queries
	m.queries = nil
	return queries
}

type measuredDriver struct{ meter *queryMeter }

func (d measuredDriver) Open(name string) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &measuredConn{conn, d.meter}, nil
}

type measuredConn struct {
	driver.Conn
	meter *queryMeter
}

func (c *measuredConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.meter.record(query, args)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func (c *measuredConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.meter.record(query, args)
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func measuredGateway(t *testing.T, size int) (*Service, *queryMeter) {
	t.Helper()
	ctx := context.Background()
	base, _ := providerGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		return syntheticStream(), nil
	}))
	var sequence int
	var schema, path string
	if err := base.db.QueryRow("PRAGMA database_list").Scan(&sequence, &schema, &path); err != nil {
		t.Fatal(err)
	}
	meter := &queryMeter{}
	name := "measured-sqlite-" + rand.Text()
	sql.Register(name, measuredDriver{meter})
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	conn, err := sql.Open(name, u.String())
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	cipher, err := vault.Open(filepath.Join(filepath.Dir(path), "key"), false)
	if err != nil {
		t.Fatal(err)
	}
	s := New(ctx, conn, accounts.New(conn, cipher), base.provider)
	t.Cleanup(s.Close)
	if _, err := conn.Exec("DELETE FROM accounts WHERE provider!='codex'"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < size; i++ {
		a, err := s.accounts.Authorize(ctx, fmt.Sprintf("Synthetic %d", i), accounts.Credential{AccountID: fmt.Sprintf("synthetic-%d", i), AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.accounts.SaveCatalog(ctx, a.ID, 0, []string{"synthetic-model"}, time.Now().Unix(), upstream.CatalogSource("codex")); err != nil {
			t.Fatal(err)
		}
	}
	configureTestPool(t, conn)
	rows, err := s.accounts.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		raw, _ := json.Marshal(upstream.Usage{UpdatedAt: time.Now().Unix(), Limits: []upstream.UsageLimit{}})
		if _, err := s.queries.SaveAccountUsage(ctx, db.SaveAccountUsageParams{AccountID: row.ID, Snapshot: raw, UpdatedAt: time.Now().Unix(), Revision: 0}); err != nil {
			t.Fatal(err)
		}
	}
	meter.take()
	return s, meter
}
