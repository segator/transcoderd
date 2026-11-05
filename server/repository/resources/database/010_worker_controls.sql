ALTER TABLE worker_settings ADD COLUMN resume_max_active_jobs bigint CHECK (resume_max_active_jobs > 0);
ALTER TABLE worker_settings ADD COLUMN resume_cpu_jobs bigint CHECK (resume_cpu_jobs >= 0);
ALTER TABLE worker_settings ADD COLUMN resume_gpu_jobs bigint CHECK (resume_gpu_jobs >= 0);
UPDATE worker_settings SET resume_max_active_jobs=max_active_jobs, resume_cpu_jobs=cpu_jobs, resume_gpu_jobs=gpu_jobs WHERE max_active_jobs>0;
CREATE TABLE worker_controls (
 worker_name varchar(100) PRIMARY KEY NOT NULL,
 id uuid NOT NULL,
 action text NOT NULL CHECK (action IN ('pause-encodes','resume-encodes','stop-after-current','resume-queue')),
 state text NOT NULL CHECK (state IN ('pending','applied','failed')),
 error text NOT NULL DEFAULT '',
 cpu_jobs bigint,
 gpu_jobs bigint,
 max_active_jobs bigint
);
