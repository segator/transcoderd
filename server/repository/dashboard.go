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

func (s *SQLRepository) GetDashboard(ctx context.Context, hours int, now time.Time, ranges ...model.DashboardRange) (*model.Dashboard, error) {
	period, err := model.ResolveDashboardRange(hours, now, ranges...)
	if err != nil {
		return nil, err
	}
	if s.tx != nil {
		return s.readDashboard(ctx, hours, now, period)
	}
	// Every panel sees the same committed state during a refresh.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	r := &SQLRepository{tx: &SQLTransaction{tx: tx}}
	d, err := r.readDashboard(ctx, hours, now, period)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *SQLRepository) readDashboard(ctx context.Context, hours int, now time.Time, period model.DashboardRange) (*model.Dashboard, error) {
	d := model.NewDashboard(now, hours)
	d.Range = period
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	since, until := period.Start, period.End
	valid := `state='completed' AND source_size>0 AND target_size>0`
	query := dashboardJobsSQL + `SELECT count(*),
 count(*) FILTER (WHERE state='queued'), count(*) FILTER (WHERE state='active'),
 count(*) FILTER (WHERE state='completed'), count(*) FILTER (WHERE state='failed'),
 count(*) FILTER (WHERE state='canceled'),
 COALESCE(sum(source_size) FILTER (WHERE ` + valid + `),0),
 COALESCE(sum(target_size) FILTER (WHERE ` + valid + `),0),
 count(*) FILTER (WHERE state='completed' AND event_time >= $1 AND event_time < $2),
 count(*) FILTER (WHERE state='failed' AND event_time >= $1 AND event_time < $2),
 COALESCE(avg(EXTRACT(EPOCH FROM (event_time-(SELECT max(e.event_time) FROM job_events e
   WHERE e.job_id=fleet.id AND e.notification_type='Job' AND e.status='started'
   AND e.job_event_id < fleet.job_event_id))))
   FILTER (WHERE state='completed' AND event_time >= $1 AND event_time < $2),0),
 COALESCE(sum(source_size) FILTER (WHERE ` + valid + ` AND event_time >= $1 AND event_time < $2),0),
 COALESCE(sum(target_size) FILTER (WHERE ` + valid + ` AND event_time >= $1 AND event_time < $2),0)
 FROM fleet`
	v := &d.Summary
	err = db.QueryRowContext(ctx, query, since, until).Scan(&v.Total, &v.Queued, &v.Active, &v.Completed,
		&v.Failed, &v.Canceled, &v.SourceBytes, &v.TargetBytes, &v.CompletedInRange, &v.FailedInRange, &v.AverageSeconds, &d.Period.SourceBytes, &d.Period.TargetBytes)
	if err != nil {
		return nil, fmt.Errorf("dashboard summary: %w", err)
	}
	unit, step := "day", period.Step()
	if step == time.Hour {
		unit = "hour"
	}
	for t := since.Truncate(step); t.Before(until); t = t.Add(step) {
		d.Timeline = append(d.Timeline, model.DashboardBucket{Time: t})
	}
	rows, err := db.QueryContext(ctx, dashboardJobsSQL+`SELECT date_trunc($2,event_time),
 count(*) FILTER (WHERE state='completed'), count(*) FILTER (WHERE state='failed'),
 COALESCE(sum(source_size) FILTER (WHERE `+valid+`),0),
 COALESCE(sum(target_size) FILTER (WHERE `+valid+`),0)
 FROM fleet WHERE event_time >= $1 AND event_time < $3 AND state IN ('completed','failed') GROUP BY 1 ORDER BY 1`, since, unit, until)
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
	if err := s.readDashboardWorkers(ctx, db, d, since, until); err != nil {
		return nil, err
	}
	if err := s.readDashboardJobs(ctx, db, d, model.DashboardJobsQuery{Limit: 100}); err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, dashboardJobsSQL+`SELECT COALESCE((SELECT stream->>'codec_name'
 FROM jsonb_array_elements(COALESCE(target_probe::jsonb->'streams','[]'::jsonb)) stream
 WHERE stream->>'codec_type'='video' LIMIT 1),'unknown'), count(*), sum(source_size),sum(target_size)
 FROM fleet WHERE `+valid+` AND event_time >= $1 AND event_time < $2 GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 12`, since, until)
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
 WHERE e.event_time >= $1 AND e.event_time < $2 AND (e.notification_type='Job' OR e.status='failed')
 ORDER BY e.event_time DESC,e.job_id,e.job_event_id DESC LIMIT 30`, since, until)
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

func (s *SQLRepository) readDashboardWorkers(ctx context.Context, db SQLDBOperations, d *model.Dashboard, since, until time.Time) error {
	rows, err := db.QueryContext(ctx, dashboardJobsSQL+`, stats AS (
 SELECT worker_name, count(*) FILTER (WHERE state='active') active,
 count(*) FILTER (WHERE state='completed' AND event_time >= $1 AND event_time < $3) completed,
 count(*) FILTER (WHERE state='failed' AND event_time >= $1 AND event_time < $3) failed,
 COALESCE(sum(source_size) FILTER (WHERE state='completed' AND source_size>0 AND target_size>0 AND event_time >= $1 AND event_time < $3),0) source_bytes,
 COALESCE(sum(target_size) FILTER (WHERE state='completed' AND source_size>0 AND target_size>0 AND event_time >= $1 AND event_time < $3),0) target_bytes
 FROM fleet GROUP BY worker_name
)
 SELECT w.name,COALESCE(w.ip,''),w.last_seen,w.runtime,ws.max_active_jobs,ws.cpu_jobs,ws.gpu_jobs,
 COALESCE(st.active,0),COALESCE(st.completed,0),COALESCE(st.failed,0),
 COALESCE(st.source_bytes,0),COALESCE(st.target_bytes,0),
 CASE WHEN wc.id IS NOT NULL THEN jsonb_build_object('id',wc.id,'action',wc.action,'state',wc.state,'error',wc.error,'jobId',COALESCE(wc.job_id::text,'')) END
 FROM workers w LEFT JOIN worker_controls wc ON wc.worker_name=w.name
 LEFT JOIN worker_settings ws ON ws.worker_name=w.name LEFT JOIN stats st ON st.worker_name=w.name
 WHERE w.last_seen >= $2 ORDER BY w.name`, since, d.GeneratedAt.Add(-30*24*time.Hour), until)
	if err != nil {
		return fmt.Errorf("dashboard workers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var w model.DashboardWorker
		var raw, control []byte
		if err := rows.Scan(&w.Name, &w.IP, &w.LastSeen, &raw, &w.DesiredMax, &w.DesiredCPUJobs, &w.DesiredGPUJobs,
			&w.ActiveJobs, &w.Completed, &w.Failed, &w.SourceBytes, &w.TargetBytes, &control); err != nil {
			return err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &w.Runtime); err != nil {
				return fmt.Errorf("decode worker runtime: %w", err)
			}
		}
		if len(control) > 0 {
			if err := json.Unmarshal(control, &w.Control); err != nil {
				return err
			}
		}
		d.Workers = append(d.Workers, w)
	}
	return rows.Err()
}

func (s *SQLRepository) readDashboardJobs(ctx context.Context, db SQLDBOperations, d *model.Dashboard, query model.DashboardJobsQuery) error {
	rows, err := db.QueryContext(ctx, dashboardJobsSQL+`SELECT id,source_path,source_size,target_size,state,phase,
 worker_name,COALESCE(message,''),event_time FROM fleet
 WHERE ($1='all' OR $1='' OR state=$1) AND ($2='' OR strpos(lower(source_path || ' ' || worker_name || ' ' || id::text),lower($2))>0)
 ORDER BY CASE WHEN state='active' THEN 0 WHEN state='queued' THEN 1 ELSE 2 END,
 event_time DESC,id LIMIT $3 OFFSET $4`, query.State, query.Search, query.Limit, query.Offset)
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

func (s *SQLRepository) GetDashboardJobs(ctx context.Context, query model.DashboardJobsQuery) (*model.DashboardJobPage, error) {
	if s.tx != nil {
		return s.readDashboardJobPage(ctx, query)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	r := &SQLRepository{tx: &SQLTransaction{tx: tx}}
	page, err := r.readDashboardJobPage(ctx, query)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return page, nil
}

func (s *SQLRepository) readDashboardJobPage(ctx context.Context, query model.DashboardJobsQuery) (*model.DashboardJobPage, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	page := &model.DashboardJobPage{Offset: query.Offset, Limit: query.Limit}
	err = db.QueryRowContext(ctx, dashboardJobsSQL+`SELECT count(*) FROM fleet
 WHERE ($1='all' OR $1='' OR state=$1) AND ($2='' OR strpos(lower(source_path || ' ' || worker_name || ' ' || id::text),lower($2))>0)`, query.State, query.Search).Scan(&page.Total)
	if err != nil {
		return nil, fmt.Errorf("dashboard job count: %w", err)
	}
	d := model.NewDashboard(time.Now().UTC(), 24)
	if err := s.readDashboardJobs(ctx, db, d, query); err != nil {
		return nil, err
	}
	page.Jobs = d.Jobs
	return page, nil
}
