-- Preserve the routing behavior of existing groups and their immutable key bindings.
ALTER TABLE account_groups ADD COLUMN routing_preference TEXT NOT NULL DEFAULT 'protocol'
    CHECK(routing_preference IN ('protocol','subscription_first','api_first'));
ALTER TABLE account_groups ADD COLUMN allow_api_fallback INTEGER NOT NULL DEFAULT 1
    CHECK(allow_api_fallback IN (0,1));

-- Readiness and alerts must apply the same explicit API fallback policy as admission.
CREATE VIEW routable_group_resources AS
SELECT ga.group_id,ga.account_id FROM group_accounts ga
JOIN accounts a ON a.id=ga.account_id
JOIN account_groups g ON g.id=ga.group_id
WHERE a.provider!='openai' OR g.routing_preference!='subscription_first' OR g.allow_api_fallback=1
OR NOT EXISTS(SELECT 1 FROM group_accounts selected JOIN accounts subscription ON subscription.id=selected.account_id
              WHERE selected.group_id=g.id AND subscription.provider!='openai');
