-- QA seed data for the notifier service.
-- Each implementation step adds INSERT statements for the states it
-- introduces. This file is embedded into the binary via //go:build qa
-- and executed on startup against a freshly migrated database.
--
-- Step 1: placeholder (no tables exist yet).
-- Step 3: notifications in pending state + enqueued jobs.
-- Step 4: notifications in all states (delivered, failed, not_sent).
SELECT 1;
