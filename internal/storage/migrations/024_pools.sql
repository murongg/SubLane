-- Rebuild on the migration runner's pinned connection with foreign keys disabled.
-- Keep identifiers, memberships and bindings; only the old default restriction changes.
DROP VIEW effective_group_access;
CREATE TABLE account_groups_next (
 id INTEGER PRIMARY KEY,
 name TEXT NOT NULL UNIQUE COLLATE NOCASE,
 enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 restricted_models INTEGER NOT NULL DEFAULT 0 CHECK(restricted_models IN (0,1))
);
INSERT INTO account_groups_next SELECT id,name,enabled,created_at,updated_at,restricted_models FROM account_groups;
DROP TABLE account_groups;
ALTER TABLE account_groups_next RENAME TO account_groups;

-- Earlier migrations seed a pool even on a fresh installation. Only remove that
-- bootstrap row before setup; initialized instances retain their existing pool.
DELETE FROM account_groups WHERE id=1 AND NOT EXISTS(SELECT 1 FROM users)
 AND NOT EXISTS(SELECT 1 FROM accounts);

CREATE VIEW effective_group_access AS
SELECT u.id AS user_id,g.id AS group_id
FROM users u CROSS JOIN account_groups g
WHERE u.enabled=1 AND g.enabled=1 AND (
 u.role='admin' OR (NOT EXISTS(SELECT 1 FROM allocation_teams) AND EXISTS(SELECT 1 FROM group_members legacy WHERE legacy.group_id=g.id AND legacy.user_id=u.id)) OR EXISTS (
  SELECT 1 FROM allocation_team_members m JOIN allocation_teams t ON t.id=m.team_id
  WHERE m.user_id=u.id AND t.enabled=1 AND (
   EXISTS(SELECT 1 FROM allocation_schemes s WHERE s.team_id=t.id AND s.group_id=g.id AND s.enabled=1)
   OR (NOT EXISTS(SELECT 1 FROM allocation_schemes s WHERE s.group_id=g.id)
    AND EXISTS(SELECT 1 FROM allocation_team_groups tg WHERE tg.team_id=t.id AND tg.group_id=g.id))
  )
 )
);
