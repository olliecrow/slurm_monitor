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
		"Behavior:",
		"Authentication:",
		"Examples:",
		"--refresh",
		"--duration",
	}
	for _, item := range required {
		if !strings.Contains(text, item) {
			t.Fatalf("help text missing %q", item)
		}
	}
}
