package repository

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"time"
	"transcoder/model"
)

var (
	ErrElementNotFound = fmt.Errorf("element not found")
)

type Repository interface {
	getConnection() (SQLDBOperations, error)
	Initialize(ctx context.Context) error
	PingServerUpdate(ctx context.Context, pingEventType model.PingEventType) error
	GetTimeoutJobs(ctx context.Context, timeout time.Duration) ([]*model.TaskEventType, error)
	GetJobs(ctx context.Context) (*[]model.Job, error)
	GetJobsByStatus(ctx context.Context, status model.NotificationStatus) (jobs []*model.Job, returnError error)
	GetJob(ctx context.Context, uuid string) (*model.Job, error)
	GetJobByPath(ctx context.Context, path string) (*model.Job, error)
	AddJob(ctx context.Context, video *model.Job) error
	UpdateJob(ctx context.Context, video *model.Job) error
	ProgressJob(ctx context.Context, progressJob *model.TaskProgressType) error
	DeleteProgressJob(ctx context.Context, progressId string, notificationType model.NotificationType) error
	AddNewTaskEvent(ctx context.Context, event *model.TaskEventType) error
	WithTransaction(ctx context.Context, transactionFunc func(ctx context.Context, tx Repository) error) error
	GetWorker(ctx context.Context, name string) (*model.Worker, error)
	RetrieveQueuedJob(ctx context.Context) (*model.Job, error)
	GetAllProgressJobs(ctx context.Context) ([]model.TaskProgressType, error)
}

type SQLDBOperations interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Prepare(query string) (*sql.Stmt, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type SQLTransaction struct {
	tx *sql.Tx
}

func (s *SQLTransaction) Exec(query string, args ...interface{}) (sql.Result, error) {
	log.Debugf("Exec: %s, args: %v", query, args)
	return s.tx.Exec(query, args...)

}

func (s *SQLTransaction) Prepare(query string) (*sql.Stmt, error) {
	log.Debugf("Prepare: %s", query)
	return s.tx.Prepare(query)
}

func (s *SQLTransaction) Query(query string, args ...interface{}) (*sql.Rows, error) {
	log.Debugf("Query: %s, args: %v", query, args)
	return s.tx.Query(query, args...)
}

func (s *SQLTransaction) QueryRow(query string, args ...interface{}) *sql.Row {
	log.Debugf("QueryRow: %s, args: %v", query, args)
	return s.tx.QueryRow(query, args...)
}

func (s *SQLTransaction) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	log.Debugf("QueryContext: %s, args: %v", query, args)
	return s.tx.QueryContext(ctx, query, args...)
}

func (s *SQLTransaction) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	log.Debugf("ExecContext: %s, args: %v", query, args)
	return s.tx.ExecContext(ctx, query, args...)
}

type SQLDatabase struct {
	db *sql.DB
}

func (s *SQLDatabase) Exec(query string, args ...interface{}) (sql.Result, error) {
	log.Debugf("Exec query: %s, args: %v", query, args)
	return s.db.Exec(query, args...)
}

func (s *SQLDatabase) Prepare(query string) (*sql.Stmt, error) {
	log.Debugf("Prepare: %s", query)
	return s.db.Prepare(query)
}

func (s *SQLDatabase) Query(query string, args ...interface{}) (*sql.Rows, error) {
	log.Debugf("Query: %s, args: %v", query, args)
	return s.db.Query(query, args...)
}

func (s *SQLDatabase) QueryRow(query string, args ...interface{}) *sql.Row {
	log.Debugf("QueryRow: %s, args: %v", query, args)
	return s.db.QueryRow(query, args...)
}

func (s *SQLDatabase) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	log.Debugf("QueryContext: %s, args: %v", query, args)
	return s.db.QueryContext(ctx, query, args...)
}

func (s *SQLDatabase) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	log.Debugf("ExecContext: %s, args: %v", query, args)
	return s.db.ExecContext(ctx, query, args...)
}

func (s *SQLDatabase) BeginTx(ctx context.Context, txOpt *sql.TxOptions) (*sql.Tx, error) {
	log.Debugf("BeginTx")
	tx, err := s.db.BeginTx(ctx, txOpt)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

type SQLRepository struct {
	db *SQLDatabase
	tx SQLDBOperations
}

type SQLServerConfig struct {
	Host     string `mapstructure:"host" envconfig:"DB_HOST"`
	Port     int    `mapstructure:"port" envconfig:"DB_PORT"`
	User     string `mapstructure:"user" envconfig:"DB_USER"`
	Password string `mapstructure:"password" envconfig:"DB_PASSWORD"`
	Scheme   string `mapstructure:"scheme" envconfig:"DB_SCHEME"`
	Driver   string `mapstructure:"driver" envconfig:"DB_DRIVER"`
}

func NewSQLRepository(config *SQLServerConfig) (*SQLRepository, error) {
	connectionString := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", config.Host, config.Port, config.User, config.Password, config.Scheme)
	db, err := sql.Open(config.Driver, connectionString)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetConnMaxLifetime(0)
	db.SetMaxIdleConns(5)
	if log.IsLevelEnabled(log.DebugLevel) {
		go func() {
			for {
				fmt.Printf("In use %d not use %d  open %d wait %d\n", db.Stats().Idle, db.Stats().InUse, db.Stats().OpenConnections, db.Stats().WaitCount)
				time.Sleep(time.Second * 5)
			}
		}()
	}

	return &SQLRepository{
		db: &SQLDatabase{db},
	}, nil

}

func (s *SQLRepository) Initialize(ctx context.Context) error {
	return s.prepareDatabase(ctx)
}

//go:embed resources/database/*.sql
var databaseSchemas embed.FS

func (s *SQLRepository) prepareDatabase(ctx context.Context) error {
	var currentVersion int
	txErr := s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		con, err := tx.getConnection()
		if err != nil {
			return err
		}

		// Ensure `schema_version` table exists
		const createTableSchemaVersion = `CREATE TABLE IF NOT EXISTS schema_version (version INT PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP );`

		_, err = con.ExecContext(ctx, createTableSchemaVersion)
		if err != nil {
			return fmt.Errorf("failed to ensure schema_version table: %w", err)
		}

		rows, err := con.QueryContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version;`)
		if err != nil {
			return fmt.Errorf("failed to fetch schema version: %w", err)
		}
		defer rows.Close()
		if rows.Next() {
			if err = rows.Scan(&currentVersion); err != nil {
				return fmt.Errorf("failed to fetch schema version: %w", err)
			}
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}

	dbFiles, err := databaseSchemas.ReadDir("resources/database")
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}
	for _, file := range dbFiles {
		if file.IsDir() {
			continue
		}
		var version int
		_, err = fmt.Sscanf(file.Name(), "%03d", &version)
		if err != nil || version <= currentVersion {
			continue
		}
		log.Infof("upgrade db Schema from %d --> %d", currentVersion, version)
		txErr = s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
			con, err := tx.getConnection()
			if err != nil {
				return err
			}

			content, err := databaseSchemas.ReadFile("resources/database/" + file.Name())
			if err != nil {
				return fmt.Errorf("failed to read migration file %s: %w", file.Name(), err)
			}

			_, err = con.ExecContext(ctx, string(content))
			if err != nil {
				return fmt.Errorf("failed to apply migration %s: %w", file.Name(), err)
			}

			// Record the applied migration
			_, err = con.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES ($1);`, version)
			if err != nil {
				return fmt.Errorf("failed to update schema version: %w", err)
			}
			currentVersion = version
			return nil
		})
		if txErr != nil {
			return txErr
		}
	}
	return nil
}

func (s *SQLRepository) getConnection() (SQLDBOperations, error) {
	if s.tx != nil {
		return s.tx, nil
	}
	return s.db, nil
}
func (s *SQLRepository) GetWorker(ctx context.Context, name string) (worker *model.Worker, err error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	worker, err = s.getWorker(ctx, db, name)
	return worker, err
}
func (s *SQLRepository) getWorker(ctx context.Context, db SQLDBOperations, name string) (*model.Worker, error) {
	rows, err := db.QueryContext(ctx, "select * from workers where name=$1", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	worker := model.Worker{}
	found := false
	if rows.Next() {
		err := rows.Scan(&worker.Name, &worker.Ip, &worker.QueueName, &worker.LastSeen)
		if err != nil {
			return nil, err
		}
		found = true
	}

	if !found {
		return nil, fmt.Errorf("%w, %s", ErrElementNotFound, name)
	}
	return &worker, err
}

func (s *SQLRepository) GetJob(ctx context.Context, uuid string) (video *model.Job, returnError error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	video, err = s.getJob(ctx, db, uuid)
	return video, err
}

func (s *SQLRepository) RetrieveQueuedJob(ctx context.Context) (video *model.Job, err error) {
	conn, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	return s.queuedJob(ctx, conn)
}

func (s *SQLRepository) GetJobsByStatus(ctx context.Context, status model.NotificationStatus) (jobs []*model.Job, returnError error) {
	conn, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	return s.getJobsByStatus(ctx, conn, status)
}

func (s *SQLRepository) getJobsByStatus(ctx context.Context, tx SQLDBOperations, statusFilter model.NotificationStatus) ([]*model.Job, error) {
	rows, err := tx.QueryContext(ctx, "SELECT v.id, v.source_path,v.source_size, v.target_path,v.target_size,vs.event_time, vs.status, vs.notification_type, vs.message FROM jobs v INNER JOIN job_status vs ON v.id = vs.job_id WHERE vs.status = $1;", statusFilter)
	if err != nil {
		return nil, err
	}
	var jobs []*model.Job

	for rows.Next() {
		job := model.Job{}
		err := rows.Scan(&job.Id, &job.SourcePath, &job.SourceSize, &job.TargetPath, &job.TargetSize, &job.LastUpdate, &job.Status, &job.StatusPhase, &job.StatusMessage)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, &job)
	}
	err = rows.Close()
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		taskEvents, err := s.getTaskEvents(ctx, tx, job.Id.String())
		if err != nil {
			return nil, err
		}
		job.Events = taskEvents
	}

	return jobs, nil
}

func (s *SQLRepository) GetTimeoutJobs(ctx context.Context, timeout time.Duration) (taskEvent []*model.TaskEventType, returnError error) {
	conn, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	taskEvent, err = s.getTimeoutJobs(ctx, conn, timeout)
	if err != nil {
		return nil, err
	}
	return taskEvent, nil
}

func (s *SQLRepository) getJob(ctx context.Context, tx SQLDBOperations, uuid string) (*model.Job, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, source_path, source_size, target_path, target_size FROM jobs WHERE id=$1", uuid)
	if err != nil {
		return nil, err
	}
	job := model.Job{}
	found := false
	if rows.Next() {
		err := rows.Scan(&job.Id, &job.SourcePath, &job.SourceSize, &job.TargetPath, &job.TargetSize)
		if err != nil {
			return nil, err
		}
		found = true
	}
	rows.Close()
	if !found {
		return nil, fmt.Errorf("%w, %s", ErrElementNotFound, uuid)
	}

	taskEvents, err := s.getTaskEvents(ctx, tx, job.Id.String())
	if err != nil {
		return nil, err
	}
	job.Events = taskEvents
	lastUpdate, status, statusPhase, statusMessage, _ := s.getJobStatus(ctx, tx, job.Id.String())

	if lastUpdate != nil {
		job.LastUpdate = lastUpdate
	}
	job.Status = status
	job.StatusPhase = statusPhase
	job.StatusMessage = statusMessage

	return &job, nil
}

func (s *SQLRepository) GetJobs(ctx context.Context) (jobs *[]model.Job, returnError error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	jobs, err = s.getJobs(ctx, db)
	return jobs, err
}

func (s *SQLRepository) getJobs(ctx context.Context, tx SQLDBOperations) (*[]model.Job, error) {
	query := `SELECT v.id, v.source_path,v.source_size, v.target_path,v.target_size, vs.event_time, vs.status, vs.notification_type, vs.message
    FROM jobs v
    INNER JOIN job_status vs ON v.id = vs.job_id`
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []model.Job{}
	for rows.Next() {
		job := model.Job{}
		err := rows.Scan(&job.Id, &job.SourcePath, &job.SourceSize, &job.TargetPath, &job.TargetSize, &job.LastUpdate, &job.Status, &job.StatusPhase, &job.StatusMessage)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return &jobs, nil
}

func (s *SQLRepository) getTaskEvents(ctx context.Context, tx SQLDBOperations, uuid string) ([]*model.TaskEventType, error) {
	rows, err := tx.QueryContext(ctx, "select * from job_events where job_id=$1 order by event_time asc", uuid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var taskEvents []*model.TaskEventType
	for rows.Next() {
		event := model.TaskEventType{}
		err := rows.Scan(&event.JobId, &event.EventID, &event.WorkerName, &event.EventTime, &event.NotificationType, &event.Status, &event.Message)
		if err != nil {
			return nil, err
		}
		taskEvents = append(taskEvents, &event)
	}
	return taskEvents, nil
}

func (s *SQLRepository) getJobStatus(ctx context.Context, tx SQLDBOperations, uuid string) (*time.Time, string, model.NotificationType, string, error) {
	var lastUpdate time.Time
	var status string
	var statusPhase model.NotificationType
	var message string

	rows, err := tx.QueryContext(ctx, "SELECT event_time, status, notification_type, message FROM job_status WHERE job_id=$1", uuid)
	if err != nil {
		return &lastUpdate, status, statusPhase, message, err
	}
	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&lastUpdate, &status, &statusPhase, &message)
		if err != nil {
			return nil, "", "", "", err
		}
	}
	return &lastUpdate, status, statusPhase, message, nil
}

func (s *SQLRepository) getJobByPath(ctx context.Context, tx SQLDBOperations, path string) (*model.Job, error) {
	rows, err := tx.QueryContext(ctx, "select * from jobs where source_path=$1", path)
	if err != nil {
		return nil, err
	}
	job := model.Job{}
	found := false
	if rows.Next() {
		err := rows.Scan(&job.Id, &job.SourcePath, &job.TargetPath, &job.SourceSize, &job.TargetSize)
		if err != nil {
			return nil, err
		}
		found = true
	}
	rows.Close()
	if !found {
		return nil, nil
	}

	taskEvents, err := s.getTaskEvents(ctx, tx, job.Id.String())
	if err != nil {
		return nil, err
	}
	job.Events = taskEvents
	lastUpdate, status, statusPhase, statusMessage, _ := s.getJobStatus(ctx, tx, job.Id.String())

	if lastUpdate != nil {
		job.LastUpdate = lastUpdate
	}
	job.Status = status
	job.StatusPhase = statusPhase
	job.StatusMessage = statusMessage
	return &job, nil
}

func (s *SQLRepository) GetJobByPath(ctx context.Context, path string) (video *model.Job, returnError error) {
	conn, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	return s.getJobByPath(ctx, conn, path)
}

func (s *SQLRepository) PingServerUpdate(ctx context.Context, pingEventType model.PingEventType) (returnError error) {
	conn, err := s.getConnection()
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "INSERT INTO workers (name, ip,last_seen ) VALUES ($1,$2,$3) ON CONFLICT (name) DO UPDATE SET ip = $2, last_seen=$3;", pingEventType.WorkerName, pingEventType.IP, time.Now())
	return err
}

func (s *SQLRepository) AddNewTaskEvent(ctx context.Context, event *model.TaskEventType) (returnError error) {
	conn, err := s.getConnection()
	if err != nil {
		return err
	}
	return s.addNewTaskEvent(ctx, conn, event)
}

func (s *SQLRepository) ProgressJob(ctx context.Context, progressJob *model.TaskProgressType) (returnError error) {
	conn, err := s.getConnection()
	if err != nil {
		return err
	}
	return s.insertOrUpdateProgressJob(ctx, conn, progressJob)
}

func (s *SQLRepository) DeleteProgressJob(ctx context.Context, progressId string, notificationType model.NotificationType) error {
	conn, err := s.getConnection()
	if err != nil {
		return err
	}
	return s.deleteProgressJob(ctx, conn, progressId, notificationType)
}

func (s *SQLRepository) GetAllProgressJobs(ctx context.Context) ([]model.TaskProgressType, error) {
	conn, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	return s.getAllProgressJobs(ctx, conn)
}

func (s *SQLRepository) addNewTaskEvent(ctx context.Context, tx SQLDBOperations, event *model.TaskEventType) error {
	rows, err := tx.QueryContext(ctx, "select COALESCE(max(job_event_id),-1) from job_events where job_id=$1", event.JobId.String())
	if err != nil {
		return err
	}

	videoEventID := -1
	if rows.Next() {
		err = rows.Scan(&videoEventID)
		if err != nil {
			return err
		}
	}
	rows.Close()

	if videoEventID+1 != event.EventID {
		return fmt.Errorf("EventID for %s not match,lastReceived %d, new %d", event.JobId.String(), videoEventID, event.EventID)
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO job_events (job_id, job_event_id,worker_name,event_time,notification_type,status,message)"+
		" VALUES ($1,$2,$3,$4,$5,$6,$7)", event.JobId.String(), event.EventID, event.WorkerName, time.Now(), event.NotificationType, event.Status, event.Message)
	return err
}
func (s *SQLRepository) AddJob(ctx context.Context, job *model.Job) error {
	conn, err := s.getConnection()
	if err != nil {
		return err
	}
	return s.addJob(ctx, conn, job)
}

func (s *SQLRepository) addJob(ctx context.Context, tx SQLDBOperations, job *model.Job) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO jobs (id, source_path,target_path,source_size,target_size)"+
		" VALUES ($1,$2,$3,$4,$5)", job.Id.String(), job.SourcePath, job.TargetPath, job.SourceSize, job.TargetSize)
	return err
}

func (s *SQLRepository) UpdateJob(ctx context.Context, job *model.Job) error {
	conn, err := s.getConnection()
	if err != nil {
		return err
	}
	return s.updateJob(ctx, conn, job)
}

func (s *SQLRepository) updateJob(ctx context.Context, tx SQLDBOperations, job *model.Job) error {
	_, err := tx.ExecContext(ctx, "UPDATE jobs SET source_path=$1, target_path=$2, source_size=$3, target_size=$4 WHERE id=$5", job.SourcePath, job.TargetPath, job.SourceSize, job.TargetSize, job.Id.String())
	return err
}

func (s *SQLRepository) getTimeoutJobs(ctx context.Context, tx SQLDBOperations, timeout time.Duration) ([]*model.TaskEventType, error) {
	timeoutDate := time.Now().Add(-timeout)

	rows, err := tx.QueryContext(ctx, "select v.* from job_events v right join "+
		"(select job_id,max(job_event_id) as job_event_id  from job_events where notification_type='Job'  group by job_id) as m "+
		"on m.job_id=v.job_id and m.job_event_id=v.job_event_id where status in ('assigned','started') and v.event_time < $1::timestamptz", timeoutDate)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var taskEvents []*model.TaskEventType
	for rows.Next() {
		event := model.TaskEventType{}
		err := rows.Scan(&event.JobId, &event.EventID, &event.WorkerName, &event.EventTime, &event.NotificationType, &event.Status, &event.Message)
		if err != nil {
			return nil, err
		}
		taskEvents = append(taskEvents, &event)
	}
	return taskEvents, nil
}

func (s *SQLRepository) WithTransaction(ctx context.Context, transactionFunc func(ctx context.Context, tx Repository) error) error {
	sqlTx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelDefault})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			err := sqlTx.Rollback()
			if err != nil {
				log.Error(p)
			}
			log.Panic(p)
		} else if err != nil {
			log.Debugf("Rollback SQLDBOperations %v", sqlTx)
			err = sqlTx.Rollback()
			if err != nil {
				log.Error(err)
			}
		} else {
			if err = sqlTx.Commit(); err != nil {
				log.Error(err)
			}
		}
	}()

	txRepository := SQLRepository{tx: &SQLTransaction{sqlTx}}
	return transactionFunc(ctx, &txRepository)
}

func (s *SQLRepository) queuedJob(ctx context.Context, tx SQLDBOperations) (*model.Job, error) {
	rows, err := tx.QueryContext(ctx, "select job_id, job_event_id from job_status where notification_type='Job' and status='queued' order by event_time asc limit 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		event := model.TaskEventType{}
		err := rows.Scan(&event.JobId, &event.EventID)
		if err != nil {
			return nil, err
		}
		return s.getJob(ctx, tx, event.JobId.String())
	}

	return nil, fmt.Errorf("%w, %s", ErrElementNotFound, "No jobs found")
}

func (s *SQLRepository) insertOrUpdateProgressJob(ctx context.Context, conn SQLDBOperations, jp *model.TaskProgressType) error {
	query := `
        INSERT INTO job_progress
        (progress_id, notification_type,job_id,worker_name, percent, eta)
        VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (progress_id,notification_type)
        DO UPDATE SET
            percent = $5,            
            eta = $6,
            last_update = now()`

	_, err := conn.ExecContext(ctx, query, jp.ProgressID, jp.NotificationType, jp.JobId, jp.WorkerName, jp.Percent, jp.ETA.Seconds())
	if err != nil {
		return err
	}
	return nil
}

func (s *SQLRepository) deleteProgressJob(ctx context.Context, conn SQLDBOperations, progressId string, notificationType model.NotificationType) error {
	_, err := conn.ExecContext(ctx, "DELETE FROM job_progress WHERE progress_id=$1 and notification_type=$2", progressId, notificationType)
	if err != nil {
		return err
	}
	return nil

}

func (s *SQLRepository) getAllProgressJobs(ctx context.Context, conn SQLDBOperations) ([]model.TaskProgressType, error) {
	rows, err := conn.QueryContext(ctx, "select progress_id, notification_type,job_id,worker_name, percent, eta, last_update from job_progress")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var progressJobs []model.TaskProgressType
	var etaSeconds float64
	if rows.Next() {
		progress := model.TaskProgressType{}
		err = rows.Scan(&progress.ProgressID, &progress.NotificationType, &progress.JobId, &progress.WorkerName, &progress.Percent, &etaSeconds, &progress.EventTime)
		if err != nil {
			return nil, err
		}
		progress.ETA = time.Duration(etaSeconds) * time.Second
		progressJobs = append(progressJobs, progress)
	}
	return progressJobs, nil
}
