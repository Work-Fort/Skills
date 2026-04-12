-- QA seed data for the notifier service.
-- Each implementation step adds INSERT statements for the states it
-- introduces. This file is embedded into the binary via //go:build qa
-- and executed on startup against a freshly migrated database.
--
-- Note: goqite job messages use gob encoding and MUST be enqueued
-- programmatically via jobs.Create() in Go code. Do NOT insert into
-- the goqite table directly from SQL.
--
-- Step 3: notifications in pending state.
-- Step 4: notifications in all states (delivered, failed, not_sent).

-- Pending notifications: the worker will attempt delivery on startup.
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-001', 'alice@company.com', 0, 0, 3);

INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-002', 'bob@company.com', 0, 0, 3);

-- Pending notification to @example.com: will auto-fail on delivery.
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-003', 'charlie@example.com', 0, 0, 3);
