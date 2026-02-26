package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"slurm_monitor/internal/config"
	"slurm_monitor/internal/transport"
)

type doctorCheck struct {
	name   string
	detail string
	err    error
}

type doctorDeps struct {
	lookPath          func(string) (string, error)
	stat              func(string) (os.FileInfo, error)
	buildTransport    func(config.Config) (transport.Transport, error)
	checkAvailability func(context.Context, transport.Transport, time.Duration) error
}

func defaultDoctorDeps() doctorDeps {
	return doctorDeps{
		lookPath:          exec.LookPath,
		stat:              os.Stat,
		buildTransport:    buildTransport,
		checkAvailability: checkSlurmAvailability,
	}
}

func RunDoctor(cfg config.Config, out io.Writer) error {
	return runDoctorWithDeps(cfg, out, defaultDoctorDeps())
}

func runDoctorWithDeps(cfg config.Config, out io.Writer, deps doctorDeps) error {
	target := "local"
	if cfg.Mode == config.ModeRemote {
		target = cfg.Target
	}

	fmt.Fprintln(out, "slurm-monitor doctor")
	fmt.Fprintf(out, "mode: %s\n", cfg.Mode)
	fmt.Fprintf(out, "target: %s\n\n", target)

	checks := buildDoctorChecks(cfg, deps)
	failed := false
	for _, check := range checks {
		if check.err != nil {
			failed = true
			fmt.Fprintf(out, "[fail] %s: %v\n", check.name, check.err)
			continue
		}
		fmt.Fprintf(out, "[ok] %s: %s\n", check.name, check.detail)
	}

	if failed {
		fmt.Fprintln(out, "\ndoctor result: FAIL")
		return errors.New("doctor checks failed")
	}

	fmt.Fprintln(out, "\ndoctor result: PASS")
	return nil
}

func buildDoctorChecks(cfg config.Config, deps doctorDeps) []doctorCheck {
	checks := make([]doctorCheck, 0, 8)

	appendToolCheck := func(scope string, tool string) {
		if path, err := deps.lookPath(tool); err != nil {
			checks = append(checks, doctorCheck{
				name: scope + " tool " + tool,
				err:  fmt.Errorf("not found in PATH"),
			})
		} else {
			checks = append(checks, doctorCheck{
				name:   scope + " tool " + tool,
				detail: path,
			})
		}
	}

	appendFileCheck := func(name string, path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		resolved := resolveHomePath(path)
		info, err := deps.stat(resolved)
		if err != nil {
			checks = append(checks, doctorCheck{
				name: name,
				err:  fmt.Errorf("path is not readable: %s", resolved),
			})
			return
		}
		if info.IsDir() {
			checks = append(checks, doctorCheck{
				name: name,
				err:  fmt.Errorf("expected a file but found a directory: %s", resolved),
			})
			return
		}
		checks = append(checks, doctorCheck{
			name:   name,
			detail: resolved,
		})
	}

	if cfg.Mode == config.ModeLocal {
		for _, tool := range []string{"bash", "sinfo", "squeue", "scontrol"} {
			appendToolCheck("local", tool)
		}
	} else {
		appendToolCheck("local", "ssh")
		appendFileCheck("ssh config file", cfg.SSHConfig)
		appendFileCheck("ssh identity file", cfg.IdentityFile)
	}

	tr, err := deps.buildTransport(cfg)
	if err != nil {
		checks = append(checks, doctorCheck{
			name: "transport initialization",
			err:  err,
		})
		return checks
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout)
	defer cancel()

	if err := deps.checkAvailability(ctx, tr, cfg.CommandTimeout); err != nil {
		checks = append(checks, doctorCheck{
			name: "slurm preflight",
			err:  err,
		})
	} else {
		checks = append(checks, doctorCheck{
			name:   "slurm preflight",
			detail: "required Slurm commands are reachable on " + tr.Describe(),
		})
	}

	return checks
}

func RunDryRun(cfg config.Config, out io.Writer) error {
	target := "local"
	if cfg.Mode == config.ModeRemote {
		target = cfg.Target
	}

	duration := "unbounded"
	if cfg.Duration > 0 {
		duration = cfg.Duration.String()
	}

	fmt.Fprintln(out, "slurm-monitor dry-run")
	fmt.Fprintf(out, "mode: %s\n", cfg.Mode)
	fmt.Fprintf(out, "target: %s\n", target)
	fmt.Fprintf(out, "refresh: %s\n", cfg.Refresh)
	fmt.Fprintf(out, "connect-timeout: %s\n", cfg.ConnectTimeout)
	fmt.Fprintf(out, "command-timeout: %s\n", cfg.CommandTimeout)
	fmt.Fprintf(out, "duration: %s\n", duration)
	fmt.Fprintf(out, "once: %t\n", cfg.Once)
	fmt.Fprintf(out, "compact: %t\n", cfg.Compact)
	fmt.Fprintf(out, "no-color: %t\n\n", cfg.NoColor)

	fmt.Fprintln(out, "planned sequence:")
	fmt.Fprintln(out, "1. Parse flags and build the configured transport.")
	if cfg.Mode == config.ModeLocal {
		fmt.Fprintln(out, "2. Run a local preflight check for bash, sinfo, squeue, and scontrol.")
	} else {
		fmt.Fprintln(out, "2. Connect over OpenSSH to the target and validate sinfo, squeue, and scontrol remotely.")
	}
	if cfg.Once {
		fmt.Fprintln(out, "3. Collect one snapshot, print summary metrics, and exit.")
	} else {
		fmt.Fprintln(out, "3. Start the polling loop and render the live TUI until interrupted or duration is reached.")
	}
	fmt.Fprintln(out, "4. Exit without mutating any Slurm queue or cluster state.")
	fmt.Fprintln(out, "\ndry-run only: no local or remote commands were executed.")

	return nil
}

func resolveHomePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil && strings.TrimSpace(home) != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
