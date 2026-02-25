package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"slurm_monitor/internal/config"
	"slurm_monitor/internal/monitor"
	"slurm_monitor/internal/slurm"
	"slurm_monitor/internal/transport"
	"slurm_monitor/internal/tui"
	"slurm_monitor/internal/uifmt"
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
		Source:      tr.Describe(),
		Compact:     cfg.Compact,
		NoColor:     cfg.NoColor,
		Refresh:     cfg.Refresh,
		MaxDuration: cfg.Duration,
		Updates:     updates,
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
	const checkCmd = `missing=""; for c in sinfo squeue scontrol; do if ! command -v "$c" >/dev/null 2>&1; then missing="$missing $c"; fi; done; if [ -n "$missing" ]; then echo "$missing"; exit 7; fi`

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := tr.Run(checkCtx, checkCmd)
	if err != nil {
		if missing := strings.TrimSpace(res.Stdout); missing != "" {
			return &missingSlurmCommandsError{
				source:  tr.Describe(),
				missing: missing,
			}
		}
		var runErr *transport.RunError
		if errors.As(err, &runErr) && runErr.Timeout {
			return fmt.Errorf("Slurm capability check timed out on %s; consider increasing --command-timeout", tr.Describe())
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
	fmt.Fprintf(os.Stdout, "nodes: %d\n", len(snapshot.Nodes))
	fmt.Fprintf(os.Stdout, "queue: running=%d pending=%d\n", snapshot.Queue.Running, snapshot.Queue.Pending)

	totals := snapshot.Totals()
	fmt.Fprintf(
		os.Stdout,
		"totals: cpu=%s mem=%s gpu=%s\n",
		uifmt.Ratio(totals.CPUAlloc, totals.CPUTotal),
		uifmt.MemPair(totals.MemAllocMB, totals.MemTotalMB),
		uifmt.Ratio(totals.GPUAlloc, totals.GPUTotal),
	)

	users := append([]slurm.UserSummary(nil), snapshot.Users...)
	slurm.SortUsersByPendingDemand(users)
	if len(users) > 10 {
		users = users[:10]
	}
	fmt.Fprintln(os.Stdout, "users:")
	for _, user := range users {
		fmt.Fprintf(
			os.Stdout,
			"  - %s running=%d pending=%d pending_cpu_jobs=%d pending_mem=%s pending_gpu_jobs=%d\n",
			user.User,
			user.Running,
			user.Pending,
			user.PendingCPUJobs,
			uifmt.MemMB(user.PendingMemMB),
			user.PendingGPUJobs,
		)
	}

	return nil
}
