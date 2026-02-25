package monitor

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"slurm_monitor/internal/slurm"
)

func TestBackoffDelayBounds(t *testing.T) {
	l := &Loop{
		BaseBackoff: 1 * time.Second,
		MaxBackoff:  10 * time.Second,
		Rand:        rand.New(rand.NewSource(1)),
	}

	for i := 1; i <= 10; i++ {
		d := l.backoffDelay(i)
		if d < l.BaseBackoff {
			t.Fatalf("delay below base: %v", d)
		}
		if d > l.MaxBackoff {
			t.Fatalf("delay above max: %v", d)
		}
	}
}

type fakeCollector struct {
	mu       sync.Mutex
	results  []slurm.Snapshot
	errors   []error
	position int
}

type scriptedCollector struct {
	mu       sync.Mutex
	position int
	steps    []collectStep
}

type collectStep struct {
	snapshot slurm.Snapshot
	err      error
}

func (s *scriptedCollector) Collect(context.Context) (slurm.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.position >= len(s.steps) {
		return slurm.Snapshot{}, errors.New("exhausted")
	}
	step := s.steps[s.position]
	s.position++
	return step.snapshot, step.err
}

func (f *fakeCollector) Collect(context.Context) (slurm.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.position >= len(f.results)+len(f.errors) {
		return slurm.Snapshot{}, errors.New("exhausted")
	}

	idx := f.position
	f.position++

	// Interleave by result index then error index as configured by explicit slices.
	// For this test we map first len(results) as results, then remaining as errors.
	if idx < len(f.results) {
		return f.results[idx], nil
	}
	return slurm.Snapshot{}, f.errors[idx-len(f.results)]
}

func TestLoopEmitsConnectedThenRecovering(t *testing.T) {
	fc := &fakeCollector{
		results: []slurm.Snapshot{
			{CollectedAt: time.Now()},
		},
		errors: []error{
			errors.New("timeout one"),
			errors.New("timeout two"),
			errors.New("timeout three"),
		},
	}

	loop := &Loop{
		Collector:        fc,
		Refresh:          5 * time.Millisecond,
		BaseBackoff:      5 * time.Millisecond,
		MaxBackoff:       10 * time.Millisecond,
		FailureThreshold: 2,
		Rand:             rand.New(rand.NewSource(1)),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	updates := make(chan Update, 10)
	go loop.Run(ctx, updates)

	var got []State
	for update := range updates {
		got = append(got, update.State)
		if len(got) >= 4 {
			cancel()
		}
	}

	if len(got) == 0 {
		t.Fatalf("expected updates")
	}
	if got[0] != StateConnected {
		t.Fatalf("expected first state connected, got %s", got[0])
	}
	foundRecovering := false
	for _, s := range got {
		if s == StateDisconnectedRecovering {
			foundRecovering = true
			break
		}
	}
	if !foundRecovering {
		t.Fatalf("expected disconnected-recovering state in updates: %v", got)
	}
}

func TestLoopRecoversAfterTransientFailures(t *testing.T) {
	now := time.Now()
	sc := &scriptedCollector{
		steps: []collectStep{
			{snapshot: slurm.Snapshot{CollectedAt: now}},
			{err: errors.New("temporary timeout")},
			{err: errors.New("temporary timeout")},
			{snapshot: slurm.Snapshot{CollectedAt: now.Add(2 * time.Second)}},
		},
	}

	loop := &Loop{
		Collector:        sc,
		Refresh:          5 * time.Millisecond,
		BaseBackoff:      5 * time.Millisecond,
		MaxBackoff:       10 * time.Millisecond,
		FailureThreshold: 2,
		Rand:             rand.New(rand.NewSource(1)),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	updates := make(chan Update, 16)
	go loop.Run(ctx, updates)

	var states []State
	for update := range updates {
		states = append(states, update.State)
		if len(states) >= 4 {
			cancel()
		}
	}

	if len(states) < 4 {
		t.Fatalf("expected at least 4 states, got %v", states)
	}
	if states[0] != StateConnected {
		t.Fatalf("expected initial connected state, got %s", states[0])
	}
	if states[1] != StateReconnecting {
		t.Fatalf("expected first error to emit reconnecting, got %s", states[1])
	}
	if states[2] != StateDisconnectedRecovering {
		t.Fatalf("expected repeated errors to emit disconnected-recovering, got %s", states[2])
	}
	if states[3] != StateConnected {
		t.Fatalf("expected recovery to return connected, got %s", states[3])
	}
}
