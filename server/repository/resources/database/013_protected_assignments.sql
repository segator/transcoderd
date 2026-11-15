-- Protected assignments outlive worker presence and fence older generations.
CREATE TABLE protected_assignments (
    job_id varchar(255) PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
    generation bigint NOT NULL CHECK (generation > 0),
    worker_name varchar(100) NOT NULL,
    key_id text NOT NULL,
    capability_hash bytea NOT NULL CHECK (octet_length(capability_hash) = 32),
    state text NOT NULL CHECK (state IN ('assigned', 'parked', 'running', 'publishing', 'completed', 'revoked')),
    lease_instance uuid,
    lease_expires_at timestamptz,
    CHECK ((lease_instance IS NULL) = (lease_expires_at IS NULL)),
    updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Empty results are persisted too: a retried request cannot later acquire a
-- different job. A subsequent poll uses a fresh request ID.
CREATE TABLE protected_job_requests (
    worker_name varchar(100) NOT NULL,
    request_id uuid NOT NULL,
    job_id varchar(255) REFERENCES jobs(id) ON DELETE RESTRICT,
    generation bigint,
    key_id text,
    event_id integer,
    source_name text,
    created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (worker_name, request_id),
    CHECK ((job_id IS NULL AND generation IS NULL AND key_id IS NULL AND event_id IS NULL AND source_name IS NULL)
        OR (job_id IS NOT NULL AND generation > 0 AND key_id IS NOT NULL AND event_id IS NOT NULL AND source_name IS NOT NULL))
);

CREATE TABLE protected_publications (
    job_id varchar(255) NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    generation bigint NOT NULL CHECK (generation > 0),
    staging_path text NOT NULL,
    checksum text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    state text NOT NULL CHECK (state IN ('staged', 'publishing', 'completed', 'abandoned')),
    final_path text,
    completion_event_id integer,
    completion_message text,
    delete_source_pending boolean NOT NULL DEFAULT false,
    cleanup_pending boolean NOT NULL DEFAULT true,
    PRIMARY KEY (job_id, generation)
);

-- Sequence numbers prevent delayed progress samples from moving a current
-- assignment backwards. Payload hashes make identical retries idempotent.
CREATE TABLE protected_progress (
    job_id varchar(255) NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    generation bigint NOT NULL CHECK (generation > 0),
    progress_id text NOT NULL,
    notification_type text NOT NULL,
    sequence bigint NOT NULL CHECK (sequence > 0),
    payload_hash bytea NOT NULL CHECK (octet_length(payload_hash) = 32),
    PRIMARY KEY (job_id, generation, progress_id, notification_type)
);
