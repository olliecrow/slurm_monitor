package transport

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SSHOptions struct {
	Target         string
	ConfigPath     string
	IdentityFile   string
	Port           int
	ConnectTimeout time.Duration
}

type SSHTransport struct {
	opts        SSHOptions
	controlPath string
}

func NewSSHTransport(opts SSHOptions) *SSHTransport {
	return &SSHTransport{
		opts:        opts,
		controlPath: buildControlPath(opts),
	}
}

func (t *SSHTransport) Describe() string {
	return "ssh:" + t.opts.Target
}

func (t *SSHTransport) Run(ctx context.Context, command string) (RunResult, error) {
	args := t.buildSSHArgs(command)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	result := RunResult{
		Stdout: outBuf.String(),
		Stderr: errBuf.String(),
	}
	if err == nil {
		return result, nil
	}

	runErr := &RunError{
		Command: command,
		Target:  t.Describe(),
		Stdout:  result.Stdout,
		Stderr:  result.Stderr,
		Err:     err,
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		runErr.ExitCode = exitErr.ExitCode()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		runErr.Timeout = true
	}

	return result, runErr
}

func (t *SSHTransport) buildSSHArgs(command string) []string {
	args := make([]string, 0, 24)
	if t.opts.ConnectTimeout > 0 {
		seconds := int(math.Ceil(t.opts.ConnectTimeout.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		args = append(args, "-o", fmt.Sprintf("ConnectTimeout=%d", seconds))
	}
	args = append(args,
		"-o", "ConnectionAttempts=2",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		"-o", "TCPKeepAlive=yes",
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=300",
		"-o", "StreamLocalBindUnlink=yes",
	)
	if t.controlPath != "" {
		args = append(args, "-o", "ControlPath="+t.controlPath)
	}

	if t.opts.ConfigPath != "" {
		args = append(args, "-F", t.opts.ConfigPath)
	}
	if t.opts.IdentityFile != "" {
		args = append(args, "-i", t.opts.IdentityFile)
	}
	if t.opts.Port > 0 {
		args = append(args, "-p", strconv.Itoa(t.opts.Port))
	}

	remoteCommand := "bash -lc " + shellQuote(command)
	args = append(args, t.opts.Target, remoteCommand)
	return args
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func buildControlPath(opts SSHOptions) string {
	base := fmt.Sprintf("%s|%s|%s|%d", opts.Target, opts.ConfigPath, opts.IdentityFile, opts.Port)
	sum := sha1.Sum([]byte(base))
	id := hex.EncodeToString(sum[:8])
	root := filepath.Join(os.TempDir(), "slurm-monitor-ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return ""
	}
	return filepath.Join(root, "cm-"+id)
}
