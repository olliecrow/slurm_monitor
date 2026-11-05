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

func useShortRuntimeDir(t *testing.T) {
	t.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("sm%d-%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
}

func TestBuildControlPath(t *testing.T) {
	useShortRuntimeDir(t)
	path, state := buildControlPath(SSHOptions{
		Target:       "host-a",
		ConfigPath:   "/tmp/cfg",
		IdentityFile: "/tmp/key",
		Port:         22,
	})
	if path == "" || state != muxOn {
		t.Fatalf("expected non-empty control path")
	}
	path2, state2 := buildControlPath(SSHOptions{
		Target:       "host-a",
		ConfigPath:   "/tmp/cfg",
		IdentityFile: "/tmp/key",
		Port:         22,
	})
	if path != path2 || state2 != muxOn {
		t.Fatalf("expected deterministic control path, got %q vs %q", path, path2)
	}
}

func TestBuildSSHArgsIncludesResilienceOptions(t *testing.T) {
	useShortRuntimeDir(t)
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
		"-- user@host",
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
		"Received disconnect from 192.0.2.1 port 22:2: Too many authentication failures",
		"Bad owner or permissions on /x/.ssh/config",
		"Unable to negotiate with 192.0.2.1 port 22: no matching host key type found.",
		"ControlPath too long ('/tmp/socket' >= 108 bytes)",
		"path \"/tmp/socket\" too long for Unix domain socket",
		"Host key verification failed.",
		"WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!",
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
		"slurm_load_jobs error: Controller is in standby mode, try a different controller",
		"slurm_load_jobs error: Communication connection failure",
		"slurm_load_jobs error: Message send failure",
		"slurm_load_jobs error: Message receive failure",
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
			dir := filepath.Join(base, fmt.Sprintf("slurm-mon-%d", os.Getuid()))
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

func TestLongTempDirDisablesControlPath(t *testing.T) {
	base := filepath.Join(t.TempDir(), strings.Repeat("r", 90))
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", base)
	tr := NewSSHTransport(SSHOptions{Target: "host.example"})
	if tr.controlPath != "" {
		t.Fatalf("expected multiplexing to be disabled, path length %d", len(tr.controlPath))
	}
	if !strings.Contains(tr.MultiplexingStatus(), "104") {
		t.Fatalf("status %q", tr.MultiplexingStatus())
	}
	if notice := tr.multiplexingNotice(); !strings.Contains(notice, "104-byte") {
		t.Fatalf("notice %q", notice)
	}
	joined := strings.Join(tr.buildSSHArgs("true"), "\n")
	if strings.Contains(joined, "Control") {
		t.Fatalf("control options present:\n%s", joined)
	}
}

func TestControlPathUsesShortNames(t *testing.T) {
	useShortRuntimeDir(t)
	tr := NewSSHTransport(SSHOptions{Target: "host.example", Port: 22})
	path := tr.controlPath
	if path == "" {
		t.Fatal("expected a control path")
	}
	if filepath.Base(filepath.Dir(path)) != fmt.Sprintf("slurm-mon-%d", os.Getuid()) {
		t.Fatalf("directory %q", path)
	}
	if len(filepath.Base(path)) != 16 {
		t.Fatalf("socket name %q", filepath.Base(path))
	}
	if len(path)+17 >= 104 {
		t.Fatalf("socket path too long: %d", len(path)+17)
	}
}

func TestMissingRuntimeDirFallsBack(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-runtime")
	fallback := filepath.Join(os.TempDir(), fmt.Sprintf("slurm-mon-%d", os.Getuid()))
	_, statErr := os.Lstat(fallback)
	t.Cleanup(func() {
		if errors.Is(statErr, os.ErrNotExist) {
			_ = os.Remove(fallback)
		}
	})
	t.Setenv("XDG_RUNTIME_DIR", missing)
	dir, err := controlDir()
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(dir, missing) {
		t.Fatalf("used missing runtime dir %s", dir)
	}
	if filepath.Base(dir) != filepath.Base(fallback) {
		t.Fatalf("dir %s", dir)
	}
}

func TestRetryClassificationUsesLastLine(t *testing.T) {
	profileThenRefused := "/etc/profile.d/x.sh: Permission denied\nslurm_load_jobs error: Connection refused"
	if !IsRetryable(&RunError{Stderr: profileThenRefused, ExitCode: 1}) {
		t.Fatal("expected the last transient line to retry")
	}
	noiseThenPermanent := "motd: temporary failure in name resolution\nsqueue: invalid option -- 'O'"
	if IsRetryable(&RunError{Stderr: noiseThenPermanent, ExitCode: 1}) {
		t.Fatal("expected the last permanent line to stop")
	}
	noiseThenAuth := "Connection refused\nReceived disconnect from 192.0.2.1 port 22:2: Too many authentication failures"
	if IsRetryable(&RunError{Stderr: noiseThenAuth, ExitCode: 255}) {
		t.Fatal("expected the last authentication failure to stop")
	}
}

func TestErrorUsesLastSanitizedLine(t *testing.T) {
	err := &RunError{Target: "ssh:host", ExitCode: 255, Stderr: "Warning\nDetails\n\x1b]0;title\a\x1b[2JHost key verification failed.\n\n"}
	if got := err.Error(); got != "command failed on ssh:host [exit=255]: Host key verification failed." {
		t.Fatalf("unexpected error: %q", got)
	}
	if got := CleanText("a\x1b]52;c;ignored\ab\x1b[31mc\t\r\x00"); got != "abc" {
		t.Fatalf("unsafe text: %q", got)
	}
}
