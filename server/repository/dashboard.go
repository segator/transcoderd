package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"transcoder/model"

	"github.com/lib/pq"
)

// A phase can be completed while the overall job is still active.
const dashboardJobsSQL = `WITH fleet AS (
 SELECT j.*, s.worker_name, s.notification_type AS phase, s.status, s.message,
        s.event_time, s.job_event_id,
        CASE WHEN s.notification_type='Job' AND s.status IN ('queued','completed','failed','canceled')
             THEN s.status ELSE 'active' END AS state
 FROM jobs j JOIN job_status s ON s.job_id=j.id
) `

func (s *SQLRepository) GetDashboard(ctx context.Context, hours int, now time.Time) (*model.Dashboard, error) {
	if hours != 24 && hours != 168 && hours != 720 {
		return nil, fmt.Errorf("dashboard hours must be 24, 168, or 720")
	}
	if s.tx != nil {
		return s.readDashboard(ctx, hours, now)
	}
	// Every panel sees the same committed state during a refresh.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	r := &SQLRepository{tx: &SQLTransaction{tx: tx}}
	d, err := r.readDashboard(ctx, hours, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *SQLRepository) readDashboard(ctx context.Context, hours int, now time.Time) (*model.Dashboard, error) {
	d := model.NewDashboard(now, hours)
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	since := now.UTC().Add(-time.Duration(hours) * time.Hour)
	valid := `state='completed' AND source_size>0 AND target_size>0`
	query := dashboardJobsSQL + `SELECT count(*),
 count(*) FILTER (WHERE state='queued'), count(*) FILTER (WHERE state='active'),
 count(*) FILTER (WHERE state='completed'), count(*) FILTER (WHERE state='failed'),
 count(*) FILTER (WHERE state='canceled'),
 COALESCE(sum(source_size) FILTER (WHERE ` + valid + `),0),
 COALESCE(sum(target_size) FILTER (WHERE ` + valid + `),0),
 count(*) FILTER (WHERE state='completed' AND event_time >= $1),
 count(*) FILTER (WHERE state='failed' AND event_time >= $1),
 COALESCE(avg(EXTRACT(EPOCH FROM (event_time-(SELECT max(e.event_time) FROM job_events e
   WHERE e.job_id=fleet.id AND e.notification_type='Job' AND e.status='started'
   AND e.job_event_id < fleet.job_event_id))))
   FILTER (WHERE state='completed' AND event_time >= $1),0)
 FROM fleet`
	v := &d.Summary
	err = db.QueryRowContext(ctx, query, since).Scan(&v.Total, &v.Queued, &v.Active, &v.Completed,
		&v.Failed, &v.Canceled, &v.SourceBytes, &v.TargetBytes, &v.CompletedInRange, &v.FailedInRange, &v.AverageSeconds)
	if err != nil {
		return nil, fmt.Errorf("dashboard summary: %w", err)
	}
	unit, step := "day", 24*time.Hour
	if hours == 24 {
		unit, step = "hour", time.Hour
	}
	for t := since.Truncate(step); !t.After(now); t = t.Add(step) {
		d.Timeline = append(d.Timeline, model.DashboardBucket{Time: t})
	}
	rows, err := db.QueryContext(ctx, dashboardJobsSQL+`SELECT date_trunc($2,event_time),
 count(*) FILTER (WHERE state='completed'), count(*) FILTER (WHERE state='failed'),
 COALESCE(sum(source_size) FILTER (WHERE `+valid+`),0),
 COALESCE(sum(target_size) FILTER (WHERE `+valid+`),0)
 FROM fleet WHERE event_time >= $1 AND state IN ('completed','failed') GROUP BY 1 ORDER BY 1`, since, unit)
	if err != nil {
		return nil, fmt.Errorf("dashboard timeline: %w", err)
	}
	for rows.Next() {
		var b model.DashboardBucket
		if err := rows.Scan(&b.Time, &b.Completed, &b.Failed, &b.SourceBytes, &b.TargetBytes); err != nil {
			rows.Close()
			return nil, err
		}
		b.Time = b.Time.UTC()
		for i := range d.Timeline {
			if d.Timeline[i].Time.Equal(b.Time) {
				d.Timeline[i] = b
				break
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := s.readDashboardWorkers(ctx, db, d, since); err != nil {
		return nil, err
	}
	if err := s.readDashboardJobs(ctx, db, d); err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, dashboardJobsSQL+`SELECT COALESCE((SELECT stream->>'codec_name'
 FROM jsonb_array_elements(COALESCE(target_probe::jsonb->'streams','[]'::jsonb)) stream
 WHERE stream->>'codec_type'='video' LIMIT 1),'unknown'), count(*), sum(source_size),sum(target_size)
 FROM fleet WHERE `+valid+` AND event_time >= $1 GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 12`, since)
	if err != nil {
		return nil, fmt.Errorf("dashboard codecs: %w", err)
	}
	for rows.Next() {
		var c model.DashboardCodec
		if err := rows.Scan(&c.Name, &c.Jobs, &c.SourceBytes, &c.TargetBytes); err != nil {
			rows.Close()
			return nil, err
		}
		d.Codecs = append(d.Codecs, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `SELECT e.job_id,j.source_path,e.worker_name,e.notification_type,e.status,
 COALESCE(e.message,''),e.event_time FROM job_events e JOIN jobs j ON j.id=e.job_id
 WHERE e.event_time >= $1 AND (e.notification_type='Job' OR e.status='failed')
 ORDER BY e.event_time DESC,e.job_id,e.job_event_id DESC LIMIT 30`, since)
	if err != nil {
		return nil, fmt.Errorf("dashboard activity: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var a model.DashboardActivity
		if err := rows.Scan(&a.JobID, &a.SourcePath, &a.Worker, &a.Phase, &a.Status, &a.Message, &a.Time); err != nil {
			return nil, err
		}
		d.Activity = append(d.Activity, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	d.Finalize()
	return d, nil
}

func (s *SQLRepository) readDashboardWorkers(ctx context.Context, db SQLDBOperations, d *model.Dashboard, since time.Time) error {
	rows, err := db.QueryContext(ctx, dashboardJobsSQL+`, names AS (
 SELECT name FROM workers UNION SELECT worker_name FROM worker_settings
 UNION SELECT worker_name FROM job_status WHERE worker_name<>''
), stats AS (
 SELECT worker_name, count(*) FILTER (WHERE state='active') active,
 count(*) FILTER (WHERE state='completed' AND event_time >= $1) completed,
 count(*) FILTER (WHERE state='failed' AND event_time >= $1) failed,
 COALESCE(sum(source_size) FILTER (WHERE state='completed' AND source_size>0 AND target_size>0 AND event_time >= $1),0) source_bytes,
 COALESCE(sum(target_size) FILTER (WHERE state='completed' AND source_size>0 AND target_size>0 AND event_time >= $1),0) target_bytes
 FROM fleet GROUP BY worker_name
)
 SELECT n.name,COALESCE(w.ip,''),w.last_seen,w.runtime,ws.max_active_jobs,ws.cpu_jobs,ws.gpu_jobs,
 COALESCE(st.active,0),COALESCE(st.completed,0),COALESCE(st.failed,0),
 COALESCE(st.source_bytes,0),COALESCE(st.target_bytes,0)
 FROM names n LEFT JOIN workers w ON w.name=n.name
 LEFT JOIN worker_settings ws ON ws.worker_name=n.name LEFT JOIN stats st ON st.worker_name=n.name
 ORDER BY n.name`, since)
	if err != nil {
		return fmt.Errorf("dashboard workers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var w model.DashboardWorker
		var raw []byte
		if err := rows.Scan(&w.Name, &w.IP, &w.LastSeen, &raw, &w.DesiredMax, &w.DesiredCPUJobs, &w.DesiredGPUJobs,
			&w.ActiveJobs, &w.Completed, &w.Failed, &w.SourceBytes, &w.TargetBytes); err != nil {
			return err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &w.Runtime); err != nil {
				return fmt.Errorf("decode worker runtime: %w", err)
			}
		}
		d.Workers = append(d.Workers, w)
	}
	return rows.Err()
}

func (s *SQLRepository) readDashboardJobs(ctx context.Context, db SQLDBOperations, d *model.Dashboard) error {
	rows, err := db.QueryContext(ctx, dashboardJobsSQL+`SELECT id,source_path,source_size,target_size,state,phase,
 worker_name,COALESCE(message,''),event_time FROM fleet
 ORDER BY CASE WHEN state='active' THEN 0 WHEN state='queued' THEN 1 ELSE 2 END,
 event_time DESC,id LIMIT 100`)
	if err != nil {
		return fmt.Errorf("dashboard jobs: %w", err)
	}
	indices := make(map[string]int)
	ids := make([]string, 0, 100)
	for rows.Next() {
		j := model.DashboardJob{Progress: []model.DashboardProgress{}}
		if err := rows.Scan(&j.ID, &j.SourcePath, &j.SourceBytes, &j.TargetBytes, &j.State, &j.Phase,
			&j.Worker, &j.Message, &j.UpdatedAt); err != nil {
			rows.Close()
			return err
		}
		indices[j.ID] = len(d.Jobs)
		ids = append(ids, j.ID)
		d.Jobs = append(d.Jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	// Match current assignment and phase to exclude abandoned retry progress.
	rows, err = db.QueryContext(ctx, `SELECT p.job_id,p.progress_id,p.notification_type,p.percent,p.eta,p.last_update
 FROM job_progress p JOIN job_status s ON s.job_id=p.job_id AND s.worker_name=p.worker_name
 AND s.notification_type=p.notification_type WHERE s.notification_type<>'Job'
 AND s.status='started' AND p.job_id=ANY($1) ORDER BY p.job_id,p.progress_id`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var p model.DashboardProgress
		if err := rows.Scan(&id, &p.ID, &p.Phase, &p.Percent, &p.ETASeconds, &p.UpdatedAt); err != nil {
			return err
		}
		if index, ok := indices[id]; ok {
			normalizeDashboardProgress(&p, d.GeneratedAt)
			d.Jobs[index].Progress = append(d.Jobs[index].Progress, p)
		}
	}
	return rows.Err()
}

func normalizeDashboardProgress(p *model.DashboardProgress, now time.Time) {
	if math.IsNaN(p.Percent) || math.IsInf(p.Percent, 0) {
		p.Percent = 0
	}
	p.Percent = math.Max(0, math.Min(100, p.Percent))
	if math.IsNaN(p.ETASeconds) || math.IsInf(p.ETASeconds, 0) || p.ETASeconds < 0 {
		p.ETASeconds = 0
	}
	p.Stale = now.Sub(p.UpdatedAt) > time.Minute
}
