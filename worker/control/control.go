package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"
)

var ErrJobNotPausable = errors.New("job has no active controllable encoding")

const controlRequestTimeout = 15 * time.Second

type AdmissionStatus struct {
	RAMPaused        bool           `json:"ramPaused"`
	PauseMode        string         `json:"pauseMode"`
	SuspendedEncodes int            `json:"suspendedEncodes"`
	Accepting        bool           `json:"accepting"`
	ActiveJobs       uint32         `json:"activeJobs"`
	ReservedJobs     int            `json:"reservedJobs"`
	MaxActiveJobs    int            `json:"maxActiveJobs"`
	MaxAcceptedJobs  int            `json:"maxAcceptedJobs"`
	ActiveEncodes    int            `json:"activeEncodes"`
	MaxEncodes       int            `json:"maxEncodes"`
	WithinSchedule   bool           `json:"withinSchedule"`
	Paused           bool           `json:"paused"`
	Progress         []TaskProgress `json:"progress,omitempty"`
	Jobs             []JobState     `json:"jobs,omitempty"`
}

type JobState struct {
	PausedByWorker     bool   `json:"pausedByWorker"`
	CanPause           bool   `json:"canPause"`
	IndividuallyPaused bool   `json:"individuallyPaused"`
	JobID              string `json:"jobId"`
	Name               string `json:"name,omitempty"`
	Profile            string `json:"profile,omitempty"`
	Codec              string `json:"codec,omitempty"`
	Phase              string `json:"phase"`
	State              string `json:"state"`
	Downloaded         bool   `json:"downloaded"`
}

type TaskProgress struct {
	JobID           string  `json:"jobId"`
	Name            string  `json:"name,omitempty"`
	Profile         string  `json:"profile,omitempty"`
	Codec           string  `json:"codec,omitempty"`
	StepID          string  `json:"stepId"`
	Phase           string  `json:"phase"`
	Percent         float64 `json:"percent"`
	ETASeconds      int64   `json:"etaSeconds"`
	ElapsedSeconds  int64   `json:"elapsedSeconds"`
	Value           int64   `json:"value"`
	Total           int64   `json:"total"`
	BytesPerSecond  int64   `json:"bytesPerSecond,omitempty"`
	Frames          int64   `json:"frames,omitempty"`
	FramesPerSecond float64 `json:"framesPerSecond,omitempty"`
	EncoderSpeed    string  `json:"encoderSpeed,omitempty"`
}

type AdmissionController interface {
	AdmissionStatus() AdmissionStatus
	PersistMaxActiveJobs(int) error
}

type Request struct {
	JobID     string `json:"jobId,omitempty"`
	PauseMode string `json:"pauseMode,omitempty"`
	Command   string `json:"command"`
	MaxJobs   int    `json:"maxJobs,omitempty"`
}

type Response struct {
	Status AdmissionStatus `json:"status"`
	Error  string          `json:"error,omitempty"`
}

type Server struct {
	listener net.Listener
	state    AdmissionController
	shutdown func()
}

func Start(path string, state AdmissionController, shutdown ...func()) (*Server, error) {
	if path == "" {
		path = ConfiguredPath()
	}
	listener, err := listenControl(path)
	if err != nil {
		return nil, err
	}
	server := &Server{listener: listener, state: state}
	if len(shutdown) > 0 {
		server.shutdown = shutdown[0]
	}
	go server.acceptLoop()
	return server, nil
}

func (s *Server) Close() error { return closeControl(s.listener) }

func (s *Server) Serve(ctx context.Context) {
	<-ctx.Done()
	_ = s.Close()
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	// PersistMaxActiveJobs may make a server request with its own 10-second
	// timeout. Keep the pipe open long enough for that request to finish and
	// return its result to the CLI/dashboard.
	_ = conn.SetDeadline(time.Now().Add(controlRequestTimeout))
	var req Request
	decoder := json.NewDecoder(bufio.NewReader(conn))
	if err := decoder.Decode(&req); err != nil {
		s.writeResponse(conn, Response{Error: "invalid control request"})
		return
	}
	result := Response{}
	stop := false
	switch req.Command {
	case "status":
	case "hold-admission":
		if h, ok := s.state.(interface{ HoldAdmission() error }); ok {
			if err := h.HoldAdmission(); err != nil {
				result.Error = err.Error()
			}
		} else {
			result.Error = "mandatory admission hold is unavailable"
		}
	case "pause-job", "resume-job":
		if p, ok := s.state.(interface{ SetJobPaused(string, bool) error }); ok {
			if err := p.SetJobPaused(req.JobID, req.Command == "pause-job"); err != nil {
				result.Error = err.Error()
			}
		} else {
			result.Error = "individual encoding control is unavailable"
		}
	case "set-pause-mode":
		if p, ok := s.state.(interface{ SetPauseMode(string) error }); ok {
			if err := p.SetPauseMode(req.PauseMode); err != nil {
				result.Error = err.Error()
			}
		} else {
			result.Error = "pause mode control is unavailable"
		}
	case "pause-encodes", "resume-encodes":
		if p, ok := s.state.(interface{ SetRAMPaused(bool) error }); ok {
			if err := p.SetRAMPaused(req.Command == "pause-encodes"); err != nil {
				result.Error = err.Error()
			}
		} else {
			result.Error = "RAM encoding control is unavailable"
		}
	case "set-admission-limit":
		if p, ok := s.state.(interface{ PersistAdmissionLimit(int) error }); ok {
			if err := p.PersistAdmissionLimit(req.MaxJobs); err != nil {
				result.Error = err.Error()
			}
		} else {
			result.Error = "independent admission control is unavailable"
		}
	case "set-max-jobs":
		if err := s.state.PersistMaxActiveJobs(req.MaxJobs); err != nil {
			result.Error = err.Error()
		}
	case "finish-accepted":
		if d, ok := s.state.(interface{ FinishAcceptedJobs() error }); ok {
			if err := d.FinishAcceptedJobs(); err != nil {
				result.Error = err.Error()
			}
		} else {
			result.Error = "finishing accepted work is unavailable"
		}
	case "drain":
		if d, ok := s.state.(interface{ DrainAcceptedJobs() error }); ok {
			if err := d.DrainAcceptedJobs(); err != nil {
				result.Error = err.Error()
			}
		} else {
			result.Error = "draining is unavailable"
		}
	case "shutdown":
		if s.shutdown == nil {
			result.Error = "shutdown is unavailable"
		} else {
			stop = true
		}
	default:
		result.Error = "unknown control command"
	}
	result.Status = s.state.AdmissionStatus()
	s.writeResponse(conn, result)
	if stop {
		s.shutdown()
	}
}

func (s *Server) writeResponse(conn net.Conn, result Response) {
	_ = json.NewEncoder(conn).Encode(result)
}

func RunCLI(args []string, output io.Writer) error {
	return runCLI(args, output, ConfiguredPath())
}

// ConfiguredPath lets supervised worker instances use distinct local control endpoints.
func ConfiguredPath() string {
	if path := os.Getenv("TRANSCODERD_CONTROL_PIPE"); path != "" {
		return path
	}
	return SocketPath
}

func runCLI(args []string, output io.Writer, socketPath string) error {
	if len(args) == 0 {
		return errors.New("usage: transcoderd-worker control status|set-max-jobs <N>|set-admission-limit <N>|pause-encodes|resume-encodes|set-pause-mode <finish|ram>|pause-job|resume-job <job UUID>")
	}
	req := Request{Command: args[0]}
	switch req.Command {
	case "status", "pause-encodes", "resume-encodes":
		if len(args) != 1 {
			return fmt.Errorf("usage: transcoderd-worker control %s", req.Command)
		}
	case "pause-job", "resume-job":
		if len(args) != 2 || args[1] == "" {
			return errors.New("usage: transcoderd-worker control pause-job|resume-job <job UUID>")
		}
		req.JobID = args[1]
	case "set-pause-mode":
		if len(args) != 2 || (args[1] != "finish" && args[1] != "ram") {
			return errors.New("usage: transcoderd-worker control set-pause-mode <finish|ram>")
		}
		req.PauseMode = args[1]
	case "set-max-jobs", "set-admission-limit":
		if len(args) != 2 {
			return errors.New("usage: transcoderd-worker control set-max-jobs <non-negative integer>")
		}
		maxJobs, err := strconv.Atoi(args[1])
		if err != nil || maxJobs < 0 {
			return errors.New("maximum active jobs must be a non-negative integer")
		}
		req.MaxJobs = maxJobs
	default:
		return fmt.Errorf("unknown control command %q", req.Command)
	}
	result, err := Call(socketPath, req)
	if err != nil {
		return err
	}
	if req.Command == "status" {
		_, err = fmt.Fprintf(output, "accepting=%t active=%d reserved=%d max_jobs=%d admitted_limit=%d encodes=%d/%d within_schedule=%t paused=%t ram_paused=%t pause_mode=%s suspended_encodes=%d\n", result.Status.Accepting, result.Status.ActiveJobs, result.Status.ReservedJobs, result.Status.MaxActiveJobs, result.Status.MaxAcceptedJobs, result.Status.ActiveEncodes, result.Status.MaxEncodes, result.Status.WithinSchedule, result.Status.Paused, result.Status.RAMPaused, result.Status.PauseMode, result.Status.SuspendedEncodes)
		return err
	}
	if req.Command == "pause-encodes" || req.Command == "resume-encodes" {
		_, err = fmt.Fprintf(output, "ram_paused=%t; max_jobs=%d; suspended_encodes=%d\n", result.Status.RAMPaused, result.Status.MaxActiveJobs, result.Status.SuspendedEncodes)
		return err
	}
	if req.Command == "pause-job" || req.Command == "resume-job" {
		_, err = fmt.Fprintf(output, "%s applied to %s; suspended_encodes=%d\n", req.Command, req.JobID, result.Status.SuspendedEncodes)
		return err
	}
	if req.Command == "set-pause-mode" {
		_, err = fmt.Fprintf(output, "pause mode set to %s; suspended_encodes=%d\n", result.Status.PauseMode, result.Status.SuspendedEncodes)
		return err
	}
	_, err = fmt.Fprintf(output, "max jobs set to %d; admitted_limit=%d; accepting=%t encodes=%d/%d\n", result.Status.MaxActiveJobs, result.Status.MaxAcceptedJobs, result.Status.Accepting, result.Status.ActiveEncodes, result.Status.MaxEncodes)
	return err
}

func Call(socketPath string, req Request) (Response, error) {
	if socketPath == "" {
		socketPath = ConfiguredPath()
	}
	conn, err := dialControl(socketPath)
	if err != nil {
		return Response{}, fmt.Errorf("connect to running worker control socket: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(controlRequestTimeout))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, fmt.Errorf("send control request: %w", err)
	}
	var result Response
	if err := json.NewDecoder(conn).Decode(&result); err != nil {
		return Response{}, fmt.Errorf("read control response: %w", err)
	}
	if result.Error != "" {
		return result, errors.New(result.Error)
	}
	return result, nil
}
