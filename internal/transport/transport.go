package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Transport interface {
	Run(ctx context.Context, command string) (RunResult, error)
	Describe() string
}

type RunError struct {
	Command  string
	Target   string
	Stdout   string
	Stderr   string
	ExitCode int
	Timeout  bool
	Err      error
}

func (e *RunError) Error() string {
	base := fmt.Sprintf("command failed on %s", e.Target)
	if e.Timeout {
		base += " (timeout)"
	}
	if e.ExitCode != 0 {
		base += fmt.Sprintf(" [exit=%d]", e.ExitCode)
	}
	if s := strings.TrimSpace(e.Stderr); s != "" {
		base += ": " + s
	}
	if e.Err != nil {
		base += fmt.Sprintf(": %v", e.Err)
	}
	return base
}

func (e *RunError) Unwrap() error {
	return e.Err
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
		return true
	}

	var runErr *RunError
	if errors.As(err, &runErr) {
		if runErr.Timeout {
			return true
		}
		if runErr.ExitCode == 255 {
			return true
		}

		stderr := strings.ToLower(runErr.Stderr)
		retrySignals := []string{
			"connection reset",
			"broken pipe",
			"connection timed out",
			"operation timed out",
			"timed out",
			"network is unreachable",
			"temporary failure",
			"connection closed",
			"no route to host",
			"connection refused",
		}
		for _, signal := range retrySignals {
			if strings.Contains(stderr, signal) {
				return true
			}
		}
	}

	return false
}
