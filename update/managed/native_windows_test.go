package managed

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestInstallationMutexExcludesAnotherThread(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Skip("machine installation mutex is tested only on an isolated runner")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	handle, err := mutex()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	defer func() { _ = windows.ReleaseMutex(handle) }()
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		other, err := mutex()
		if err == nil {
			_ = windows.ReleaseMutex(other)
			_ = windows.CloseHandle(other)
		}
		done <- err
	}()
	if err = <-done; err == nil {
		t.Fatal("another thread acquired the installation mutex")
	}
}

func TestProtectedUpdateFilesHavePrivilegedOwner(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("protected update files require an elevated isolated runner")
	}
	dir := t.TempDir()
	identity, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if err = SaveInstallation(dir, Installation{Dir: dir, Owner: identity.User.Sid.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadInstallation(dir); err != nil {
		t.Fatal(err)
	}
	if err = Save(dir, State{Status: Status{Pending: true, Phase: "pending"}}); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(StatePath(dir), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner.String() != "S-1-5-32-544" {
		t.Fatalf("journal owner is not the administrators group: %v", err)
	}
}

func TestWorkspaceRemainsOutsideInstallation(t *testing.T) {
	for _, tt := range []struct {
		name, workspace string
		allowed         bool
	}{
		{"default", "", true},
		{"another drive", `D:\Encoding`, true},
		{"sibling", `C:\Program Files\TranscoderD-other\work`, true},
		{"installation root", `C:\Program Files\TranscoderD`, false},
		{"nested", `C:\Program Files\TranscoderD\work`, false},
		{"case insensitive", `c:\PROGRAM FILES\transcoderd\work`, false},
		{"normalized path", `C:\Program Files\other\..\TranscoderD\work`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkspace(`C:\Program Files\TranscoderD`, tt.workspace)
			if (err == nil) != tt.allowed {
				t.Fatalf("ValidateWorkspace() = %v, allowed %v", err, tt.allowed)
			}
		})
	}
}

func TestPreparedFailurePreservesInstallationAndSchedulesRetry(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" || !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("privileged helper tests require an isolated runner")
	}
	t.Setenv("ProgramData", t.TempDir())
	manifest, pub, key, _ := fixtureManifest(t)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	candidate := Candidate{Data: data, Signature: ed25519.Sign(key, data)}
	originalMode, originalKey := FixtureMode, FixtureKey
	FixtureMode, FixtureKey = "true", base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { FixtureMode, FixtureKey = originalMode, originalKey })
	install := t.TempDir()
	sentinel := filepath.Join(install, "original.txt")
	if err = os.WriteFile(sentinel, []byte("preserve me"), 0600); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	owner := user.User.Sid.String()
	dir := DataDir()
	if err = SaveInstallation(dir, Installation{Dir: install, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	if err = Save(dir, State{Status: Status{Pending: true, Phase: "pending"}, Candidate: &candidate}); err != nil {
		t.Fatal(err)
	}
	if err = SaveJSON(TransactionPath(dir), &Transaction{Phase: "prepared", Candidate: candidate, InstallDir: install, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	if err = RunHelper(context.Background()); err == nil {
		t.Fatal("helper accepted a missing installer")
	}
	state, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := LoadTransaction(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Pending || state.Phase != "retrying" || state.NextRetry.IsZero() || transaction.Phase != "retrying" {
		t.Fatalf("prepared failure did not retain a bounded retry: %+v, %+v", state, transaction)
	}
	if contents, err := os.ReadFile(sentinel); err != nil || string(contents) != "preserve me" {
		t.Fatalf("prepared failure changed original files: %s, %v", contents, err)
	}
}

func TestRecoveryTaskXMLRegistersWithWindowsScheduler(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" || !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("SYSTEM recovery task registration requires an isolated runner")
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(t.TempDir(), "recovery & unicode é.exe")
	if err = CopyFile(source, helper); err != nil {
		t.Fatal(err)
	}
	task := fmt.Sprintf("TranscoderD-XML-Test-%d", time.Now().UnixNano())
	defer func() { _, _ = exec.Command("schtasks.exe", "/Delete", "/TN", task, "/F").CombinedOutput() }()
	if err = registerRecoveryTask(helper, task); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("schtasks.exe", "/Query", "/TN", task).CombinedOutput(); err != nil {
		t.Fatalf("registered task is unavailable: %v %s", err, output)
	}
}
