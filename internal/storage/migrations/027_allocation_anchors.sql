-- Rebuild without timestamp uniqueness; captured revision IDs remain immutable.
-- Published migrations must remain an unchanged prefix for backup validation.
PRAGMA legacy_alter_table=ON;
CREATE TABLE allocation_revisions_next (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 scheme_id INTEGER NOT NULL REFERENCES allocation_schemes(id),
 effective_at INTEGER NOT NULL,
 accounting_anchor_at INTEGER NOT NULL CHECK(accounting_anchor_at>=0),
 config TEXT NOT NULL CHECK(json_valid(config))
);
INSERT INTO allocation_revisions_next(id,scheme_id,effective_at,accounting_anchor_at,config)
SELECT id,scheme_id,effective_at,effective_at,config FROM allocation_revisions;
-- Deleted pending IDs must never be issued again as optimistic write conditions.
INSERT INTO sqlite_sequence(name,seq)
SELECT 'allocation_revisions_next',seq FROM sqlite_sequence
WHERE name='allocation_revisions' AND NOT EXISTS(SELECT 1 FROM sqlite_sequence WHERE name='allocation_revisions_next');
UPDATE sqlite_sequence SET seq=max(seq,coalesce((SELECT seq FROM sqlite_sequence WHERE name='allocation_revisions'),0))
WHERE name='allocation_revisions_next';
DROP TABLE allocation_revisions;
ALTER TABLE allocation_revisions_next RENAME TO allocation_revisions;
PRAGMA legacy_alter_table=OFF;
CREATE INDEX allocation_revisions_order ON allocation_revisions(scheme_id,effective_at,id);
