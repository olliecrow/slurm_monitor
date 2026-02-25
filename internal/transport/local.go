package transport

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

type LocalTransport struct{}

func NewLocalTransport() *LocalTransport {
	return &LocalTransport{}
}

func (t *LocalTransport) Describe() string {
	return "local"
}

func (t *LocalTransport) Run(ctx context.Context, command string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
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
