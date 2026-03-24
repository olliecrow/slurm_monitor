package transport

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestShellQuote(t *testing.T) {
	got := shellQuote("echo 'hello world'")
	want := `'echo '"'"'hello world'"'"''`
	if got != want {
		t.Fatalf("unexpected quote output\nwant: %s\ngot:  %s", want, got)
	}
}

func TestBuildControlPath(t *testing.T) {
	path := buildControlPath(SSHOptions{
		Target:       "host-a",
		ConfigPath:   "/tmp/cfg",
		IdentityFile: "/tmp/key",
		Port:         22,
	})
	if path == "" {
		t.Fatalf("expected non-empty control path")
	}
	path2 := buildControlPath(SSHOptions{
		Target:       "host-a",
		ConfigPath:   "/tmp/cfg",
		IdentityFile: "/tmp/key",
		Port:         22,
	})
	if path != path2 {
		t.Fatalf("expected deterministic control path, got %q vs %q", path, path2)
	}
}

func TestBuildSSHArgsIncludesResilienceOptions(t *testing.T) {
	tr := NewSSHTransport(SSHOptions{
		Target:         "user@host",
		ConfigPath:     "/tmp/ssh_config",
		IdentityFile:   "/tmp/id",
		Port:           2222,
		ConnectTimeout: 1500 * time.Millisecond,
	})
	args := tr.buildSSHArgs("echo hello")
	joined := strings.Join(args, " ")

	required := []string{
		"ConnectTimeout=2",
		"ConnectionAttempts=2",
		"ServerAliveInterval=15",
		"ServerAliveCountMax=3",
		"TCPKeepAlive=yes",
		"ControlMaster=auto",
		"ControlPersist=300",
		"StreamLocalBindUnlink=yes",
		"ControlPath=",
		"-F /tmp/ssh_config",
		"-i /tmp/id",
		"-p 2222",
		"user@host",
		"sh -lc 'echo hello'",
	}
	for _, token := range required {
		if !strings.Contains(joined, token) {
			t.Fatalf("expected token %q in args: %s", token, joined)
		}
	}
}

func TestIsRetryableRecognizesTransientSSHFailures(t *testing.T) {
	err := &RunError{
		Stderr:   "Connection timed out",
		ExitCode: 255,
		Err:      errors.New("exit status 255"),
	}
	if !IsRetryable(err) {
		t.Fatalf("expected transient ssh timeout to be retryable")
	}
}

func TestIsRetryableRejectsPermanentSSHFailures(t *testing.T) {
	err := &RunError{
		Stderr:   "Permission denied (publickey)",
		ExitCode: 255,
		Err:      errors.New("exit status 255"),
	}
	if IsRetryable(err) {
		t.Fatalf("expected auth failure to be non-retryable")
	}
}
