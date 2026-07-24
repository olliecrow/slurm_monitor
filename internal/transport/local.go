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
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	result := RunResult{
		Stdout: outBuf.String(),
	}
	if err == nil {
		return result, nil
	}

	runErr := &RunError{
		Target: t.Describe(),
		Stderr: errBuf.String(),
		Err:    err,
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
