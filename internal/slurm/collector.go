package slurm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"slurm_monitor/internal/transport"
)

const (
	// Use -r so job arrays are expanded one task per line; this keeps queue/user
	// counts and pending CPU/GPU demand accurate for large arrays.
	combinedCollectCommand = `scontrol show node -o; echo "__SLURM_MONITOR_SPLIT__"; squeue -h -r -o "%i|%T|%u|%C|%m|%b|%P|%j|%r"`
)

type Collector struct {
	transport           transport.Transport
	commandTimeout      time.Duration
	pendingGPUByJobRoot map[string]bool
}

func NewCollector(t transport.Transport, commandTimeout time.Duration) *Collector {
	return &Collector{
		transport:           t,
		commandTimeout:      commandTimeout,
		pendingGPUByJobRoot: make(map[string]bool),
	}
}

func (c *Collector) Collect(ctx context.Context) (Snapshot, error) {
	raw, err := c.runWithTimeout(ctx, combinedCollectCommand)
	if err != nil {
		return Snapshot{}, fmt.Errorf("collect snapshot: %w", err)
	}

	nodesRaw, queueRaw, err := splitCombinedOutput(raw)
	if err != nil {
		return Snapshot{}, err
	}

	nodes, err := parseNodeLines(nodesRaw)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse nodes: %w", err)
	}
	c.fillPendingGPURequestCache(ctx, queueRaw)
	queue, users := parseQueueLines(queueRaw, c.pendingGPUByJobRoot)

	return Snapshot{
		Nodes:       nodes,
		Queue:       queue,
		Users:       users,
		CollectedAt: time.Now(),
	}, nil
}

func (c *Collector) runWithTimeout(ctx context.Context, command string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, c.commandTimeout)
	defer cancel()

	res, err := c.transport.Run(cmdCtx, command)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(res.Stdout, "\n"), nil
}

func (c *Collector) fillPendingGPURequestCache(ctx context.Context, queueRaw string) {
	roots := extractPendingJobRoots(queueRaw)
	for _, root := range roots {
		if _, ok := c.pendingGPUByJobRoot[root]; ok {
			continue
		}
		hasGPU, err := c.jobRootRequestsGPU(ctx, root)
		if err != nil {
			continue
		}
		c.pendingGPUByJobRoot[root] = hasGPU
	}
}

func (c *Collector) jobRootRequestsGPU(ctx context.Context, root string) (bool, error) {
	if !isNumericJobID(root) {
		return false, fmt.Errorf("invalid job root id %q", root)
	}
	raw, err := c.runWithTimeout(ctx, fmt.Sprintf("scontrol show job -o %s", root))
	if err != nil {
		return false, err
	}
	reqTRES := extractReqTRES(raw)
	return strings.Contains(strings.ToLower(reqTRES), "gres/gpu"), nil
}

func extractPendingJobRoots(queueRaw string) []string {
	lines := strings.Split(queueRaw, "\n")
	set := make(map[string]struct{})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 9)
		if len(parts) < 9 {
			continue
		}
		state := strings.ToUpper(strings.TrimSpace(parts[1]))
		if !strings.Contains(state, "PENDING") {
			continue
		}
		root := rootJobID(parts[0])
		if root == "" {
			continue
		}
		set[root] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for root := range set {
		out = append(out, root)
	}
	return out
}

func extractReqTRES(raw string) string {
	idx := strings.Index(raw, "ReqTRES=")
	if idx < 0 {
		return ""
	}
	tail := raw[idx+len("ReqTRES="):]
	fields := strings.Fields(tail)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func isNumericJobID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func splitCombinedOutput(raw string) (nodes string, queue string, err error) {
	const marker = "__SLURM_MONITOR_SPLIT__"
	parts := strings.SplitN(raw, marker, 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected collector output format: split marker missing")
	}
	nodes = strings.TrimSpace(parts[0])
	queue = strings.TrimSpace(parts[1])
	return nodes, queue, nil
}
