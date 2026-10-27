package managed

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"transcoder/helper/command"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const ServiceName = "TranscoderDWorker"
const RecoveryTask = "TranscoderDUpdateRecovery"

type Installation struct {
	Dir   string `json:"dir"`
	Owner string `json:"owner"`
}

func DataDir() string                    { return filepath.Join(os.Getenv("ProgramData"), "TranscoderD") }
func InstallationPath(dir string) string { return filepath.Join(dir, "installation.json") }
func ReadInstallation(dir string) (Installation, error) {
	var i Installation
	path := InstallationPath(dir)
	if err := CheckPath(path); err != nil {
		return i, err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return i, err
	}
	owner, _, err := sd.Owner()
	if err != nil || (owner.String() != "S-1-5-18" && owner.String() != "S-1-5-32-544") {
		return i, errors.New("installation metadata is not owned by SYSTEM or administrators")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return i, err
	}
	err = json.Unmarshal(b, &i)
	if err == nil {
		_, err = windows.StringToSid(i.Owner)
	}
	if err == nil && (!filepath.IsAbs(i.Dir) || filepath.Dir(i.Dir) == i.Dir) {
		err = errors.New("invalid Windows installation directory")
	}
	return i, err
}
func SaveInstallation(dir string, i Installation) error {
	if _, err := windows.StringToSid(i.Owner); err != nil {
		return err
	}
	if !filepath.IsAbs(i.Dir) || filepath.Dir(i.Dir) == i.Dir {
		return errors.New("invalid Windows installation directory")
	}
	return SaveJSON(InstallationPath(dir), i)
}

func ServiceOwner() (string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return "", err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return "", err
	}
	defer s.Close()
	c, err := s.Config()
	if err != nil {
		return "", err
	}
	for _, v := range strings.Fields(c.BinaryPathName) {
		v = strings.Trim(v, "\"")
		if strings.HasPrefix(v, "S-1-") {
			if _, err = windows.StringToSid(v); err == nil {
				return v, nil
			}
		}
	}
	return "", errors.New("cannot identify installed service owner")
}

func serviceStop() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(ServiceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return err
	}
	if st.State == svc.Stopped {
		return nil
	}
	var process windows.Handle
	if st.ProcessId != 0 {
		process, _ = windows.OpenProcess(windows.SYNCHRONIZE, false, st.ProcessId)
		if process != 0 {
			defer func() { _ = windows.CloseHandle(process) }()
		}
	}
	if st.State != svc.StopPending {
		if _, err = s.Control(svc.Stop); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		st, err = s.Query()
		if err != nil {
			return err
		}
		if st.State == svc.Stopped {
			if process != 0 {
				result, e := windows.WaitForSingleObject(process, 20000)
				if e != nil {
					return e
				}
				if result != windows.WAIT_OBJECT_0 {
					return errors.New("windows service process did not exit")
				}
			}
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("windows service did not stop")
}

func serviceStart() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()
	err = s.Start()
	if errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return nil
	}
	return err
}

func health(owner, version string) error {
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		timeout := 2 * time.Second
		c, err := winio.DialPipe(`\\.\pipe\transcoderd-desktop-`+owner, &timeout)
		if err == nil {
			fd, ok := c.(interface{ Fd() uintptr })
			if !ok {
				c.Close()
				return errors.New("cannot verify update service pipe")
			}
			sd, e := windows.GetSecurityInfo(windows.Handle(fd.Fd()), windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
			if e != nil {
				c.Close()
				return e
			}
			sid, _, e := sd.Owner()
			if e != nil || sid.String() != "S-1-5-18" {
				c.Close()
				return errors.New("update service pipe is not owned by SYSTEM")
			}
			_ = c.SetDeadline(time.Now().Add(15 * time.Second))
			err = json.NewEncoder(c).Encode(map[string]string{"command": "status"})
			if err == nil {
				var r struct {
					Status struct {
						Version string `json:"version"`
					} `json:"status"`
					Error string `json:"error"`
				}
				err = json.NewDecoder(c).Decode(&r)
				if err == nil && r.Error == "" && strings.TrimPrefix(r.Status.Version, "v") == strings.TrimPrefix(version, "v") {
					c.Close()
					return nil
				}
				if err == nil {
					err = errors.New("installed service reported an unexpected version")
				}
			}
			c.Close()
		}
		last = err
		time.Sleep(time.Second)
	}
	return fmt.Errorf("windows service health check failed: %w", last)
}

func escapedXML(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
func registerRecovery(helper string) error {
	start := time.Now().Add(5 * time.Minute).UTC().Format("2006-01-02T15:04:05Z")
	doc := `<?xml version="1.0" encoding="UTF-8"?><Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Triggers><BootTrigger><Enabled>true</Enabled></BootTrigger><TimeTrigger><Repetition><Interval>PT5M</Interval><StopAtDurationEnd>false</StopAtDurationEnd></Repetition><StartBoundary>` + start + `</StartBoundary><Enabled>true</Enabled></TimeTrigger></Triggers><Principals><Principal id="System"><UserId>S-1-5-18</UserId><LogonType>ServiceAccount</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals><Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><ExecutionTimeLimit>PT0S</ExecutionTimeLimit></Settings><Actions Context="System"><Exec><Command>` + escapedXML(helper) + `</Command><Arguments>--recover</Arguments></Exec></Actions></Task>`
	file := filepath.Join(filepath.Dir(helper), "recovery-task.xml")
	if err := os.WriteFile(file, []byte(doc), 0600); err != nil {
		return err
	}
	out, err := exec.Command("schtasks.exe", "/Create", "/TN", RecoveryTask, "/XML", file, "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("register Windows update recovery: %w: %s", err, string(out))
	}
	return nil
}

func PrepareInstall(dir, install, owner, previous string, candidate *Candidate, m *Manifest, installer string) error {
	i, err := ReadInstallation(dir)
	if err != nil {
		return err
	}
	if !strings.EqualFold(i.Dir, install) || i.Owner != owner {
		return errors.New("update installation identity changed")
	}
	if err = VerifyFile(installer, m.Installer); err != nil {
		return err
	}
	stage := StageDir(dir, m)
	helper := filepath.Join(stage, "transcoderd-updater.exe")
	if err = CopyFile(filepath.Join(install, "transcoderd-updater.exe"), helper); err != nil {
		return err
	}
	t := &Transaction{Phase: "prepared", Candidate: *candidate, InstallDir: install, Owner: owner, PreviousVersion: previous, Started: time.Now()}
	if err = SaveJSON(TransactionPath(dir), t); err != nil {
		return err
	}
	if err = registerRecovery(helper); err != nil {
		t.Phase = "retrying"
		_ = SaveJSON(TransactionPath(dir), t)
		return err
	}
	cmd := exec.Command(helper, "--recover")
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if err = cmd.Start(); err != nil {
		t.Phase = "retrying"
		_ = SaveJSON(TransactionPath(dir), t)
		return err
	}
	return cmd.Process.Release()
}

func RecoverRunning(t *Transaction) error {
	if !t.Active() {
		return nil
	}
	if t.PID != 0 {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, t.PID)
		if err == nil {
			defer func() { _ = windows.CloseHandle(h) }()
			var code uint32
			if windows.GetExitCodeProcess(h, &code) == nil && code == 259 {
				var image [32768]uint16
				size := uint32(len(image))
				key, e := PublicKey()
				if e == nil {
					manifest, e := Verify(t.Candidate.Data, t.Candidate.Signature, key)
					if e == nil && windows.QueryFullProcessImageName(h, 0, &image[0], &size) == nil && strings.EqualFold(windows.UTF16ToString(image[:size]), filepath.Join(StageDir(DataDir(), manifest), "transcoderd-updater.exe")) {
						return nil
					}
				}
			}
		}
	}
	if time.Since(t.Started) < 30*time.Second {
		return nil
	}
	_, err := exec.Command("schtasks.exe", "/Run", "/TN", RecoveryTask).CombinedOutput()
	return err
}

func mutex() (windows.Handle, error) {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return 0, err
	}
	name, _ := windows.UTF16PtrFromString(`Global\TranscoderDInstallationUpdate`)
	h, err := windows.CreateMutex(&windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return 0, err
	}
	result, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		_ = windows.CloseHandle(h)
		return 0, err
	}
	if result != windows.WAIT_OBJECT_0 && result != windows.WAIT_ABANDONED {
		_ = windows.CloseHandle(h)
		return 0, errors.New("windows installation update is already running")
	}
	return h, nil
}

func relaunchOwner(install, owner, args string) error {
	var sessions *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &sessions, &count); err != nil {
		return err
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessions)))
	for _, session := range unsafe.Slice(sessions, count) {
		if session.State != windows.WTSActive {
			continue
		}
		var token windows.Token
		if windows.WTSQueryUserToken(session.SessionID, &token) != nil {
			continue
		}
		user, err := token.GetTokenUser()
		if err != nil || user.User.Sid.String() != owner {
			token.Close()
			continue
		}
		app, _ := windows.UTF16PtrFromString(filepath.Join(install, "transcoderd-tray.exe"))
		line, _ := windows.UTF16PtrFromString(`"` + filepath.Join(install, "transcoderd-tray.exe") + `" ` + args)
		desktop, _ := windows.UTF16PtrFromString(`winsta0\default`)
		si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Desktop: desktop}
		var pi windows.ProcessInformation
		var environment *uint16
		if err = windows.CreateEnvironmentBlock(&environment, token, false); err != nil {
			token.Close()
			return err
		}
		err = windows.CreateProcessAsUser(token, app, line, nil, nil, false, windows.CREATE_UNICODE_ENVIRONMENT, environment, nil, &si, &pi)
		_ = windows.DestroyEnvironmentBlock(environment)
		token.Close()
		if err != nil {
			return err
		}
		_ = windows.CloseHandle(pi.Process)
		_ = windows.CloseHandle(pi.Thread)
	}
	return nil
}

func stopTrays(install, owner string) error {
	_ = relaunchOwner(install, owner, "--quit")
	time.Sleep(2 * time.Second)
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "transcoderd-tray.exe") {
			continue
		}
		h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, entry.ProcessID)
		if e != nil {
			return e
		}
		var path [32768]uint16
		size := uint32(len(path))
		e = windows.QueryFullProcessImageName(h, 0, &path[0], &size)
		var token windows.Token
		identity := false
		if e == nil && windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token) == nil {
			user, e := token.GetTokenUser()
			identity = e == nil && user.User.Sid.String() == owner
			token.Close()
		}
		if identity && strings.EqualFold(windows.UTF16ToString(path[:size]), filepath.Join(install, "transcoderd-tray.exe")) {
			e = windows.TerminateProcess(h, 0)
			if e == nil {
				_, e = windows.WaitForSingleObject(h, 10000)
			}
		} else {
			e = nil
		}
		_ = windows.CloseHandle(h)
		if e != nil {
			return e
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	return nil
}

func CloseInstalledTrays() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("administrator privileges are required")
	}
	i, err := ReadInstallation(DataDir())
	if err != nil {
		return err
	}
	return stopTrays(i.Dir, i.Owner)
}

// RunHelper is invoked by the protected task/installer helper, never by IPC.
func RunHelper(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("windows update recovery requires administrator privileges")
	}
	dir := DataDir()
	if err := ProtectDir(filepath.Join(dir, "updates")); err != nil {
		return err
	}
	h, err := mutex()
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	defer func() { _ = windows.ReleaseMutex(h) }()
	t, err := LoadTransaction(dir)
	if err != nil {
		return err
	}
	if t == nil || !t.Active() {
		return nil
	}
	i, err := ReadInstallation(dir)
	if err != nil {
		return err
	}
	if i.Owner != t.Owner || !strings.EqualFold(i.Dir, t.InstallDir) {
		return errors.New("recovery installation identity mismatch")
	}
	key, err := PublicKey()
	if err != nil {
		return err
	}
	m, err := Verify(t.Candidate.Data, t.Candidate.Signature, key)
	if err != nil {
		return err
	}
	stage := StageDir(dir, m)
	backup := filepath.Join(stage, "backup")
	installer := filepath.Join(stage, m.Installer.Name)
	t.PID = uint32(os.Getpid())
	t.Started = time.Now()
	if t.Phase == "restart-required" {
		boot := time.Now().Add(-windows.DurationSinceBoot())
		delta := boot.Sub(t.BootTime)
		if delta < 0 {
			delta = -delta
		}
		if delta < 10*time.Second {
			return nil
		}
		if err = VerifyInstallation(t.InstallDir, m.Files); err == nil {
			err = serviceStart()
		}
		if err == nil {
			err = health(t.Owner, m.Version)
		}
		if err == nil {
			return completeInstallation(dir, t, m)
		}
		return recoverAfterError(ctx, dir, t, backup, err)
	}
	if t.Phase != "prepared" {
		return recoverInstallation(ctx, dir, t, backup)
	}
	// Interrupted installations must remain recoverable even when the staged
	// installer has disappeared or become corrupt. Only a fresh install needs it.
	if err = VerifyFile(installer, m.Installer); err != nil {
		return err
	}
	previousFiles, err := InstallationFiles(t.InstallDir)
	if err != nil {
		return err
	}
	var backupSize int64
	for _, file := range previousFiles {
		backupSize += file.Size
	}
	if err = AvailableSpace(stage, backupSize); err != nil {
		return err
	}
	if err = stopTrays(t.InstallDir, t.Owner); err != nil {
		return err
	}
	if err = CheckPath(backup); err != nil {
		return err
	}
	if err = os.RemoveAll(backup); err != nil {
		return err
	}
	if err = CopyTree(t.InstallDir, backup); err != nil {
		return err
	}
	t.BackupFiles, err = InstallationFiles(backup)
	if err != nil {
		return err
	}
	t.Phase = "installing"
	if err = SaveJSON(TransactionPath(dir), t); err != nil {
		return err
	}
	if err = serviceStop(); err != nil {
		return recoverAfterError(ctx, dir, t, backup, err)
	}
	log, err := os.OpenFile(filepath.Join(stage, "installer.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return recoverAfterError(ctx, dir, t, backup, err)
	}
	installCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	writeLog := func(b []byte, _ bool) { _, _ = log.Write(b) }
	// The shared command runner kills the complete Windows Job Object on a
	// timeout, so a stuck prerequisite cannot outlive Setup and lock rollback.
	_, err = command.NewCommand(installer, "/S").SetStdoutFunc(writeLog).SetStderrFunc(writeLog).RunWithContext(installCtx)
	_ = log.Close()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 3010 {
			t.Phase = "restart-required"
			t.PID = 0
			t.BootTime = time.Now().Add(-windows.DurationSinceBoot())
			return SaveJSON(TransactionPath(dir), t)
		}
		return recoverAfterError(ctx, dir, t, backup, fmt.Errorf("silent installer failed: %w", err))
	}
	t.Phase = "verifying"
	if err = SaveJSON(TransactionPath(dir), t); err != nil {
		return err
	}
	if err = VerifyInstallation(t.InstallDir, m.Files); err != nil {
		return recoverAfterError(ctx, dir, t, backup, err)
	}
	if err = serviceStart(); err != nil {
		return recoverAfterError(ctx, dir, t, backup, err)
	}
	if err = health(t.Owner, m.Version); err != nil {
		return recoverAfterError(ctx, dir, t, backup, err)
	}
	return completeInstallation(dir, t, m)
}

func completeInstallation(dir string, t *Transaction, m *Manifest) error {
	t.Phase = "completed"
	t.PID = 0
	if err := SaveJSON(TransactionPath(dir), t); err != nil {
		return err
	}
	if err := ClearCompleted(dir, m.Version); err != nil {
		return err
	}
	_ = relaunchOwner(t.InstallDir, t.Owner, "--background")
	_, _ = exec.Command("schtasks.exe", "/Delete", "/TN", RecoveryTask, "/F").CombinedOutput()
	return nil
}

func recoverAfterError(ctx context.Context, dir string, t *Transaction, backup string, cause error) error {
	t.Error = cause.Error()
	t.Phase = "recovering"
	if err := SaveJSON(TransactionPath(dir), t); err != nil {
		return err
	}
	return recoverInstallation(ctx, dir, t, backup)
}

func recoverInstallation(ctx context.Context, dir string, t *Transaction, backup string) error {
	t.Phase = "recovering"
	t.PID = uint32(os.Getpid())
	_ = SaveJSON(TransactionPath(dir), t)
	for attempt := 1; ; attempt++ {
		err := rollback(t, backup)
		if err == nil {
			s, e := Load(dir)
			if e != nil {
				return e
			}
			s.Pending = true
			cause := t.Error
			if cause == "" {
				cause = "An interrupted update was recovered."
			}
			s.Fail(errors.New(cause), time.Now())
			if e = Save(dir, s); e != nil {
				return e
			}
			t.Phase = "retrying"
			t.PID = 0
			if e = SaveJSON(TransactionPath(dir), t); e != nil {
				return e
			}
			_ = relaunchOwner(t.InstallDir, t.Owner, "--background")
			return nil
		}
		t.Error = err.Error()
		_ = SaveJSON(TransactionPath(dir), t)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(RetryDelay(attempt)):
		}
	}
}

func rollback(t *Transaction, backup string) error {
	if len(t.BackupFiles) == 0 {
		return errors.New("rollback snapshot is unavailable")
	}
	if err := serviceStop(); err != nil {
		return err
	}
	if err := stopTrays(t.InstallDir, t.Owner); err != nil {
		return err
	}
	if err := RestoreTree(backup, t.InstallDir, t.BackupFiles); err != nil {
		return err
	}
	if err := installationReadAccess(t.InstallDir); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	if old, e := m.OpenService(ServiceName); e == nil {
		err = old.Delete()
		old.Close()
		if err != nil {
			_ = m.Disconnect()
			return err
		}
	}
	_ = m.Disconnect()
	cmd := exec.Command(filepath.Join(t.InstallDir, "transcoderd-desktop-service.exe"), "--install-owner", t.Owner)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restore Windows service: %w: %s", err, out)
	}
	return health(t.Owner, t.PreviousVersion)
}

func installationReadAccess(dir string) error {
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;BU)")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return filepath.WalkDir(dir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	})
}

// WriteLog records helper failures even when the service is temporarily absent.
func WriteLog(err error) {
	if err == nil {
		return
	}
	path := filepath.Join(DataDir(), "updates", "recovery.log")
	if ProtectDir(filepath.Dir(path)) != nil {
		return
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e == nil {
		defer f.Close()
		_, _ = io.WriteString(f, time.Now().UTC().Format(time.RFC3339)+" "+err.Error()+"\n")
	}
}
