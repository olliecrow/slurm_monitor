package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/olliecrow/slurm_monitor/internal/config"
	"github.com/olliecrow/slurm_monitor/internal/monitor"
	"github.com/olliecrow/slurm_monitor/internal/slurm"
	"github.com/olliecrow/slurm_monitor/internal/transport"
	"github.com/olliecrow/slurm_monitor/internal/tui"
)

// missingSlurmCommandsError is typed so retry classification is stable and
// does not depend on brittle string matching.
type missingSlurmCommandsError struct {
	source  string
	missing string
}

func (e *missingSlurmCommandsError) Error() string {
	return fmt.Sprintf("missing required Slurm commands on %s: %s", e.source, e.missing)
}

func Run(cfg config.Config) error {
	switch cfg.Command {
	case config.CommandDoctor:
		return RunDoctor(cfg, os.Stdout)
	case config.CommandDryRun:
		return RunDryRun(cfg, os.Stdout)
	case config.CommandMonitor:
		// Continue into monitor execution.
	default:
		return fmt.Errorf("unsupported command: %s", cfg.Command)
	}

	tr, err := buildTransport(cfg)
	if err != nil {
		return err
	}

	rootCtx := context.Background()
	ctx, cancel := context.WithCancel(rootCtx)
	if cfg.Duration > 0 {
		ctx, cancel = context.WithTimeout(rootCtx, cfg.Duration)
	}
	defer cancel()

	if err := awaitSlurmAvailability(ctx, tr, cfg.CommandTimeout); err != nil {
		return err
	}

	collector := slurm.NewCollector(tr, cfg.CommandTimeout)
	if cfg.Once {
		return runOnce(ctx, collector, tr.Describe())
	}

	updates := make(chan monitor.Update, 8)
	loop := monitor.NewLoop(collector, cfg.Refresh)
	go loop.Run(ctx, updates)

	model := tui.NewModel(tui.Options{
		Source:  tr.Describe(),
		Compact: cfg.Compact,
		NoColor: cfg.NoColor,
		Updates: updates,
	})

	prog := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		return err
	}

	return nil
}

func buildTransport(cfg config.Config) (transport.Transport, error) {
	switch cfg.Mode {
	case config.ModeLocal:
		return transport.NewLocalTransport(), nil
	case config.ModeRemote:
		return transport.NewSSHTransport(transport.SSHOptions{
			Target:         cfg.Target,
			ConfigPath:     cfg.SSHConfig,
			IdentityFile:   cfg.IdentityFile,
			Port:           cfg.Port,
			ConnectTimeout: cfg.ConnectTimeout,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported mode: %s", cfg.Mode)
	}
}

func checkSlurmAvailability(ctx context.Context, tr transport.Transport, timeout time.Duration) error {
	const missingMarker = "__SLURM_MONITOR_MISSING__"
	const checkCmd = `missing=""; for c in squeue scontrol; do if ! command -v "$c" >/dev/null 2>&1; then missing="$missing $c"; fi; done; if [ -n "$missing" ]; then printf '__SLURM_MONITOR_MISSING__%s\n' "$missing"; exit 7; fi`

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := tr.Run(checkCtx, checkCmd)
	if err != nil {
		var runErr *transport.RunError
		if errors.As(err, &runErr) && runErr.ExitCode == 7 {
			missing := ""
			for _, line := range strings.Split(res.Stdout, "\n") {
				if value, ok := strings.CutPrefix(strings.TrimSpace(line), missingMarker); ok {
					missing = strings.TrimSpace(value)
					break
				}
			}
			if missing == "" {
				return fmt.Errorf("Slurm capability check failed without a missing-command list on %s: %w", tr.Describe(), err)
			}
			return &missingSlurmCommandsError{
				source:  tr.Describe(),
				missing: missing,
			}
		}
		if errors.As(err, &runErr) && runErr.Timeout {
			return fmt.Errorf("Slurm capability check timed out on %s; consider increasing --command-timeout: %w", tr.Describe(), err)
		}
		return fmt.Errorf("failed Slurm capability check on %s: %w", tr.Describe(), err)
	}
	return nil
}

func awaitSlurmAvailability(ctx context.Context, tr transport.Transport, timeout time.Duration) error {
	return awaitSlurmAvailabilityWithBackoff(ctx, tr, timeout, 1*time.Second, 30*time.Second)
}

func awaitSlurmAvailabilityWithBackoff(
	ctx context.Context,
	tr transport.Transport,
	timeout time.Duration,
	baseDelay time.Duration,
	maxDelay time.Duration,
) error {
	if baseDelay <= 0 {
		baseDelay = 1 * time.Second
	}
	if maxDelay < baseDelay {
		maxDelay = baseDelay
	}

	delay := baseDelay
	for {
		err := checkSlurmAvailability(ctx, tr, timeout)
		if err == nil {
			return nil
		}
		if isMissingSlurmCommandError(err) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !transport.IsRetryable(err) {
			return err
		}

		fmt.Fprintf(
			os.Stderr,
			"slurm-monitor: transient preflight failure on %s: %v; retrying in %s (Ctrl+C to stop)\n",
			tr.Describe(),
			err,
			delay,
		)

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func isMissingSlurmCommandError(err error) bool {
	if err == nil {
		return false
	}
	var missingErr *missingSlurmCommandsError
	return errors.As(err, &missingErr)
}

func runOnce(ctx context.Context, collector *slurm.Collector, source string) error {
	collectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	snapshot, err := collector.Collect(collectCtx)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "source: %s\n", source)
	fmt.Fprintf(os.Stdout, "collected_at: %s\n", snapshot.CollectedAt.Format(time.RFC3339))
	fmt.Fprintf(
		os.Stdout,
		"queue_jobs: running_cpu=%d running_gpu=%d pending_cpu=%d pending_gpu=%d other=%d total=%d\n",
		snapshot.Queue.RunningCPUJobs,
		snapshot.Queue.RunningGPUJobs,
		snapshot.Queue.PendingCPUJobs,
		snapshot.Queue.PendingGPUJobs,
		snapshot.Queue.Other,
		snapshot.Queue.TotalJobs(),
	)
	fmt.Fprintf(
		os.Stdout,
		"queue_resources: running_cpu=%d running_gpu=%d pending_cpu=%d pending_gpu=%d\n",
		snapshot.Queue.ResourceLoad.RunningCPU,
		snapshot.Queue.ResourceLoad.RunningGPU,
		snapshot.Queue.ResourceLoad.PendingCPU,
		snapshot.Queue.ResourceLoad.PendingGPU,
	)

	partitions := append([]slurm.PartitionSummary(nil), snapshot.Partitions...)
	slurm.SortPartitionsForDisplay(partitions)
	totalPartitions := len(partitions)
	if len(partitions) > 10 {
		partitions = partitions[:10]
	}
	fmt.Fprintf(os.Stdout, "partitions: shown=%d total=%d\n", len(partitions), totalPartitions)
	for _, partition := range partitions {
		q := partition.Queue
		fmt.Fprintf(
			os.Stdout,
			"  - %s running_cpu_jobs=%d running_gpu_jobs=%d pending_cpu_jobs=%d pending_gpu_jobs=%d other=%d running_cpu=%d running_gpu=%d pending_cpu=%d pending_gpu=%d\n",
			partition.Name,
			q.RunningCPUJobs,
			q.RunningGPUJobs,
			q.PendingCPUJobs,
			q.PendingGPUJobs,
			q.Other,
			q.ResourceLoad.RunningCPU,
			q.ResourceLoad.RunningGPU,
			q.ResourceLoad.PendingCPU,
			q.ResourceLoad.PendingGPU,
		)
	}

	users := append([]slurm.UserSummary(nil), snapshot.Users...)
	slurm.SortUsersForDisplay(users)
	totalUsers := len(users)
	if len(users) > 10 {
		users = users[:10]
	}
	fmt.Fprintf(os.Stdout, "users: shown=%d total=%d\n", len(users), totalUsers)
	for _, user := range users {
		fmt.Fprintf(
			os.Stdout,
			"  - %s running_cpu=%d running_gpu=%d running_cpu_jobs=%d running_gpu_jobs=%d pending_cpu_jobs=%d pending_gpu_jobs=%d pending_cpu=%d pending_gpu=%d pending_mem=%s\n",
			user.User,
			user.RunningCPU,
			user.RunningGPU,
			user.RunningCPUJobs,
			user.RunningGPUJobs,
			user.PendingCPUJobs,
			user.PendingGPUJobs,
			user.PendingCPU,
			user.PendingGPU,
			formatMemMB(user.PendingMemMB),
		)
	}

	pendingReasons := append([]slurm.PendingReasonSummary(nil), snapshot.PendingReasons...)
	slurm.SortPendingReasonsForDisplay(pendingReasons)
	totalPendingReasons := len(pendingReasons)
	if len(pendingReasons) > 10 {
		pendingReasons = pendingReasons[:10]
	}
	fmt.Fprintf(os.Stdout, "pending_reasons: shown=%d total=%d\n", len(pendingReasons), totalPendingReasons)
	for _, reason := range pendingReasons {
		fmt.Fprintf(
			os.Stdout,
			"  - reason=%q tasks=%d cpu=%d gpu=%d\n",
			reason.Reason,
			reason.Tasks,
			reason.CPU,
			reason.GPU,
		)
	}

	jobs := append([]slurm.JobSummary(nil), snapshot.Jobs...)
	slurm.SortJobsForDisplay(jobs)
	totalJobs := len(jobs)
	if len(jobs) > 10 {
		jobs = jobs[:10]
	}
	fmt.Fprintf(os.Stdout, "jobs: shown=%d total=%d grouping=root+user+partition+state+pending_reason\n", len(jobs), totalJobs)
	for _, job := range jobs {
		fmt.Fprintf(
			os.Stdout,
			"  - %s user=%s partition=%s state=%s reason=%q tasks=%d cpu=%d gpu=%d\n",
			job.JobID,
			job.User,
			job.Partition,
			job.State,
			job.Reason,
			job.Tasks,
			job.CPU,
			job.GPU,
		)
	}

	return nil
}

func formatMemMB(value int) string {
	if value >= 1024*1024 {
		return fmt.Sprintf("%.1fT", float64(value)/1024.0/1024.0)
	}
	if value >= 1024 {
		return fmt.Sprintf("%.1fG", float64(value)/1024.0)
	}
	return fmt.Sprintf("%dM", value)
}
