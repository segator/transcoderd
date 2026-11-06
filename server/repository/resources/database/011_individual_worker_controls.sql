ALTER TABLE worker_controls ADD COLUMN job_id uuid;
ALTER TABLE worker_controls DROP CONSTRAINT worker_controls_action_check;
ALTER TABLE worker_controls ADD CONSTRAINT worker_controls_action_check CHECK (action IN ('pause-encodes','resume-encodes','stop-after-current','resume-queue','pause-job','resume-job'));
ALTER TABLE worker_controls ADD CONSTRAINT worker_controls_job_check CHECK (action NOT IN ('pause-job','resume-job') OR job_id IS NOT NULL);
