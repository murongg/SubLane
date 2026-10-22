-- Existing bindings keep their current account as the continuity baseline.
-- Earlier account moves were not recorded and cannot be reconstructed.
ALTER TABLE account_affinity ADD COLUMN rebound INTEGER NOT NULL DEFAULT 0 CHECK(rebound IN (0,1));
