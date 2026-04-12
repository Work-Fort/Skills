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

-- Step 4: notifications in terminal/retry states.

-- Delivered notification (status=2).
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-004', 'delivered@company.com', 2, 0, 3);

-- Failed notification (status=3) -- exhausted retries.
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-005', 'failed@company.com', 3, 3, 3);

-- Not_sent notification (status=4) with 1 retry used -- will auto-retry.
INSERT INTO notifications (id, email, status, retry_count, retry_limit)
VALUES ('ntf_seed-006', 'retry@company.com', 4, 1, 3);

-- Audit log entries for pre-seeded terminal-state notifications.
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-004', 'pending', 'sending', 'send');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-004', 'sending', 'delivered', 'delivered');

INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'pending', 'sending', 'send');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'sending', 'not_sent', 'soft_fail');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'not_sent', 'sending', 'retry');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'sending', 'not_sent', 'soft_fail');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'not_sent', 'sending', 'retry');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'sending', 'not_sent', 'soft_fail');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'not_sent', 'sending', 'retry');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-005', 'sending', 'failed', 'failed');

INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-006', 'pending', 'sending', 'send');
INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger)
VALUES ('notification', 'ntf_seed-006', 'sending', 'not_sent', 'soft_fail');
