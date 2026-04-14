package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"slurm_monitor/internal/slurm"
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
			{err: &transport.RunError{Stderr: "Connection timed out", ExitCode: 255, Err: errors.New("exit status 255")}},
			{err: &transport.RunError{Stderr: "Connection timed out", ExitCode: 255, Err: errors.New("exit status 255")}},
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
			{err: &transport.RunError{Stderr: "Connection timed out", ExitCode: 255, Err: errors.New("exit status 255")}},
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

func TestAwaitSlurmAvailabilityStopsOnPermanentTransportFailure(t *testing.T) {
	tr := &scriptedTransport{
		responses: []transportResponse{
			{err: &transport.RunError{Stderr: "Permission denied (publickey)", ExitCode: 255, Err: errors.New("exit status 255")}},
			{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := awaitSlurmAvailabilityWithBackoff(ctx, tr, 50*time.Millisecond, 5*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected permanent failure")
	}
	if tr.calls != 1 {
		t.Fatalf("expected no retries for permanent failure, got %d calls", tr.calls)
	}
}

func TestRunOncePrintsQueueAndUserCPUAndGPUSplit(t *testing.T) {
	raw := strings.Join([]string{
		"NodeName=node001 State=IDLE CPUTot=64 CPUAlloc=32 CPULoad=16.00 RealMemory=256000 AllocMem=128000 FreeMem=96000 Partitions=main CfgTRES=cpu=64,mem=256000M,billing=64,gres/gpu=4 AllocTRES=cpu=32,mem=128000M,billing=32,gres/gpu=2",
		"__SLURM_MONITOR_SPLIT__",
		"1001|RUNNING|alice|8|20G|gres/gpu:1|train|jobA|None",
		"1002|PENDING|alice|4|10G|N/A|train|jobB|Priority",
	}, "\n")
	collector := slurm.NewCollector(fakeTransport{
		result: transport.RunResult{Stdout: raw},
	}, 2*time.Second)

	out := captureStdout(t, func() {
		if err := runOnce(context.Background(), collector, "fake"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	if !strings.Contains(out, "queue: running_cpu=0 running_gpu=1 pending_cpu=1 pending_gpu=0 other=0 total=2") {
		t.Fatalf("expected queue cpu/gpu split in output, got: %q", out)
	}
	if !strings.Contains(out, "alice running_cpu_jobs=0 running_gpu_jobs=1 pending_cpu_jobs=1 pending_gpu_jobs=0") {
		t.Fatalf("expected user cpu/gpu split in output, got: %q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = orig
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close write pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close read pipe: %v", err)
	}
	return string(out)
}
