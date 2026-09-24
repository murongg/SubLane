package storage

import (
	"context"
	"database/sql"
	"errors"
)

const formerProxyChecksMigration = "004_proxy_checks.sql"

// These columns were briefly introduced by a separate prerelease migration.
// Keep this upgrade path while databases and archives with that history exist.
const formerProxyChecksSQL = `ALTER TABLE proxies ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE proxies ADD COLUMN checked_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE proxies ADD COLUMN reachable INTEGER NOT NULL DEFAULT 0 CHECK(reachable IN (0,1));
ALTER TABLE proxies ADD COLUMN exit_ip TEXT NOT NULL DEFAULT '' CHECK(length(exit_ip) <= 45);
ALTER TABLE proxies ADD COLUMN country TEXT NOT NULL DEFAULT '' CHECK(length(country) <= 2);
ALTER TABLE proxies ADD COLUMN region TEXT NOT NULL DEFAULT '' CHECK(length(region) <= 128);
ALTER TABLE proxies ADD COLUMN city TEXT NOT NULL DEFAULT '' CHECK(length(city) <= 128);
ALTER TABLE proxies ADD COLUMN latency_ms INTEGER NOT NULL DEFAULT 0 CHECK(latency_ms BETWEEN 0 AND 30000);
ALTER TABLE proxies ADD COLUMN check_error TEXT NOT NULL DEFAULT '' CHECK(length(check_error) <= 32);`

type proxyColumnQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func verifyProxyCheckColumns(ctx context.Context, q proxyColumnQueryer) error {
	rows, err := q.QueryContext(ctx, `SELECT revision,checked_at,reachable,exit_ip,country,region,city,latency_ms,check_error FROM proxies LIMIT 0`)
	if err != nil {
		return err
	}
	defer rows.Close()
	return rows.Err()
}

func reconcileProxyMigrations(ctx context.Context, connection *sql.DB) error {
	var hasProxy, hasSeparateChecks bool
	if err := connection.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM schema_migrations WHERE name='003_proxies.sql'),
 EXISTS(SELECT 1 FROM schema_migrations WHERE name='004_proxy_checks.sql')`).Scan(&hasProxy, &hasSeparateChecks); err != nil {
		return err
	}
	if !hasProxy {
		if hasSeparateChecks {
			return errors.New("proxy check migration exists without proxy schema")
		}
		return nil
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var hasRevision bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pragma_table_info('proxies') WHERE name='revision')`).Scan(&hasRevision); err != nil {
		return err
	}
	if !hasRevision {
		if hasSeparateChecks {
			return errors.New("recorded proxy check migration has no check columns")
		}
		if _, err := tx.ExecContext(ctx, formerProxyChecksSQL); err != nil {
			return err
		}
	}
	if err := verifyProxyCheckColumns(ctx, tx); err != nil {
		return err
	}
	if hasSeparateChecks {
		if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE name='004_proxy_checks.sql'`); err != nil {
			return err
		}
	}
	return tx.Commit()
}
