package repository

import (
	"context"
	"database/sql"
	"embed"
	_ "embed"
	"fmt"
	_ "github.com/lib/pq"
	"time"
	"transcoder/model"
)

var (
	ErrElementNotFound = fmt.Errorf("element not found")
)

type Repository interface {
	getConnection(ctx context.Context) (Transaction, error)
	Initialize(ctx context.Context) error
	ProcessEvent(ctx context.Context, event *model.TaskEvent) error
	PingServerUpdate(ctx context.Context, name string, ip string) error
	GetTimeoutJobs(ctx context.Context, timeout time.Duration) ([]*model.TaskEvent, error)
	GetJobs(ctx context.Context) (*[]model.Job, error)
	GetJobsByStatus(ctx context.Context, status model.NotificationStatus) (jobs []*model.Job, returnError error)
	GetJob(ctx context.Context, uuid string) (*model.Job, error)
	GetJobByPath(ctx context.Context, path string) (*model.Job, error)
	AddJob(ctx context.Context, video *model.Job) error
	UpdateJob(ctx context.Context, video *model.Job) error
	AddNewTaskEvent(ctx context.Context, event *model.TaskEvent) error
	WithTransaction(ctx context.Context, transactionFunc func(ctx context.Context, tx Repository) error) error
	GetWorker(ctx context.Context, name string) (*model.Worker, error)
	RetrieveQueuedJob(ctx context.Context) (*model.Job, error)
}

type Transaction interface {
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

func (S *SQLTransaction) Exec(query string, args ...interface{}) (sql.Result, error) {
	return S.tx.Exec(query, args...)

}

func (S *SQLTransaction) Prepare(query string) (*sql.Stmt, error) {
	return S.tx.Prepare(query)
}

func (S *SQLTransaction) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return S.tx.Query(query, args...)
}

func (S *SQLTransaction) QueryRow(query string, args ...interface{}) *sql.Row {
	return S.tx.QueryRow(query, args...)
}

func (S *SQLTransaction) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return S.tx.QueryContext(ctx, query, args...)
}

func (S *SQLTransaction) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return S.tx.ExecContext(ctx, query, args...)
}

type SQLRepository struct {
	db  *sql.DB
	con Transaction
}

type SQLServerConfig struct {
	Host     string `mapstructure:"host", envconfig:"DB_HOST"`
	Port     int    `mapstructure:"port", envconfig:"DB_PORT"`
	User     string `mapstructure:"user", envconfig:"DB_USER"`
	Password string `mapstructure:"password", envconfig:"DB_PASSWORD"`
	Scheme   string `mapstructure:"scheme", envconfig:"DB_SCHEME"`
	Driver   string `mapstructure:"driver", envconfig:"DB_DRIVER"`
}

func NewSQLRepository(config SQLServerConfig) (*SQLRepository, error) {
	connectionString := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", config.Host, config.Port, config.User, config.Password, config.Scheme)
	db, err := sql.Open(config.Driver, connectionString)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetConnMaxLifetime(0)
	db.SetMaxIdleConns(5)
	/*	go func(){
		for {
			fmt.Printf("In use %d not use %d  open %d wait %d\n",db.Stats().Idle, db.Stats().InUse, db.Stats().OpenConnections,db.Stats().WaitCount)
			time.Sleep(time.Second*5)
		}
	}()*/
	return &SQLRepository{
		db: db,
	}, nil

}

func (S *SQLRepository) Initialize(ctx context.Context) error {
	return S.prepareDatabase(ctx)
}

func (S *SQLRepository) ProcessEvent(ctx context.Context, taskEvent *model.TaskEvent) error {
	var err error
	switch taskEvent.EventType {
	case model.PingEvent:
		err = S.PingServerUpdate(ctx, taskEvent.WorkerName, taskEvent.IP)
	case model.NotificationEvent:
		err = S.AddNewTaskEvent(ctx, taskEvent)
	}
	return err
}

//go:embed resources/database/*.sql
var databaseSchemas embed.FS

func (S *SQLRepository) prepareDatabase(ctx context.Context) error {
	var currentVersion int
	txErr := S.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		con, err := tx.getConnection(ctx)
		if err != nil {
			return err
		}

		// Ensure `schema_version` table exists
		const createTableSchemaVersion = `
            CREATE TABLE IF NOT EXISTS schema_version (
                version INT PRIMARY KEY,
                applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
            );
        `
		_, err = con.ExecContext(ctx, createTableSchemaVersion)
		if err != nil {
			return fmt.Errorf("failed to ensure schema_version table: %w", err)
		}

		rows, err := con.QueryContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version;`)
		if err != nil {
			return fmt.Errorf("failed to fetch schema version: %w", err)
		}
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

		txErr = S.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
			con, err := tx.getConnection(ctx)
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

			return nil
		})
		if txErr != nil {
			return txErr
		}
	}
	return nil
}

func (S *SQLRepository) getConnection(ctx context.Context) (Transaction, error) {
	//return S.db.Conn(ctx)
	if S.con != nil {
		return S.con, nil
	}
	return S.db, nil
}
func (S *SQLRepository) GetWorker(ctx context.Context, name string) (worker *model.Worker, err error) {
	db, err := S.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	worker, err = S.getWorker(ctx, db, name)
	return worker, err
}
func (S *SQLRepository) getWorker(ctx context.Context, db Transaction, name string) (*model.Worker, error) {
	rows, err := db.QueryContext(ctx, "select * from workers where name=$1", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	worker := model.Worker{}
	found := false
	if rows.Next() {
		rows.Scan(&worker.Name, &worker.Ip, &worker.QueueName, &worker.LastSeen)
		found = true
	}
	if !found {
		return nil, fmt.Errorf("%w, %s", ErrElementNotFound, name)
	}
	return &worker, err
}

func (S *SQLRepository) GetJob(ctx context.Context, uuid string) (video *model.Job, returnError error) {
	db, err := S.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	video, err = S.getJob(ctx, db, uuid)
	return video, err
}

func (S *SQLRepository) RetrieveQueuedJob(ctx context.Context) (video *model.Job, err error) {
	conn, err := S.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	return S.queuedJob(ctx, conn)
}

func (S *SQLRepository) GetJobsByStatus(ctx context.Context, status model.NotificationStatus) (jobs []*model.Job, returnError error) {
	conn, err := S.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	return S.getJobsByStatus(ctx, conn, status)
}

func (S *SQLRepository) getJobsByStatus(ctx context.Context, tx Transaction, statusFilter model.NotificationStatus) ([]*model.Job, error) {
	rows, err := tx.QueryContext(ctx, "SELECT v.id, v.source_path,v.source_size, v.target_path,v.target_size FROM jobs v INNER JOIN job_status vs ON v.id = vs.job_id WHERE vs.status = $1;", statusFilter)
	if err != nil {
		return nil, err
	}
	var jobs []*model.Job
	defer rows.Close()
	for rows.Next() {
		job := model.Job{}
		rows.Scan(&job.Id, &job.SourcePath, &job.SourceSize, &job.TargetPath, &job.TargetSize)
		taskEvents, err := S.getTaskEvents(ctx, tx, job.Id.String())
		if err != nil {
			return nil, err
		}
		job.Events = taskEvents
		lastUpdate, status, statusPhase, statusMessage, _ := S.getJobStatus(ctx, tx, job.Id.String())
		if lastUpdate != nil {
			job.LastUpdate = lastUpdate
		}
		job.Status = status
		job.StatusPhase = statusPhase
		job.StatusMessage = statusMessage
		jobs = append(jobs, &job)
	}

	return jobs, nil
}

func (S *SQLRepository) GetTimeoutJobs(ctx context.Context, timeout time.Duration) (taskEvent []*model.TaskEvent, returnError error) {
	conn, err := S.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	taskEvent, err = S.getTimeoutJobs(ctx, conn, timeout)
	if err != nil {
		return nil, err
	}
	return taskEvent, nil
}

func (S *SQLRepository) getJob(ctx context.Context, tx Transaction, uuid string) (*model.Job, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, source_path, source_size, target_path, target_size FROM jobs WHERE id=$1", uuid)
	if err != nil {
		return nil, err
	}
	job := model.Job{}
	found := false
	if rows.Next() {
		rows.Scan(&job.Id, &job.SourcePath, &job.SourceSize, &job.TargetPath, &job.TargetSize)
		found = true
	}
	rows.Close()
	if !found {
		return nil, fmt.Errorf("%w, %s", ErrElementNotFound, uuid)
	}

	taskEvents, err := S.getTaskEvents(ctx, tx, job.Id.String())
	if err != nil {
		return nil, err
	}
	job.Events = taskEvents
	lastUpdate, status, statusPhase, statusMessage, _ := S.getJobStatus(ctx, tx, job.Id.String())

	if lastUpdate != nil {
		job.LastUpdate = lastUpdate
	}
	job.Status = status
	job.StatusPhase = statusPhase
	job.StatusMessage = statusMessage

	return &job, nil
}

func (S *SQLRepository) GetJobs(ctx context.Context) (jobs *[]model.Job, returnError error) {
	db, err := S.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	jobs, err = S.getJobs(ctx, db)
	return jobs, err
}

func (S *SQLRepository) getJobs(ctx context.Context, tx Transaction) (*[]model.Job, error) {
	query := fmt.Sprintf(`
    SELECT v.id, v.source_path,v.source_size, v.target_path,v.target_size, vs.event_time, vs.status, vs.notification_type, vs.message
    FROM jobs v
    INNER JOIN job_status vs ON v.id = vs.job_id
`)
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []model.Job{}
	for rows.Next() {
		job := model.Job{}
		rows.Scan(&job.Id, &job.SourcePath, &job.SourceSize, &job.TargetPath, &job.TargetSize, &job.LastUpdate, &job.Status, &job.StatusPhase, &job.StatusMessage)
		jobs = append(jobs, job)
	}

	return &jobs, nil
}

func (S *SQLRepository) getTaskEvents(ctx context.Context, tx Transaction, uuid string) ([]*model.TaskEvent, error) {
	rows, err := tx.QueryContext(ctx, "select * from job_events where job_id=$1 order by event_time asc", uuid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var taskEvents []*model.TaskEvent
	for rows.Next() {
		event := model.TaskEvent{}
		rows.Scan(&event.Id, &event.EventID, &event.WorkerName, &event.EventTime, &event.EventType, &event.NotificationType, &event.Status, &event.Message)
		taskEvents = append(taskEvents, &event)
	}
	return taskEvents, nil
}

func (S *SQLRepository) getJobStatus(ctx context.Context, tx Transaction, uuid string) (*time.Time, string, model.NotificationType, string, error) {
	var last_update time.Time
	var status string
	var statusPhase model.NotificationType
	var message string

	rows, err := tx.QueryContext(ctx, "SELECT event_time, status, notification_type, message FROM job_status WHERE job_id=$1", uuid)
	if err != nil {
		return &last_update, status, statusPhase, message, err
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&last_update, &status, &statusPhase, &message)
	}
	return &last_update, status, statusPhase, message, nil
}

func (S *SQLRepository) getJobByPath(ctx context.Context, tx Transaction, path string) (*model.Job, error) {
	rows, err := tx.QueryContext(ctx, "select * from jobs where source_path=$1", path)
	if err != nil {
		return nil, err
	}
	job := model.Job{}
	found := false
	if rows.Next() {
		rows.Scan(&job.Id, &job.SourcePath, &job.TargetPath, &job.SourceSize, &job.TargetSize)
		found = true
	}
	rows.Close()
	if !found {
		return nil, nil
	}

	taskEvents, err := S.getTaskEvents(ctx, tx, job.Id.String())
	if err != nil {
		return nil, err
	}
	job.Events = taskEvents
	lastUpdate, status, statusPhase, statusMessage, _ := S.getJobStatus(ctx, tx, job.Id.String())

	if lastUpdate != nil {
		job.LastUpdate = lastUpdate
	}
	job.Status = status
	job.StatusPhase = statusPhase
	job.StatusMessage = statusMessage
	return &job, nil
}

func (S *SQLRepository) GetJobByPath(ctx context.Context, path string) (video *model.Job, returnError error) {
	conn, err := S.getConnection(ctx)
	if err != nil {
		return nil, err
	}
	return S.getJobByPath(ctx, conn, path)
}

func (S *SQLRepository) PingServerUpdate(ctx context.Context, name string, ip string) (returnError error) {
	conn, err := S.getConnection(ctx)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "INSERT INTO workers (name, ip,last_seen ) VALUES ($1,$2,$3) ON CONFLICT (name) DO UPDATE SET ip = $2, last_seen=$3;", name, ip, time.Now())
	return err
}

func (S *SQLRepository) AddNewTaskEvent(ctx context.Context, event *model.TaskEvent) (returnError error) {
	conn, err := S.getConnection(ctx)
	if err != nil {
		return err
	}
	return S.addNewTaskEvent(ctx, conn, event)
}

func (S *SQLRepository) addNewTaskEvent(ctx context.Context, tx Transaction, event *model.TaskEvent) error {
	rows, err := tx.QueryContext(ctx, "select max(job_event_id) from job_events where job_id=$1", event.Id.String())
	if err != nil {
		return err
	}

	videoEventID := -1
	if rows.Next() {
		rows.Scan(&videoEventID)
	}
	rows.Close()
	if videoEventID+1 != event.EventID {
		return fmt.Errorf("EventID for %s not match,lastReceived %d, new %d", event.Id.String(), videoEventID, event.EventID)
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO job_events (job_id, job_event_id,worker_name,event_time,event_type,notification_type,status,message)"+
		" VALUES ($1,$2,$3,$4,$5,$6,$7,$8)", event.Id.String(), event.EventID, event.WorkerName, time.Now(), event.EventType, event.NotificationType, event.Status, event.Message)
	return err
}
func (S *SQLRepository) AddJob(ctx context.Context, job *model.Job) error {
	conn, err := S.getConnection(ctx)
	if err != nil {
		return err
	}
	return S.addJob(ctx, conn, job)
}

func (S *SQLRepository) addJob(ctx context.Context, tx Transaction, job *model.Job) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO jobs (id, source_path,target_path,source_size,target_size)"+
		" VALUES ($1,$2,$3,$4,$5)", job.Id.String(), job.SourcePath, job.TargetPath, job.SourceSize, job.TargetSize)
	return err
}

func (S *SQLRepository) UpdateJob(ctx context.Context, job *model.Job) error {
	conn, err := S.getConnection(ctx)
	if err != nil {
		return err
	}
	return S.updateJob(ctx, conn, job)
}

func (S *SQLRepository) updateJob(ctx context.Context, tx Transaction, job *model.Job) error {
	_, err := tx.ExecContext(ctx, "UPDATE jobs SET source_path=$1, target_path=$2, source_size=$3, target_size=$4 WHERE id=$5", job.SourcePath, job.TargetPath, job.SourceSize, job.TargetSize, job.Id.String())
	return err
}

func (S *SQLRepository) getTimeoutJobs(ctx context.Context, tx Transaction, timeout time.Duration) ([]*model.TaskEvent, error) {
	timeoutDate := time.Now().Add(-timeout)
	timeoutDate.Format(time.RFC3339)

	rows, err := tx.QueryContext(ctx, "select v.* from job_events v right join "+
		"(select job_id,max(job_event_id) as job_event_id  from job_events where notification_type='Job'  group by job_id) as m "+
		"on m.job_id=v.job_id and m.job_event_id=v.job_event_id where status='assigned' and v.event_time < $1::timestamptz", timeoutDate)

	//2020-05-17 20:50:41.428531 +00:00
	if err != nil {
		return nil, err
	}
	var taskEvents []*model.TaskEvent
	for rows.Next() {
		event := model.TaskEvent{}
		rows.Scan(&event.Id, &event.EventID, &event.WorkerName, &event.EventTime, &event.EventType, &event.NotificationType, &event.Status, &event.Message)
		taskEvents = append(taskEvents, &event)
	}
	return taskEvents, nil
}

func (S *SQLRepository) WithTransaction(ctx context.Context, transactionFunc func(ctx context.Context, tx Repository) error) error {
	sqlTx, err := S.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelDefault})
	if err != nil {
		return err
	}
	txRepository := *S
	txRepository.con = &SQLTransaction{
		tx: sqlTx,
	}

	err = transactionFunc(ctx, &txRepository)
	if err != nil {
		sqlTx.Rollback()
		return err
	} else {
		if err = sqlTx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (S *SQLRepository) queuedJob(ctx context.Context, tx Transaction) (*model.Job, error) {
	rows, err := tx.QueryContext(ctx, "select job_id, job_event_id from job_status where notification_type='Job' and status='queued' order by event_time asc limit 1")

	//2020-05-17 20:50:41.428531 +00:00
	if err != nil {
		return nil, err
	}

	if rows.Next() {
		event := model.TaskEvent{}
		rows.Scan(&event.Id, &event.EventID)
		return S.getJob(ctx, tx, event.Id.String())
	}
	defer rows.Close()
	return nil, fmt.Errorf("%w, %s", ErrElementNotFound, "No jobs found")
}
