package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/olliecrow/slurm_monitor/internal/config"
	"github.com/olliecrow/slurm_monitor/internal/slurm"
	"github.com/olliecrow/slurm_monitor/internal/transport"
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
		result: transport.RunResult{Stdout: "login banner\n__SLURM_MONITOR_MISSING__ squeue scontrol\n"},
		err:    &transport.RunError{ExitCode: 7, Err: errors.New("exit status 7")},
	}
	err := checkSlurmAvailability(context.Background(), tr, 2*time.Second)
	if err == nil {
		t.Fatalf("expected error")
	}
	var missingErr *missingSlurmCommandsError
	if !errors.As(err, &missingErr) {
		t.Fatalf("expected missingSlurmCommandsError, got %T: %v", err, err)
	}
	if missingErr.missing != "squeue scontrol" {
		t.Fatalf("unexpected missing-command list %q", missingErr.missing)
	}
}

func TestCheckSlurmAvailabilityDoesNotTreatFailureOutputAsMissingCommands(t *testing.T) {
	tr := fakeTransport{
		result: transport.RunResult{Stdout: "remote login banner"},
		err: &transport.RunError{
			Stderr:   "Permission denied (publickey)",
			ExitCode: 255,
			Err:      errors.New("exit status 255"),
		},
	}

	err := checkSlurmAvailability(context.Background(), tr, 2*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	var missingErr *missingSlurmCommandsError
	if errors.As(err, &missingErr) {
		t.Fatalf("expected transport failure, got missing-command error: %v", err)
	}
	if strings.Contains(err.Error(), "remote login banner") {
		t.Fatalf("transport error exposed stdout: %v", err)
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
				result: transport.RunResult{Stdout: "__SLURM_MONITOR_MISSING__ squeue scontrol"},
				err:    &transport.RunError{ExitCode: 7, Err: errors.New("exit status 7")},
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
	if isMissingSlurmCommandError(errors.New("missing required Slurm commands on fake: squeue")) {
		t.Fatalf("expected false for plain string error")
	}
	err := &missingSlurmCommandsError{source: "fake", missing: "squeue"}
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

func TestMonitorDurationExpiryIsCleanOnlyForInteractiveMonitor(t *testing.T) {
	expiredCtx, cancelExpired := context.WithTimeout(context.Background(), 0)
	defer cancelExpired()
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		cfg  config.Config
		ctx  context.Context
		want bool
	}{
		{name: "interactive duration", cfg: config.Config{Duration: time.Second}, ctx: expiredCtx, want: true},
		{name: "one-shot duration", cfg: config.Config{Duration: time.Second, Once: true}, ctx: expiredCtx, want: false},
		{name: "no configured duration", cfg: config.Config{}, ctx: expiredCtx, want: false},
		{name: "operator cancellation", cfg: config.Config{Duration: time.Second}, ctx: canceledCtx, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := monitorDurationExpired(test.cfg, test.ctx); got != test.want {
				t.Fatalf("monitorDurationExpired()=%t want %t", got, test.want)
			}
		})
	}
}

func TestRunOncePrintsQueueAndUserCPUAndGPUSplit(t *testing.T) {
	raw := strings.Join([]string{
		"NodeName=node-a CPUAlloc=24 CPUEfctv=32 CPUTot=32 State=MIXED CfgTRES=cpu=32,gres/gpu=4 AllocTRES=cpu=24,gres/gpu=1",
		"__SLURM_MONITOR_SPLIT__",
		"1001|RUNNING|alice|gpu|8|20G|cpu=8,mem=20G,gres/gpu=1|None",
		"1002|PENDING|alice|cpu|4|10G|cpu=4,mem=10G|Priority",
	}, "\n")
	collector := slurm.NewCollector(fakeTransport{
		result: transport.RunResult{Stdout: raw},
	}, 2*time.Second)

	out := captureStdout(t, func() {
		if err := runOnce(context.Background(), collector, "fake"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	if !strings.Contains(out, "queue_jobs: running_cpu=0 running_gpu=1 pending_cpu=1 pending_gpu=0 other=0 total=2") {
		t.Fatalf("expected queue cpu/gpu split in output, got: %q", out)
	}
	if !strings.Contains(out, "queue_resources: running_cpu=8 running_gpu=1 pending_cpu=4 pending_gpu=0") {
		t.Fatalf("expected queue resource totals in output, got: %q", out)
	}
	if !strings.Contains(out, "available_resources: cpu=8 gpu=3 schedulable_nodes=1 total_nodes=1") {
		t.Fatalf("expected available resource totals in output, got: %q", out)
	}
	if !strings.Contains(out, "alice running_cpu=8 running_gpu=1 running_cpu_jobs=0 running_gpu_jobs=1 pending_cpu_jobs=1 pending_gpu_jobs=0 pending_cpu=4 pending_gpu=0") {
		t.Fatalf("expected user resource and cpu/gpu job split in output, got: %q", out)
	}
	if !strings.Contains(out, "gpu running_cpu_jobs=0 running_gpu_jobs=1") || !strings.Contains(out, "cpu running_cpu_jobs=0 running_gpu_jobs=0 pending_cpu_jobs=1") {
		t.Fatalf("expected partition job counts in output, got: %q", out)
	}
	if !strings.Contains(out, `1001 user=alice partition=gpu state=RUNNING reason="" tasks=1 cpu=8 gpu=1`) {
		t.Fatalf("expected grouped job rows in output, got: %q", out)
	}
	if !strings.Contains(out, `pending_reasons: shown=1 total=1`) || !strings.Contains(out, `reason="Priority" tasks=1 cpu=4 gpu=0`) {
		t.Fatalf("expected pending-reason totals in output, got: %q", out)
	}
	if strings.Contains(out, "nodes:") || strings.Contains(out, "totals:") {
		t.Fatalf("did not expect removed node output, got: %q", out)
	}
	if !strings.Contains(out, "partitions: shown=2 total=2") || !strings.Contains(out, "users: shown=1 total=1") || !strings.Contains(out, "jobs: shown=2 total=2 grouping=root+user+partition+state+pending_reason") {
		t.Fatalf("expected shown/total counts and the job grouping key, got: %q", out)
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
