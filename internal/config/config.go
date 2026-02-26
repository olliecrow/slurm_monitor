package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type Mode string

const (
	ModeLocal  Mode = "local"
	ModeRemote Mode = "remote"
)

type Command string

const (
	CommandMonitor Command = "monitor"
	CommandDoctor  Command = "doctor"
	CommandDryRun  Command = "dry-run"
)

type Config struct {
	Command        Command
	Mode           Mode
	Target         string
	Refresh        time.Duration
	ConnectTimeout time.Duration
	CommandTimeout time.Duration
	SSHConfig      string
	IdentityFile   string
	Port           int
	NoColor        bool
	Compact        bool
	Once           bool
	Duration       time.Duration
}

var ErrHelpRequested = errors.New("help requested")

func defaultConfig() Config {
	return Config{
		Command:        CommandMonitor,
		Refresh:        2 * time.Second,
		ConnectTimeout: 10 * time.Second,
		CommandTimeout: 15 * time.Second,
	}
}

func newFlagSet(cfg *Config) *flag.FlagSet {
	fs := flag.NewFlagSet("slurm-monitor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.DurationVar(&cfg.Refresh, "refresh", cfg.Refresh, "poll interval for collecting new Slurm snapshots")
	fs.DurationVar(&cfg.ConnectTimeout, "connect-timeout", cfg.ConnectTimeout, "max SSH connection setup time per command (remote mode)")
	fs.DurationVar(&cfg.CommandTimeout, "command-timeout", cfg.CommandTimeout, "max runtime for each Slurm command before retry")
	fs.StringVar(&cfg.SSHConfig, "ssh-config", "", "alternate OpenSSH config path (remote mode, supports Host aliases/ProxyJump)")
	fs.StringVar(&cfg.IdentityFile, "identity-file", "", "explicit SSH private key path passed to ssh -i (remote mode)")
	fs.IntVar(&cfg.Port, "port", 0, "override SSH port for remote target (remote mode)")
	fs.BoolVar(&cfg.NoColor, "no-color", false, "disable ANSI color styling")
	fs.BoolVar(&cfg.Compact, "compact", false, "force compact TUI layout for smaller terminals")
	fs.BoolVar(&cfg.Once, "once", false, "collect one snapshot, print summary, and exit")
	fs.DurationVar(&cfg.Duration, "duration", 0, "optional total runtime limit; 0 means run until interrupted")

	return fs
}

func HelpText() string {
	cfg := defaultConfig()
	fs := newFlagSet(&cfg)

	var b strings.Builder
	b.WriteString("slurm-monitor: resilient, read-only Slurm queue/node monitor\n\n")
	b.WriteString("Usage:\n")
	b.WriteString("  slurm-monitor [flags] [ssh-target]\n")
	b.WriteString("  slurm-monitor doctor [flags] [ssh-target]\n")
	b.WriteString("  slurm-monitor dry-run [flags] [ssh-target]\n\n")
	b.WriteString("Commands:\n")
	b.WriteString("  monitor  Start live monitoring (default when no command is given).\n")
	b.WriteString("  doctor   Run non-mutating preflight checks and exit.\n")
	b.WriteString("  dry-run  Print planned execution order and exit.\n\n")
	b.WriteString("Positional target:\n")
	b.WriteString("  ssh-target is optional.\n")
	b.WriteString("  - omitted: run locally (requires local sinfo/squeue/scontrol)\n")
	b.WriteString("  - provided: run remotely through OpenSSH using alias or user@host\n\n")
	b.WriteString("Behavior:\n")
	b.WriteString("  - monitor is read-only and never mutates Slurm state\n")
	b.WriteString("  - doctor and dry-run are non-mutating helpers for setup and validation\n")
	b.WriteString("  - transient SSH/network failures retry automatically with backoff in monitor mode\n")
	b.WriteString("  - retries are infinite by default; set --duration to time-box a run\n")
	b.WriteString("  - missing Slurm commands are treated as non-recoverable errors\n\n")
	b.WriteString("Authentication:\n")
	b.WriteString("  - uses standard OpenSSH auth flows (ssh-agent, keys, config)\n")
	b.WriteString("  - supports SSH config aliases and bastion/proxy jumps\n")
	b.WriteString("  - does not accept password flags\n\n")
	b.WriteString("Flags:\n")
	fs.SetOutput(&b)
	fs.PrintDefaults()
	b.WriteString("\nExamples:\n")
	b.WriteString("  slurm-monitor\n")
	b.WriteString("  slurm-monitor cluster_alias\n")
	b.WriteString("  slurm-monitor user@cluster.example.org --refresh 1s\n")
	b.WriteString("  slurm-monitor --once cluster_alias\n")
	b.WriteString("  slurm-monitor --duration 30m cluster_alias\n")
	b.WriteString("  slurm-monitor doctor cluster_alias\n")
	b.WriteString("  slurm-monitor dry-run --once cluster_alias\n")

	return b.String()
}

func splitCommand(args []string) (Command, []string) {
	if len(args) == 0 {
		return CommandMonitor, args
	}

	switch strings.TrimSpace(args[0]) {
	case string(CommandDoctor):
		return CommandDoctor, args[1:]
	case string(CommandDryRun):
		return CommandDryRun, args[1:]
	case string(CommandMonitor):
		return CommandMonitor, args[1:]
	default:
		return CommandMonitor, args
	}
}

func ParseArgs(args []string) (Config, error) {
	cfg := defaultConfig()
	cfg.Command, args = splitCommand(args)
	fs := newFlagSet(&cfg)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Config{}, ErrHelpRequested
		}
		return Config{}, err
	}

	pos := fs.Args()
	if len(pos) > 1 {
		return Config{}, fmt.Errorf("expected zero or one positional target, got %d", len(pos))
	}
	if len(pos) == 1 {
		cfg.Target = strings.TrimSpace(pos[0])
	}

	if cfg.Target == "" {
		cfg.Mode = ModeLocal
	} else {
		cfg.Mode = ModeRemote
	}

	if cfg.Refresh <= 0 {
		return Config{}, fmt.Errorf("--refresh must be > 0")
	}
	if cfg.ConnectTimeout <= 0 {
		return Config{}, fmt.Errorf("--connect-timeout must be > 0")
	}
	if cfg.CommandTimeout <= 0 {
		return Config{}, fmt.Errorf("--command-timeout must be > 0")
	}
	if cfg.Duration < 0 {
		return Config{}, fmt.Errorf("--duration must be >= 0")
	}
	if cfg.Port < 0 {
		return Config{}, fmt.Errorf("--port must be >= 0")
	}

	if cfg.Mode == ModeLocal {
		if cfg.SSHConfig != "" || cfg.IdentityFile != "" || cfg.Port != 0 {
			return Config{}, fmt.Errorf("ssh-specific flags require a remote target")
		}
	}

	return cfg, nil
}
