package transport

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type LocalTransport struct{}

func NewLocalTransport() *LocalTransport   { return &LocalTransport{} }
func (t *LocalTransport) Describe() string { return "local" }
func (t *LocalTransport) Run(ctx context.Context, command string) (RunResult, error) {
	return runPrepared(ctx, t.Describe(), killLocalProcessGroup, "sh", "-lc", command)
}

// killLocalProcessGroup puts the local shell in its own process group and
// kills that group on cancel. SSH stays in the foreground group so it can
// read a terminal prompt.
func killLocalProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil || cmd.Process.Pid <= 0 {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
