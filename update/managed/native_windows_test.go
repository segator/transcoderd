package managed

import (
	"os"
	"runtime"
	"testing"

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
