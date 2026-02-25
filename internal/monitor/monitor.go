package monitor

import (
	"context"
	"math/rand"
	"time"

	"slurm_monitor/internal/slurm"
)

type State string

const (
	StateConnected              State = "connected"
	StateReconnecting           State = "reconnecting"
	StateDisconnectedRecovering State = "disconnected-recovering"
)

type Update struct {
	Snapshot    *slurm.Snapshot
	State       State
	LastError   string
	LastSuccess time.Time
	NextRetry   time.Time
}

type Collector interface {
	Collect(ctx context.Context) (slurm.Snapshot, error)
}

type Loop struct {
	Collector        Collector
	Refresh          time.Duration
	BaseBackoff      time.Duration
	MaxBackoff       time.Duration
	FailureThreshold int
	Rand             *rand.Rand
}

func NewLoop(collector Collector, refresh time.Duration) *Loop {
	return &Loop{
		Collector:        collector,
		Refresh:          refresh,
		BaseBackoff:      1 * time.Second,
		MaxBackoff:       30 * time.Second,
		FailureThreshold: 3,
	}
}

func (l *Loop) Run(ctx context.Context, updates chan<- Update) {
	defer close(updates)

	if l.Rand == nil {
		l.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	failures := 0
	var lastSuccess time.Time

	for {
		snapshot, err := l.Collector.Collect(ctx)
		if err == nil {
			failures = 0
			lastSuccess = snapshot.CollectedAt
			if !sendUpdate(ctx, updates, Update{
				Snapshot:    &snapshot,
				State:       StateConnected,
				LastSuccess: lastSuccess,
			}) {
				return
			}
			if !wait(ctx, l.Refresh) {
				return
			}
			continue
		}

		failures++
		state := StateReconnecting
		if failures >= l.FailureThreshold {
			state = StateDisconnectedRecovering
		}
		delay := l.backoffDelay(failures)

		if !sendUpdate(ctx, updates, Update{
			State:       state,
			LastError:   err.Error(),
			LastSuccess: lastSuccess,
			NextRetry:   time.Now().Add(delay),
		}) {
			return
		}

		if !wait(ctx, delay) {
			return
		}
	}
}

func (l *Loop) backoffDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	delay := l.BaseBackoff
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= l.MaxBackoff {
			delay = l.MaxBackoff
			break
		}
	}

	jitterFactor := 0.8 + (l.Rand.Float64() * 0.4)
	jittered := time.Duration(float64(delay) * jitterFactor)
	if jittered < l.BaseBackoff {
		jittered = l.BaseBackoff
	}
	if jittered > l.MaxBackoff {
		jittered = l.MaxBackoff
	}
	return jittered
}

func sendUpdate(ctx context.Context, updates chan<- Update, update Update) bool {
	select {
	case <-ctx.Done():
		return false
	case updates <- update:
		return true
	}
}

func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
