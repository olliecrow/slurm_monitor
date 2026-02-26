package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"slurm_monitor/internal/config"
	"slurm_monitor/internal/transport"
)

func TestRunDoctorWithDepsLocalPass(t *testing.T) {
	cfg := config.Config{
		Mode:           config.ModeLocal,
		CommandTimeout: 2 * time.Second,
	}

	deps := doctorDeps{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		stat: os.Stat,
		buildTransport: func(config.Config) (transport.Transport, error) {
			return fakeTransport{}, nil
		},
		checkAvailability: func(context.Context, transport.Transport, time.Duration) error {
			return nil
		},
	}

	var out strings.Builder
	err := runDoctorWithDeps(cfg, &out, deps)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	text := out.String()
	required := []string{
		"slurm-monitor doctor",
		"[ok] local tool sinfo",
		"[ok] local tool squeue",
		"[ok] local tool scontrol",
		"doctor result: PASS",
	}
	for _, item := range required {
		if !strings.Contains(text, item) {
			t.Fatalf("doctor output missing %q", item)
		}
	}
}

func TestRunDoctorWithDepsRemoteFailure(t *testing.T) {
	cfg := config.Config{
		Mode:           config.ModeRemote,
		Target:         "cluster_alias",
		CommandTimeout: 2 * time.Second,
	}

	deps := doctorDeps{
		lookPath: func(name string) (string, error) {
			if name == "ssh" {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
		stat: os.Stat,
		buildTransport: func(config.Config) (transport.Transport, error) {
			return fakeTransport{}, nil
		},
		checkAvailability: func(context.Context, transport.Transport, time.Duration) error {
			return errors.New("remote check failed")
		},
	}

	var out strings.Builder
	err := runDoctorWithDeps(cfg, &out, deps)
	if err == nil {
		t.Fatalf("expected failure")
	}

	text := out.String()
	required := []string{
		"[fail] local tool ssh",
		"[fail] slurm preflight",
		"doctor result: FAIL",
	}
	for _, item := range required {
		if !strings.Contains(text, item) {
			t.Fatalf("doctor output missing %q", item)
		}
	}
}

func TestRunDryRunLocal(t *testing.T) {
	cfg := config.Config{
		Mode:           config.ModeLocal,
		Refresh:        2 * time.Second,
		ConnectTimeout: 10 * time.Second,
		CommandTimeout: 15 * time.Second,
	}

	var out strings.Builder
	if err := RunDryRun(cfg, &out); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	text := out.String()
	required := []string{
		"slurm-monitor dry-run",
		"mode: local",
		"Run a local preflight check",
		"dry-run only: no local or remote commands were executed.",
	}
	for _, item := range required {
		if !strings.Contains(text, item) {
			t.Fatalf("dry-run output missing %q", item)
		}
	}
}

func TestRunDryRunRemoteOnce(t *testing.T) {
	cfg := config.Config{
		Mode:           config.ModeRemote,
		Target:         "cluster_alias",
		Refresh:        1 * time.Second,
		ConnectTimeout: 9 * time.Second,
		CommandTimeout: 11 * time.Second,
		Once:           true,
		Duration:       30 * time.Minute,
	}

	var out strings.Builder
	if err := RunDryRun(cfg, &out); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	text := out.String()
	required := []string{
		"mode: remote",
		"target: cluster_alias",
		"Collect one snapshot, print summary metrics, and exit.",
		"duration: 30m0s",
	}
	for _, item := range required {
		if !strings.Contains(text, item) {
			t.Fatalf("dry-run output missing %q", item)
		}
	}
}
