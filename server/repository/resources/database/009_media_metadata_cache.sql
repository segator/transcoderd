CREATE TABLE IF NOT EXISTS media_metadata_cache (
    root_key varchar(64) NOT NULL,
    relative_path text NOT NULL,
    source_size bigint NOT NULL,
    modified_ns bigint NOT NULL,
    probe jsonb NOT NULL,
    updated_at timestamp NOT NULL DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    PRIMARY KEY (root_key, relative_path)
);

CREATE INDEX IF NOT EXISTS jobs_media_source_path ON jobs (source_path);
CREATE INDEX IF NOT EXISTS jobs_media_target_path ON jobs (target_path);
