ALTER TABLE jobs
    ADD COLUMN source_size BIGINT NOT NULL default 0;
ALTER TABLE jobs
    ADD COLUMN target_size BIGINT NOT NULL default 0;
ALTER TABLE jobs
    RENAME COLUMN destination_path TO target_path;