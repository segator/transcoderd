//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"transcoder/model"
	"transcoder/worker/control"
	"transcoder/worker/desktop"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "TranscoderDWorker"
const pipePrefix = `\\.\pipe\transcoderd-desktop-`

func currentSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

func dataDir() string { return filepath.Join(os.Getenv("ProgramData"), "TranscoderD") }

func protectDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	ptr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("service data directory cannot be a reparse point")
	}
	existing, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	existingOwner, _, err := existing.Owner()
	if err != nil {
		return err
	}
	sid, err := currentSID()
	if err != nil {
		return err
	}
	if owner := existingOwner.String(); owner != sid && owner != "S-1-5-18" && owner != "S-1-5-32-544" {
		return errors.New("service data directory is owned by a different user")
	}
	for _, name := range []string{"service.json", "config.yaml", "worker.log", "work"} {
		path, err := windows.UTF16PtrFromString(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		a, err := windows.GetFileAttributes(path)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			continue
		}
		if err != nil {
			return err
		}
		if a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("service data cannot contain reparse points")
		}
	}
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, dacl, nil)
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return err
	}
	return nil
}

func install(owner string) error {
	if _, err := windows.StringToSid(owner); err != nil {
		return fmt.Errorf("invalid installing user SID: %w", err)
	}
	if err := protectDir(dataDir()); err != nil {
		return fmt.Errorf("protect service settings: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if old, err := m.OpenService(serviceName); err == nil {
		old.Close()
		return errors.New("service already exists; stop and remove it before reinstalling")
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{DisplayName: "TranscoderD Worker", Description: "TranscoderD background worker", StartType: mgr.StartAutomatic}, "--run", owner)
	if err != nil {
		return err
	}
	defer s.Close()
	if err = s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}}, 3600); err != nil {
		_ = s.Delete()
		return err
	}
	return s.Start()
}

func uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer s.Close()
	status, err := s.Query()
	if err != nil {
		return err
	}
	if status.State != svc.Stopped {
		return errors.New("stop the service before uninstalling")
	}
	return s.Delete()
}

func prepareUpgrade() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer s.Close()
	state, err := s.Query()
	if err != nil {
		return err
	}
	if state.State == svc.Stopped {
		return nil
	}
	config, err := s.Config()
	if err != nil {
		return err
	}
	owner := regexp.MustCompile(`S-1-(?:[0-9]+-)*[0-9]+`).FindString(config.BinaryPathName)
	if owner == "" {
		return errors.New("cannot identify the service owner")
	}
	timeout := 3 * time.Second
	conn, err := winio.DialPipe(pipePrefix+owner, &timeout)
	if err != nil {
		return fmt.Errorf("cannot contact the service: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	if err = json.NewEncoder(conn).Encode(desktop.Request{Command: "prepare-upgrade"}); err != nil {
		conn.Close()
		return err
	}
	var reply desktop.Response
	err = json.NewDecoder(conn).Decode(&reply)
	conn.Close()
	if err != nil {
		return err
	}
	if reply.Error != "" {
		return errors.New(reply.Error)
	}
	if _, err = s.Control(svc.Stop); err != nil {
		return err
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		state, err = s.Query()
		if err != nil {
			return err
		}
		if state.State == svc.Stopped {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("service did not stop")
}

type supervisor struct {
	mu           sync.Mutex
	state        desktop.Persisted
	dir, worker  string
	child        *exec.Cmd
	message      string
	pendingDrain bool
	readyToApply bool
	maintenance  bool
	queue        *model.QueueSnapshot
	queueError   string
	pipeSuffix   string
	testCancel   context.CancelFunc
}

func (s *supervisor) snapshotLocked() desktop.Status {
	status := desktop.Status{Configured: s.state.Active != nil, Running: s.child != nil, Pending: s.state.Pending != nil || s.state.PendingLimit, Message: s.message, Queue: s.queue, QueueError: s.queueError}
	value := s.state.Active
	if s.state.Pending != nil {
		value = s.state.Pending
	}
	if value != nil {
		settings := value.Public()
		status.Settings = &settings
	}
	if s.child != nil {
		if r, err := control.Call("", control.Request{Command: "status"}); err == nil {
			status.Worker = &r.Status
		} else {
			status.Message = "The worker is starting or not responding"
		}
	}
	return status
}

func sameExceptLimit(a, b desktop.Settings) bool {
	a.MaxJobs = 0
	b.MaxJobs = 0
	return reflect.DeepEqual(a, b)
}

func (s *supervisor) handle(req desktop.Request) desktop.Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := desktop.Response{}
	if s.maintenance && (req.Command == "configure" || req.Command == "set-max-jobs") {
		result.Error = "An installation or upgrade is being prepared"
		result.Status = s.snapshotLocked()
		return result
	}
	switch req.Command {
	case "status":
	case "shutdown-test":
		if s.testCancel == nil {
			result.Error = "Unknown command"
		} else {
			go func() { time.Sleep(200 * time.Millisecond); s.testCancel() }()
		}
	case "configure":
		if req.Settings == nil {
			result.Error = "Settings are missing"
			break
		}
		value := *req.Settings
		current := s.state.Active
		if s.state.Pending != nil {
			current = s.state.Pending
		}
		if value.Token == "" && current != nil {
			value.Token = current.Token
		}
		value.Domain = strings.TrimRight(strings.TrimSpace(value.Domain), "/")
		value.Name = strings.TrimSpace(value.Name)
		if err := value.Validate(); err != nil {
			result.Error = err.Error()
			break
		}
		next := s.state
		switch {
		case next.Active == nil:
			next.Active = &value
			next.PendingLimit = true
		case next.Pending == nil && sameExceptLimit(*next.Active, value):
			next.Active = &value
			next.PendingLimit = true
		default:
			next.Pending = &value
			next.PendingLimit = true
		}
		if err := desktop.Save(s.dir, next); err != nil {
			result.Error = "Could not save settings"
			break
		}
		s.state = next
	case "set-max-jobs":
		if req.MaxJobs < 0 || req.MaxJobs > 64 {
			result.Error = "The number of concurrent encodes must be between 0 and 64"
			break
		}
		if s.state.Active == nil {
			result.Error = "Configure the server first"
			break
		}
		if s.state.Pending != nil {
			result.Error = "Settings are waiting to be applied"
			break
		}
		if s.child == nil {
			result.Error = "The worker is not connected"
			break
		}
		if _, err := control.Call("", control.Request{Command: "set-max-jobs", MaxJobs: req.MaxJobs}); err != nil {
			result.Error = "Could not change the server limit"
			break
		}
		next := s.state
		copy := *next.Active
		copy.MaxJobs = req.MaxJobs
		next.Active = &copy
		next.PendingLimit = false
		if err := desktop.Save(s.dir, next); err != nil {
			result.Error = "The limit changed, but could not be saved locally"
			break
		}
		s.state = next
	case "prepare-upgrade":
		s.maintenance = true
		if s.child != nil {
			r, err := control.Call("", control.Request{Command: "drain"})
			if err != nil || r.Status.ActiveJobs != 0 || r.Status.ReservedJobs != 0 {
				result.Error = "Jobs are still running. Wait for them to finish and try again."
				s.maintenance = false
			}
		}
	default:
		result.Error = "Unknown command"
	}
	result.Status = s.snapshotLocked()
	return result
}

func (s *supervisor) run(ctx context.Context) {
	var done <-chan error
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		if !s.maintenance && s.child == nil && s.state.Active != nil {
			if s.state.Pending == nil && s.state.PendingLimit {
				if err := persistInitialLimit(ctx, *s.state.Active); err != nil {
					s.message = "Could not connect to the server or save the limit; retrying"
					s.mu.Unlock()
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						continue
					}
				}
				next := s.state
				next.PendingLimit = false
				if err := desktop.Save(s.dir, next); err != nil {
					s.message = "Could not save configuration state"
					s.mu.Unlock()
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						continue
					}
				}
				s.state = next
			}
			if err := desktop.WorkerConfig(s.dir, *s.state.Active); err != nil {
				s.message = "Could not prepare the worker configuration"
			} else {
				cmd := exec.Command(s.worker, "--noUpdates")
				cmd.Dir = s.dir
				cmd.Env = append(os.Environ(), "TRANSCODERD_INTERNAL_CONFIG="+filepath.Join(s.dir, "config.yaml"))
				logFile, logErr := os.OpenFile(filepath.Join(s.dir, "worker.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
				if logErr != nil {
					s.message = "Could not open the worker log"
					s.mu.Unlock()
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						continue
					}
				}
				cmd.Stdout = logFile
				cmd.Stderr = logFile
				if err = cmd.Start(); err != nil {
					_ = logFile.Close()
					s.message = "Could not start the worker"
				} else {
					s.child = cmd
					s.pendingDrain = false
					s.message = ""
					ch := make(chan error, 1)
					done = ch
					go func() { err := cmd.Wait(); _ = logFile.Close(); ch <- err }()
				}
			}
		}
		if !s.maintenance && s.child != nil && s.state.Pending != nil {
			if !s.pendingDrain {
				if _, err := control.Call("", control.Request{Command: "drain"}); err == nil {
					s.pendingDrain = true
				}
			}
			if s.pendingDrain {
				if r, err := control.Call("", control.Request{Command: "status"}); err == nil && r.Status.ActiveJobs == 0 && r.Status.ReservedJobs == 0 {
					if _, err := control.Call("", control.Request{Command: "shutdown"}); err == nil {
						s.readyToApply = true
					}
				}
			}
		} else if !s.maintenance && s.child != nil && s.state.PendingLimit {
			if _, err := control.Call("", control.Request{Command: "set-max-jobs", MaxJobs: s.state.Active.MaxJobs}); err == nil {
				next := s.state
				next.PendingLimit = false
				if desktop.Save(s.dir, next) == nil {
					s.state = next
				}
			}
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			s.mu.Lock()
			child := s.child
			s.mu.Unlock()
			if child != nil {
				_, _ = control.Call("", control.Request{Command: "shutdown"})
				select {
				case <-done:
				case <-time.After(8 * time.Second):
					_ = child.Process.Kill()
					<-done
				}
			}
			return
		case err := <-done:
			s.mu.Lock()
			s.child = nil
			done = nil
			s.pendingDrain = false
			if s.state.Pending != nil && s.readyToApply {
				next := s.state
				next.Active = next.Pending
				next.Pending = nil
				if desktop.Save(s.dir, next) == nil {
					s.state = next
				} else {
					s.message = "Could not apply pending settings"
				}
			} else if err != nil {
				s.message = "The worker stopped; retrying"
			}
			s.readyToApply = false
			s.mu.Unlock()
		case <-ticker.C:
		}
	}
}

func persistInitialLimit(ctx context.Context, settings desktop.Settings) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	body, err := json.Marshal(struct {
		Max int `json:"maxActiveJobs"`
	}{settings.MaxJobs})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, settings.Domain+"/api/v1/worker/settings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+settings.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("workerName", settings.Name)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("worker settings returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *supervisor) pollQueue(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		var settings *desktop.Settings
		if s.state.Active != nil {
			value := *s.state.Active
			settings = &value
		}
		s.mu.Unlock()
		if settings != nil {
			qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			queue, err := desktop.FetchQueue(qctx, *settings)
			cancel()
			s.mu.Lock()
			if s.state.Active != nil && s.state.Active.Domain == settings.Domain && s.state.Active.Token == settings.Token {
				s.queue = queue
				s.queueError = ""
				if err != nil {
					s.queueError = "unavailable"
					if errors.Is(err, desktop.ErrQueueUnsupported) {
						s.queueError = "unsupported"
					}
				}
			}
			s.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *supervisor) serve(ctx context.Context, owner string) error {
	if _, err := windows.StringToSid(owner); err != nil {
		return err
	}
	listener, err := winio.ListenPipe(pipePrefix+owner+s.pipeSuffix, &winio.PipeConfig{SecurityDescriptor: "O:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + owner + ")"})
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() { <-ctx.Done(); _ = listener.Close() }()
	for {
		c, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handleConnection(c)
	}
}

func (s *supervisor) handleConnection(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	var req desktop.Request
	if err := json.NewDecoder(bufio.NewReader(io.LimitReader(c, 65536))).Decode(&req); err != nil {
		_ = json.NewEncoder(c).Encode(desktop.Response{Error: "Invalid request"})
		return
	}
	_ = json.NewEncoder(c).Encode(s.handle(req))
}

type handler struct {
	owner       string
	dir, worker string
}

func (h *handler) Execute(_ []string, changes <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	state, err := desktop.Load(h.dir)
	if err != nil {
		return false, 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &supervisor{dir: h.dir, worker: h.worker, state: state}
	ready := make(chan error, 1)
	go func() { ready <- s.serve(ctx, h.owner) }()
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); s.run(ctx) }()
	go s.pollQueue(ctx)
	statuses <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-ready:
			if err != nil {
				cancel()
				<-workerDone
				return false, 1
			}
		case change := <-changes:
			switch change.Cmd {
			case svc.Interrogate:
				statuses <- change.CurrentStatus
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending}
				cancel()
				<-workerDone
				return false, 0
			}
		}
	}
}

func runConsole(dir, worker string) error {
	owner, err := currentSID()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	state, err := desktop.Load(dir)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	supervisor := &supervisor{dir: dir, worker: worker, state: state, pipeSuffix: os.Getenv("TRANSCODERD_TEST_PIPE_SUFFIX"), testCancel: cancel}
	done := make(chan struct{})
	go func() { defer close(done); supervisor.run(ctx) }()
	go supervisor.pollQueue(ctx)
	err = supervisor.serve(ctx, owner)
	cancel()
	<-done
	return err
}

func main() {
	if len(os.Args) == 4 && os.Args[1] == "--console" {
		if err := runConsole(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--start" {
		if err := startService(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--prepare-upgrade" {
		if err := prepareUpgrade(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--install" {
		sid, err := currentSID()
		if err == nil {
			err = install(sid)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--uninstall" {
		if err := uninstall(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 3 || os.Args[1] != "--run" {
		fmt.Fprintln(os.Stderr, "usage: service --install|--uninstall|--run <owner SID>")
		os.Exit(2)
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	h := &handler{owner: os.Args[2], dir: dataDir(), worker: filepath.Join(filepath.Dir(exe), "transcoderd-worker.exe")}
	if err = svc.Run(serviceName, h); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
