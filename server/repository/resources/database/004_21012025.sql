-- Set workers as unlogged for extra performance
ALTER TABLE workers SET UNLOGGED;

-- to remove event_type we need to recreate funcs and triggers that updates job_status table
DROP TRIGGER IF EXISTS event_insert_job_status_update ON job_events;


DROP FUNCTION IF EXISTS fn_trigger_job_status_update();

DROP FUNCTION IF EXISTS fn_job_status_update(varchar, integer, varchar, timestamp, varchar, varchar, varchar, text);

ALTER TABLE job_events DROP COLUMN event_type;

ALTER TABLE job_status drop column event_type;



CREATE OR REPLACE FUNCTION fn_job_status_update(
    p_job_id varchar,
    p_job_event_id integer,
    p_worker_name varchar,
    p_event_time timestamp,
    p_notification_type varchar,
    p_status varchar,
    p_message text
) RETURNS VOID SECURITY DEFINER LANGUAGE plpgsql AS $$
DECLARE
    p_video_path varchar;
BEGIN
    SELECT
        v.source_path INTO p_video_path
    FROM
        jobs v
    WHERE
        v.id = p_job_id;

    INSERT INTO job_status (
        job_id,
        job_event_id,
        video_path,
        worker_name,
        event_time,
        notification_type,
        status,
        message
    )
    VALUES (
               p_job_id,
               p_job_event_id,
               p_video_path,
               p_worker_name,
               p_event_time,
               p_notification_type,
               p_status,
               p_message
           )
    ON CONFLICT ON CONSTRAINT job_status_pkey DO
        UPDATE
        SET
            job_event_id = p_job_event_id,
            video_path = p_video_path,
            worker_name = p_worker_name,
            event_time = p_event_time,
            notification_type = p_notification_type,
            status = p_status,
            message = p_message;
END;
$$;

CREATE OR REPLACE FUNCTION fn_trigger_job_status_update() RETURNS TRIGGER SECURITY DEFINER LANGUAGE plpgsql AS $$
BEGIN
    PERFORM fn_job_status_update(
            NEW.job_id,
            NEW.job_event_id,
            NEW.worker_name,
            NEW.event_time,
            NEW.notification_type,
            NEW.status,
            NEW.message
            );

    RETURN NEW;
END;
$$;

CREATE TRIGGER event_insert_job_status_update
    AFTER INSERT
    ON job_events
    FOR EACH ROW
EXECUTE PROCEDURE fn_trigger_job_status_update();


CREATE UNLOGGED TABLE  IF NOT EXISTS job_progress (
                                                      progress_id varchar(255) NOT NULL,
                                                      notification_type varchar(255) NOT NULL,
                                                      job_id varchar(255) NOT NULL,
                                                      worker_name varchar(255) NOT NULL,
                                                      percent real NOT NULL,
                                                      eta timestamp NOT NULL,
                                                      last_update timestamp NOT NULL DEFAULT NOW(),
                                                      PRIMARY KEY (progress_id,notification_type),
                                                      FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);



