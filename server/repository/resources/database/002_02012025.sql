ALTER TABLE jobs
    ADD COLUMN source_size BIGINT NOT NULL;
ALTER TABLE jobs
    ADD COLUMN target_size BIGINT NOT NULL;
ALTER TABLE jobs
    RENAME COLUMN destination_path TO target_path;

UPDATE jobs SET source_size = 0, target_size = 0;