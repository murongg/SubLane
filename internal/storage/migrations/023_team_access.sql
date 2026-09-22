-- Legacy individual grants remain stored for upgrade history but no longer grant access.
CREATE TABLE allocation_team_groups (
 team_id INTEGER NOT NULL REFERENCES allocation_teams(id),
 group_id INTEGER NOT NULL REFERENCES account_groups(id),
 PRIMARY KEY(team_id,group_id)
);

-- Every discovery and authorization path uses this view. Reserved pools only
-- inherit their owning scheme's team, even if another team granted them earlier.
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
