ALTER TABLE worker_settings ADD COLUMN cpu_jobs bigint CHECK (cpu_jobs >= 0);
ALTER TABLE worker_settings ADD COLUMN gpu_jobs bigint CHECK (gpu_jobs >= 0);
ALTER TABLE worker_settings ADD CONSTRAINT worker_settings_encoder_pair
    CHECK ((cpu_jobs IS NULL AND gpu_jobs IS NULL) OR (cpu_jobs IS NOT NULL AND gpu_jobs IS NOT NULL));
