ALTER TABLE workers ADD COLUMN IF NOT EXISTS runtime jsonb;
ALTER TABLE job_progress ALTER COLUMN last_update SET DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC');
CREATE INDEX IF NOT EXISTS job_events_dashboard_time ON job_events (event_time DESC);
CREATE INDEX IF NOT EXISTS job_status_dashboard_time ON job_status (event_time DESC);
