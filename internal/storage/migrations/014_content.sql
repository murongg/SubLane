ALTER TABLE tenants ADD COLUMN content_config BLOB NOT NULL DEFAULT X'' CHECK(length(content_config)<=131072);
ALTER TABLE request_records ADD COLUMN content_mode TEXT NOT NULL DEFAULT '' CHECK(content_mode IN ('','observe','block'));
ALTER TABLE request_records ADD COLUMN content_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE request_records ADD COLUMN content_rule_ids TEXT NOT NULL DEFAULT '[]' CHECK(length(content_rule_ids)<=4096 AND json_valid(content_rule_ids));
ALTER TABLE request_records ADD COLUMN content_check_failed INTEGER NOT NULL DEFAULT 0 CHECK(content_check_failed IN (0,1));
