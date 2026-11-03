package model

import "time"

// WorkerRuntime is optional so older workers can continue publishing presence.
type WorkerRuntime struct {
	CapacityVersion  int    `json:"capacityVersion,omitempty"`
	CPUJobs          int    `json:"cpuJobs"`
	GPUJobs          int    `json:"gpuJobs"`
	ActiveCPUEncodes int    `json:"activeCPUEncodes"`
	ActiveGPUEncodes int    `json:"activeGPUEncodes"`
	CPUMaxEncodes    int    `json:"cpuMaxEncodes"`
	GPUMaxEncodes    int    `json:"gpuMaxEncodes"`
	CPUEncoder       string `json:"cpuEncoder,omitempty"`
	GPUEncoder       string `json:"gpuEncoder,omitempty"`
	Encoder          string `json:"encoder"`
	Kind             string `json:"kind"`
	Accepting        bool   `json:"accepting"`
	WithinSchedule   bool   `json:"withinSchedule"`
	ActiveJobs       uint32 `json:"activeJobs"`
	ReservedJobs     int    `json:"reservedJobs"`
	MaxActiveJobs    int    `json:"maxActiveJobs"`
	ActiveEncodes    int    `json:"activeEncodes"`
	MaxEncodes       int    `json:"maxEncodes"`
}

type Dashboard struct {
	GeneratedAt time.Time           `json:"generatedAt"`
	Hours       int                 `json:"hours"`
	Summary     DashboardSummary    `json:"summary"`
	Timeline    []DashboardBucket   `json:"timeline"`
	Workers     []DashboardWorker   `json:"workers"`
	Jobs        []DashboardJob      `json:"jobs"`
	Codecs      []DashboardCodec    `json:"codecs"`
	Activity    []DashboardActivity `json:"activity"`
}

type DashboardSummary struct {
	Total              int64   `json:"total"`
	Queued             int64   `json:"queued"`
	Active             int64   `json:"active"`
	Completed          int64   `json:"completed"`
	Failed             int64   `json:"failed"`
	Canceled           int64   `json:"canceled"`
	SourceBytes        int64   `json:"sourceBytes"`
	TargetBytes        int64   `json:"targetBytes"`
	SavedBytes         int64   `json:"savedBytes"`
	CompressionPercent float64 `json:"compressionPercent"`
	CompletedInRange   int64   `json:"completedInRange"`
	FailedInRange      int64   `json:"failedInRange"`
	AverageSeconds     float64 `json:"averageSeconds"`
}

type DashboardBucket struct {
	Time        time.Time `json:"time"`
	Completed   int64     `json:"completed"`
	Failed      int64     `json:"failed"`
	SourceBytes int64     `json:"sourceBytes"`
	TargetBytes int64     `json:"targetBytes"`
}

type DashboardWorker struct {
	Name           string         `json:"name"`
	IP             string         `json:"ip"`
	LastSeen       *time.Time     `json:"lastSeen"`
	Online         bool           `json:"online"`
	Runtime        *WorkerRuntime `json:"runtime"`
	DesiredMax     *int           `json:"desiredMax"`
	DesiredCPUJobs *int           `json:"desiredCPUJobs"`
	DesiredGPUJobs *int           `json:"desiredGPUJobs"`
	ActiveJobs     int64          `json:"activeJobs"`
	Completed      int64          `json:"completed"`
	Failed         int64          `json:"failed"`
	SourceBytes    int64          `json:"sourceBytes"`
	TargetBytes    int64          `json:"targetBytes"`
}

type DashboardProgress struct {
	ID         string    `json:"id"`
	Phase      string    `json:"phase"`
	Percent    float64   `json:"percent"`
	ETASeconds float64   `json:"etaSeconds"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Stale      bool      `json:"stale"`
}

type DashboardJob struct {
	ID          string              `json:"id"`
	SourcePath  string              `json:"sourcePath"`
	SourceBytes int64               `json:"sourceBytes"`
	TargetBytes int64               `json:"targetBytes"`
	State       string              `json:"state"`
	Phase       string              `json:"phase"`
	Worker      string              `json:"worker"`
	Message     string              `json:"message"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	Progress    []DashboardProgress `json:"progress"`
}

type DashboardCodec struct {
	Name        string `json:"name"`
	Jobs        int64  `json:"jobs"`
	SourceBytes int64  `json:"sourceBytes"`
	TargetBytes int64  `json:"targetBytes"`
}

type DashboardActivity struct {
	JobID      string    `json:"jobId"`
	SourcePath string    `json:"sourcePath"`
	Worker     string    `json:"worker"`
	Phase      string    `json:"phase"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	Time       time.Time `json:"time"`
}

func NewDashboard(now time.Time, hours int) *Dashboard {
	return &Dashboard{GeneratedAt: now.UTC(), Hours: hours,
		Timeline: []DashboardBucket{}, Workers: []DashboardWorker{}, Jobs: []DashboardJob{},
		Codecs: []DashboardCodec{}, Activity: []DashboardActivity{}}
}

func (d *Dashboard) Finalize() {
	d.Summary.SavedBytes = d.Summary.SourceBytes - d.Summary.TargetBytes
	if d.Summary.SourceBytes > 0 {
		d.Summary.CompressionPercent = 100 * float64(d.Summary.SavedBytes) / float64(d.Summary.SourceBytes)
	}
	for i := range d.Workers {
		w := &d.Workers[i]
		w.Online = w.LastSeen != nil && d.GeneratedAt.Sub(*w.LastSeen) <= 30*time.Second
	}
}
