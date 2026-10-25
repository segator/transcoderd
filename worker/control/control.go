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
	"path/filepath"
	"strconv"
	"time"
)

const SocketPath = "/run/transcoderd-worker/control.sock"

type AdmissionStatus struct {
	Accepting      bool   `json:"accepting"`
	ActiveJobs     uint32 `json:"activeJobs"`
	ReservedJobs   int    `json:"reservedJobs"`
	MaxActiveJobs  int    `json:"maxActiveJobs"`
	ActiveEncodes  int    `json:"activeEncodes"`
	MaxEncodes     int    `json:"maxEncodes"`
	WithinSchedule bool   `json:"withinSchedule"`
}

type AdmissionController interface {
	AdmissionStatus() AdmissionStatus
	SetMaxActiveJobs(int) error
}

type request struct {
	Command string `json:"command"`
	MaxJobs int    `json:"maxJobs,omitempty"`
}

type response struct {
	Status AdmissionStatus `json:"status"`
	Error  string          `json:"error,omitempty"`
}

type Server struct {
	listener net.Listener
	state    AdmissionController
}

func Start(path string, state AdmissionController) (*Server, error) {
	if path == "" {
		path = SocketPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create worker control directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("restrict worker control directory permissions: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("control socket path exists and is not a socket: %s", path)
		}
		conn, dialErr := net.DialTimeout("unix", path, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("worker control socket is already in use: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale control socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect control socket: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on worker control socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("restrict control socket permissions: %w", err)
	}
	server := &Server{listener: listener, state: state}
	go server.acceptLoop()
	return server, nil
}

func (s *Server) Close() error {
	path := s.listener.Addr().String()
	closeErr := s.listener.Close()
	removeErr := os.Remove(path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(closeErr, removeErr)
}

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
	var req request
	decoder := json.NewDecoder(bufio.NewReader(conn))
	if err := decoder.Decode(&req); err != nil {
		s.writeResponse(conn, response{Error: "invalid control request"})
		return
	}
	result := response{}
	switch req.Command {
	case "status":
	case "set-max-jobs":
		if err := s.state.SetMaxActiveJobs(req.MaxJobs); err != nil {
			result.Error = err.Error()
		}
	default:
		result.Error = "unknown control command"
	}
	result.Status = s.state.AdmissionStatus()
	s.writeResponse(conn, result)
}

func (s *Server) writeResponse(conn net.Conn, result response) {
	_ = json.NewEncoder(conn).Encode(result)
}

func RunCLI(args []string, output io.Writer) error {
	return runCLI(args, output, SocketPath)
}

func runCLI(args []string, output io.Writer, socketPath string) error {
	if len(args) == 0 {
		return errors.New("usage: transcoderd-worker control status|set-max-jobs <N>")
	}
	req := request{Command: args[0]}
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
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("connect to running worker control socket: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("send control request: %w", err)
	}
	var result response
	if err := json.NewDecoder(conn).Decode(&result); err != nil {
		return fmt.Errorf("read control response: %w", err)
	}
	if result.Error != "" {
		return errors.New(result.Error)
	}
	if req.Command == "status" {
		_, err = fmt.Fprintf(output, "accepting=%t active=%d reserved=%d max=%d encodes=%d/%d within_schedule=%t\n", result.Status.Accepting, result.Status.ActiveJobs, result.Status.ReservedJobs, result.Status.MaxActiveJobs, result.Status.ActiveEncodes, result.Status.MaxEncodes, result.Status.WithinSchedule)
		return err
	}
	_, err = fmt.Fprintf(output, "max active jobs set to %d; accepting=%t encodes=%d/%d\n", result.Status.MaxActiveJobs, result.Status.Accepting, result.Status.ActiveEncodes, result.Status.MaxEncodes)
	return err
}
