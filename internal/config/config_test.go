package config

import (
	"errors"
	"strings"
	"testing"
)

func TestParseArgsLocalDefault(t *testing.T) {
	cfg, err := ParseArgs(nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cfg.Command != CommandMonitor {
		t.Fatalf("expected monitor command, got %s", cfg.Command)
	}
	if cfg.Mode != ModeLocal {
		t.Fatalf("expected local mode, got %s", cfg.Mode)
	}
}

func TestParseArgsRemoteTarget(t *testing.T) {
	cfg, err := ParseArgs([]string{"cluster_alias"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cfg.Mode != ModeRemote {
		t.Fatalf("expected remote mode, got %s", cfg.Mode)
	}
	if cfg.Target != "cluster_alias" {
		t.Fatalf("unexpected target: %q", cfg.Target)
	}
}

func TestParseArgsRemoteUserHostTarget(t *testing.T) {
	cfg, err := ParseArgs([]string{"user@cluster.example.org"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cfg.Mode != ModeRemote {
		t.Fatalf("expected remote mode, got %s", cfg.Mode)
	}
	if cfg.Target != "user@cluster.example.org" {
		t.Fatalf("unexpected target: %q", cfg.Target)
	}
}

func TestParseArgsDoctorCommand(t *testing.T) {
	cfg, err := ParseArgs([]string{"doctor", "cluster_alias"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cfg.Command != CommandDoctor {
		t.Fatalf("expected doctor command, got %s", cfg.Command)
	}
	if cfg.Mode != ModeRemote {
		t.Fatalf("expected remote mode, got %s", cfg.Mode)
	}
	if cfg.Target != "cluster_alias" {
		t.Fatalf("unexpected target: %q", cfg.Target)
	}
}

func TestParseArgsDryRunCommand(t *testing.T) {
	cfg, err := ParseArgs([]string{"dry-run", "--once", "cluster_alias"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cfg.Command != CommandDryRun {
		t.Fatalf("expected dry-run command, got %s", cfg.Command)
	}
	if !cfg.Once {
		t.Fatalf("expected once=true")
	}
}

func TestParseArgsSSHFlagsWithoutTarget(t *testing.T) {
	_, err := ParseArgs([]string{"--ssh-config", "/tmp/x"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseArgsRejectExtraPositional(t *testing.T) {
	_, err := ParseArgs([]string{"a", "b"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseArgsHelpRequested(t *testing.T) {
	_, err := ParseArgs([]string{"--help"})
	if err == nil {
		t.Fatalf("expected help error")
	}
	if !errors.Is(err, ErrHelpRequested) {
		t.Fatalf("expected ErrHelpRequested, got %v", err)
	}
}

func TestHelpTextIncludesUsageAndExamples(t *testing.T) {
	text := HelpText()
	required := []string{
		"Usage:",
		"slurm-monitor [flags] [ssh-target]",
		"slurm-monitor doctor [flags] [ssh-target]",
		"slurm-monitor dry-run [flags] [ssh-target]",
		"slurm-monitor completion [bash|zsh]",
		"Commands:",
		"doctor   Run non-mutating preflight checks and exit.",
		"completion Print shell completion script output and exit.",
		"Behavior:",
		"Authentication:",
		"Examples:",
		"--refresh",
		"--duration",
		"slurm-monitor doctor cluster_alias",
		"slurm-monitor completion bash",
	}
	for _, item := range required {
		if !strings.Contains(text, item) {
			t.Fatalf("help text missing %q", item)
		}
	}
}
