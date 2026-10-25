CREATE TABLE IF NOT EXISTS worker_settings (
    worker_name varchar(100) PRIMARY KEY NOT NULL,
    max_active_jobs bigint NOT NULL CHECK (max_active_jobs >= 0),
    updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
);
