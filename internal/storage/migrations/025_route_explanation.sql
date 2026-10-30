ALTER TABLE request_records ADD COLUMN route_explanation TEXT NOT NULL DEFAULT 'null'
CHECK(length(route_explanation)<=1024 AND json_valid(route_explanation));
