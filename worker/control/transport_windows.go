package control

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// The default is expanded with the current user's SID before opening the pipe.
const SocketPath = `\\.\pipe\transcoderd-worker`

func pipeIdentity(path string) (string, string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", "", fmt.Errorf("get control pipe user: %w", err)
	}
	sid := user.User.Sid.String()
	if !strings.HasPrefix(path, `\\.\pipe\`) {
		path = fmt.Sprintf(`\\.\pipe\transcoderd-worker-%x`, sha256.Sum256([]byte(path)))
	}
	return path + "-" + sid, "D:P(A;;GA;;;SY)(A;;GA;;;" + sid + ")", nil
}

func listenControl(path string) (net.Listener, error) {
	name, security, err := pipeIdentity(path)
	if err != nil {
		return nil, err
	}
	return winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: security})
}

func dialControl(path string) (net.Conn, error) {
	name, _, err := pipeIdentity(path)
	if err != nil {
		return nil, err
	}
	timeout := 2 * time.Second
	return winio.DialPipe(name, &timeout)
}

func closeControl(listener net.Listener) error { return listener.Close() }
