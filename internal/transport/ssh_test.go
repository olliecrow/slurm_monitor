package transport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
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
	tests := []string{
		"Permission denied (publickey)",
		"Can't open user config file /path/to/config: No such file or directory",
	}
	for _, stderr := range tests {
		err := &RunError{
			Stderr:   stderr,
			ExitCode: 255,
			Err:      errors.New("exit status 255"),
		}
		if IsRetryable(err) {
			t.Fatalf("expected permanent ssh failure %q to be non-retryable", stderr)
		}
	}
}

func TestRunErrorDoesNotRepeatKnownExitStatus(t *testing.T) {
	err := &RunError{
		Target:   "ssh:cluster_alias",
		Stderr:   "Permission denied (publickey)",
		ExitCode: 255,
		Err:      errors.New("exit status 255"),
	}
	got := err.Error()
	if !strings.Contains(got, "[exit=255]") {
		t.Fatalf("expected structured exit code, got %q", got)
	}
	if strings.Contains(got, "exit status 255") {
		t.Fatalf("expected exit status not to be repeated, got %q", got)
	}
}

func TestRetryableControllerAndResolverFailures(t *testing.T) {
	for _, text := range []string{
		"ssh: Could not resolve hostname host: Temporary failure in name resolution",
		"slurm_load_node error: Unable to contact slurm controller (connect failure)",
		"slurm_load_jobs error: Zero Bytes were transmitted or received",
		"slurm_load_jobs error: Slurm backup controller in standby mode",
		"slurm_load_jobs error: Socket timed out on send/recv operation",
		"Resource temporarily unavailable",
	} {
		if !IsRetryable(&RunError{Stderr: text, ExitCode: 1}) {
			t.Errorf("not retryable: %s", text)
		}
	}
	if IsRetryable(&RunError{Stderr: "ssh: Could not resolve hostname host: Name or service not known", ExitCode: 255}) {
		t.Fatal("invalid startup host should fail")
	}
}

func TestControlDirSafety(t *testing.T) {
	for _, kind := range []string{"new", "public", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			t.Setenv("XDG_RUNTIME_DIR", base)
			dir := filepath.Join(base, fmt.Sprintf("slurm-monitor-%d", os.Getuid()))
			if kind == "public" {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "symlink" {
				if err := os.Symlink(t.TempDir(), dir); err != nil {
					t.Fatal(err)
				}
			}
			got, err := controlDir()
			if kind == "new" {
				if err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(got)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0o700 {
					t.Fatal(info.Mode())
				}
			} else {
				if err == nil {
					t.Fatal("accepted unsafe directory")
				}
				args := strings.Join(NewSSHTransport(SSHOptions{Target: "host"}).buildSSHArgs("true"), " ")
				for _, option := range []string{"ControlPath=", "ControlMaster=", "ControlPersist="} {
					if strings.Contains(args, option) {
						t.Fatalf("unsafe multiplexing: %s", args)
					}
				}
			}
		})
	}
}

func TestControlSocketPathLength(t *testing.T) {
	path := filepath.Join(strings.Repeat("x", 48), "slurm-monitor-501", "cm-0123456789abcdef")
	if len(path)+17 >= 104 {
		t.Fatalf("socket path too long: %d", len(path)+17)
	}
}
