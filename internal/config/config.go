package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
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
		Refresh:        5 * time.Second,
		ConnectTimeout: 10 * time.Second,
		CommandTimeout: 15 * time.Second,
	}
}

func newFlagSet(cfg *Config) *flag.FlagSet {
	fs := flag.NewFlagSet("slurm-monitor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.DurationVar(&cfg.Refresh, "refresh", cfg.Refresh, "poll interval for collecting new Slurm snapshots")
	fs.DurationVar(&cfg.ConnectTimeout, "connect-timeout", cfg.ConnectTimeout, "max SSH connection setup time per command (remote mode)")
	fs.DurationVar(&cfg.CommandTimeout, "command-timeout", cfg.CommandTimeout, "max command runtime; a local timeout kills child processes, and SSH pipe cleanup can take 2 more seconds")
	fs.StringVar(&cfg.SSHConfig, "ssh-config", "", "alternate OpenSSH config path (supports ~/, Host aliases/ProxyJump)")
	fs.StringVar(&cfg.IdentityFile, "identity-file", "", "SSH private key path passed to ssh -i (supports ~/)")
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
	b.WriteString("slurm-monitor: read-only Slurm scheduler monitor\n\n")
	b.WriteString("Usage:\n")
	b.WriteString("  slurm-monitor [flags] [ssh-target]\n")
	b.WriteString("  slurm-monitor doctor [flags] [ssh-target]\n")
	b.WriteString("  slurm-monitor dry-run [flags] [ssh-target]\n")
	b.WriteString("  slurm-monitor completion [bash|zsh]\n\n")
	b.WriteString("Commands:\n")
	b.WriteString("  monitor  Start live monitoring (default when no command is given).\n")
	b.WriteString("  doctor   Run non-mutating preflight checks and exit.\n")
	b.WriteString("  dry-run  Print planned execution order and exit.\n")
	b.WriteString("  completion Print shell completion script output and exit.\n\n")
	b.WriteString("Positional target:\n")
	b.WriteString("  ssh-target is optional.\n")
	b.WriteString("  - omitted: run locally (requires local squeue/scontrol)\n")
	b.WriteString("  - provided: run remotely through OpenSSH using alias or user@host\n\n")
	b.WriteString("Behavior:\n")
	b.WriteString("  - flags work before or after the target; -- ends flag parsing\n")
	b.WriteString("  - in the TUI, q or Ctrl+C exits and restores the terminal\n")
	b.WriteString("  - monitor is read-only and never mutates Slurm state\n")
	b.WriteString("  - doctor checks required commands; dry-run prints the resolved plan without running commands\n")
	b.WriteString("  - monitor mode retries transient SSH and network failures with backoff\n")
	b.WriteString("  - retries continue until interrupted unless --duration sets a deadline\n")
	b.WriteString("  - missing Slurm commands and permanent SSH, authentication, or configuration failures stop without retry\n\n")
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
	b.WriteString("  slurm-monitor --refresh 1s user@cluster.example.org\n")
	b.WriteString("  slurm-monitor --once cluster_alias\n")
	b.WriteString("  slurm-monitor --duration 30m cluster_alias\n")
	b.WriteString("  slurm-monitor doctor cluster_alias\n")
	b.WriteString("  slurm-monitor dry-run --once cluster_alias\n")
	b.WriteString("  slurm-monitor completion bash\n")

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

	var pos []string
	for len(args) > 0 {
		if args[0] == "--" {
			pos = append(pos, args[1:]...)
			break
		}
		if !strings.HasPrefix(args[0], "-") || args[0] == "-" {
			pos = append(pos, args[0])
			args = args[1:]
			continue
		}
		n := 1
		name, _, hasValue := strings.Cut(strings.TrimLeft(args[0], "-"), "=")
		if f := fs.Lookup(name); f != nil && !hasValue {
			boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool })
			if (!ok || !boolFlag.IsBoolFlag()) && len(args) > 1 {
				n = 2
			}
		}
		if err := fs.Parse(args[:n]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return Config{}, ErrHelpRequested
			}
			return Config{}, err
		}
		args = args[n:]
	}
	if len(pos) > 1 {
		return Config{}, fmt.Errorf("expected zero or one positional target, got %d", len(pos))
	}
	if len(pos) == 1 {
		cfg.Target = pos[0]
		if strings.HasPrefix(cfg.Target, "-") || strings.ContainsFunc(cfg.Target, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return Config{}, fmt.Errorf("invalid SSH target")
		}
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
	if cfg.Port < 0 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("--port must be between 0 and 65535")
	}

	if cfg.Mode == ModeLocal {
		if cfg.SSHConfig != "" || cfg.IdentityFile != "" || cfg.Port != 0 {
			return Config{}, fmt.Errorf("ssh-specific flags require a remote target")
		}
	}

	cfg.SSHConfig = resolveHomePath(cfg.SSHConfig)
	cfg.IdentityFile = resolveHomePath(cfg.IdentityFile)
	return cfg, nil
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
