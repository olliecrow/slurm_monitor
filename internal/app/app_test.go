package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"slurm_monitor/internal/transport"
)

type fakeTransport struct {
	result transport.RunResult
	err    error
}

func (f fakeTransport) Run(context.Context, string) (transport.RunResult, error) {
	return f.result, f.err
}

func (f fakeTransport) Describe() string {
	return "fake"
}

type scriptedTransport struct {
	calls     int
	responses []transportResponse
}

type transportResponse struct {
	result transport.RunResult
	err    error
}

func (s *scriptedTransport) Run(context.Context, string) (transport.RunResult, error) {
	idx := s.calls
	s.calls++
	if len(s.responses) == 0 {
		return transport.RunResult{}, nil
	}
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	r := s.responses[idx]
	return r.result, r.err
}

func (s *scriptedTransport) Describe() string {
	return "scripted"
}

func TestCheckSlurmAvailabilityMissingCommands(t *testing.T) {
	tr := fakeTransport{
		result: transport.RunResult{Stdout: " sinfo scontrol"},
		err:    errors.New("exit 7"),
	}
	err := checkSlurmAvailability(context.Background(), tr, 2*time.Second)
	if err == nil {
		t.Fatalf("expected error")
	}
	var missingErr *missingSlurmCommandsError
	if !errors.As(err, &missingErr) {
		t.Fatalf("expected missingSlurmCommandsError, got %T: %v", err, err)
	}
}

func TestCheckSlurmAvailabilityPasses(t *testing.T) {
	tr := fakeTransport{}
	err := checkSlurmAvailability(context.Background(), tr, 2*time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestAwaitSlurmAvailabilityRetriesThenPasses(t *testing.T) {
	tr := &scriptedTransport{
		responses: []transportResponse{
			{err: errors.New("temporary ssh failure")},
			{err: errors.New("temporary ssh failure")},
			{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := awaitSlurmAvailabilityWithBackoff(ctx, tr, 50*time.Millisecond, 5*time.Millisecond, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if tr.calls < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", tr.calls)
	}
}

func TestAwaitSlurmAvailabilityStopsOnMissingCommands(t *testing.T) {
	tr := &scriptedTransport{
		responses: []transportResponse{
			{
				result: transport.RunResult{Stdout: " sinfo scontrol"},
				err:    errors.New("exit 7"),
			},
			{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := awaitSlurmAvailabilityWithBackoff(ctx, tr, 50*time.Millisecond, 5*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected missing-command error")
	}
	if tr.calls != 1 {
		t.Fatalf("expected no retries for missing commands, got %d calls", tr.calls)
	}
}

func TestIsMissingSlurmCommandError(t *testing.T) {
	if isMissingSlurmCommandError(nil) {
		t.Fatalf("expected false for nil error")
	}
	if isMissingSlurmCommandError(errors.New("missing required Slurm commands on fake: sinfo")) {
		t.Fatalf("expected false for plain string error")
	}
	err := &missingSlurmCommandsError{source: "fake", missing: "sinfo"}
	if !isMissingSlurmCommandError(err) {
		t.Fatalf("expected true for missingSlurmCommandsError")
	}
}

func TestAwaitSlurmAvailabilityHonorsContextCancellation(t *testing.T) {
	tr := &scriptedTransport{
		responses: []transportResponse{
			{err: errors.New("temporary ssh failure")},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	err := awaitSlurmAvailabilityWithBackoff(ctx, tr, 20*time.Millisecond, 10*time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if tr.calls < 2 {
		t.Fatalf("expected retries before context cancellation, got %d calls", tr.calls)
	}
}
