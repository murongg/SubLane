CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id INTEGER NOT NULL,
    actor_name TEXT NOT NULL,
    actor_role TEXT NOT NULL,
    source TEXT NOT NULL CHECK(source IN ('user','local')),
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK(outcome IN ('success','failure')),
    http_status INTEGER,
    created_at INTEGER NOT NULL
);
CREATE INDEX audit_events_created ON audit_events(created_at);
