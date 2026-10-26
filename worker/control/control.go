package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

type AdmissionStatus struct {
	Accepting       bool           `json:"accepting"`
	ActiveJobs      uint32         `json:"activeJobs"`
	ReservedJobs    int            `json:"reservedJobs"`
	MaxActiveJobs   int            `json:"maxActiveJobs"`
	MaxAcceptedJobs int            `json:"maxAcceptedJobs"`
	ActiveEncodes   int            `json:"activeEncodes"`
	MaxEncodes      int            `json:"maxEncodes"`
	WithinSchedule  bool           `json:"withinSchedule"`
	Progress        []TaskProgress `json:"progress,omitempty"`
	Jobs            []JobState     `json:"jobs,omitempty"`
}

type JobState struct {
	JobID      string `json:"jobId"`
	Name       string `json:"name,omitempty"`
	Phase      string `json:"phase"`
	State      string `json:"state"`
	Downloaded bool   `json:"downloaded"`
}

type TaskProgress struct {
	JobID      string  `json:"jobId"`
	Name       string  `json:"name,omitempty"`
	StepID     string  `json:"stepId"`
	Phase      string  `json:"phase"`
	Percent    float64 `json:"percent"`
	ETASeconds int64   `json:"etaSeconds"`
}

type AdmissionController interface {
	AdmissionStatus() AdmissionStatus
	PersistMaxActiveJobs(int) error
}

type Request struct {
	Command string `json:"command"`
	MaxJobs int    `json:"maxJobs,omitempty"`
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
		path = SocketPath
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
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
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
	case "set-max-jobs":
		if err := s.state.PersistMaxActiveJobs(req.MaxJobs); err != nil {
			result.Error = err.Error()
		}
	case "drain":
		if d, ok := s.state.(interface{ SetMaxActiveJobs(int) error }); ok {
			if err := d.SetMaxActiveJobs(0); err != nil {
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
	return runCLI(args, output, SocketPath)
}

func runCLI(args []string, output io.Writer, socketPath string) error {
	if len(args) == 0 {
		return errors.New("usage: transcoderd-worker control status|set-max-jobs <N>")
	}
	req := Request{Command: args[0]}
	switch req.Command {
	case "status":
		if len(args) != 1 {
			return fmt.Errorf("usage: transcoderd-worker control %s", req.Command)
		}
	case "set-max-jobs":
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
		_, err = fmt.Fprintf(output, "accepting=%t active=%d reserved=%d max_jobs=%d admitted_limit=%d encodes=%d/%d within_schedule=%t\n", result.Status.Accepting, result.Status.ActiveJobs, result.Status.ReservedJobs, result.Status.MaxActiveJobs, result.Status.MaxAcceptedJobs, result.Status.ActiveEncodes, result.Status.MaxEncodes, result.Status.WithinSchedule)
		return err
	}
	_, err = fmt.Fprintf(output, "max jobs set to %d; admitted_limit=%d; accepting=%t encodes=%d/%d\n", result.Status.MaxActiveJobs, result.Status.MaxAcceptedJobs, result.Status.Accepting, result.Status.ActiveEncodes, result.Status.MaxEncodes)
	return err
}

func Call(socketPath string, req Request) (Response, error) {
	if socketPath == "" {
		socketPath = SocketPath
	}
	conn, err := dialControl(socketPath)
	if err != nil {
		return Response{}, fmt.Errorf("connect to running worker control socket: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
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
