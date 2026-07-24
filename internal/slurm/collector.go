package slurm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/olliecrow/slurm_monitor/internal/transport"
)

const (
	// Use -r so job arrays are expanded one task per line; this keeps queue/user
	// counts and requested/allocated CPU/GPU demand accurate for large arrays.
	// Use tres-alloc instead of %b so GPU demand comes from Slurm's documented
	// TRES view for both running and pending jobs.
	combinedCollectCommand = `scontrol show node -o; echo "__SLURM_MONITOR_SPLIT__"; squeue -h -r -O "JobID:|,State:|,UserName:|,NumCPUs:|,MinMemory:|,tres-alloc"`

	maxPendingGPUProbesPerCollect = 4
)

type Collector struct {
	transport                transport.Transport
	commandTimeout           time.Duration
	pendingGPUCountByJobRoot map[string]int
}

func NewCollector(t transport.Transport, commandTimeout time.Duration) *Collector {
	return &Collector{
		transport:                t,
		commandTimeout:           commandTimeout,
		pendingGPUCountByJobRoot: make(map[string]int),
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
	probeCtx, cancelProbes := context.WithTimeout(ctx, c.commandTimeout)
	c.fillPendingGPURequestCache(probeCtx, queueRaw)
	cancelProbes()
	queue, users := parseQueueLines(queueRaw, c.pendingGPUCountByJobRoot)

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
	active := make(map[string]struct{}, len(roots))
	probes := 0
	for _, root := range roots {
		active[root] = struct{}{}
		if _, ok := c.pendingGPUCountByJobRoot[root]; ok {
			continue
		}
		if probes >= maxPendingGPUProbesPerCollect {
			continue
		}
		probes++
		gpuCount, err := c.jobRootRequestsGPU(ctx, root)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}
		c.pendingGPUCountByJobRoot[root] = gpuCount
	}
	for root := range c.pendingGPUCountByJobRoot {
		if _, ok := active[root]; !ok {
			delete(c.pendingGPUCountByJobRoot, root)
		}
	}
}

func (c *Collector) jobRootRequestsGPU(ctx context.Context, root string) (int, error) {
	if !isNumericJobID(root) {
		return 0, fmt.Errorf("invalid job root id %q", root)
	}
	raw, err := c.runWithTimeout(ctx, fmt.Sprintf("scontrol show job -o %s", root))
	if err != nil {
		return 0, err
	}
	reqTRES := extractReqTRES(raw)
	return parseGPUReq(reqTRES), nil
}

func extractPendingJobRoots(queueRaw string) []string {
	lines := strings.Split(queueRaw, "\n")
	set := make(map[string]struct{})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 6 {
			continue
		}
		state := strings.ToUpper(strings.TrimSpace(parts[1]))
		if !strings.Contains(state, "PENDING") {
			continue
		}
		tresAlloc := strings.TrimSpace(parts[5])
		if tresAlloc != "" && !strings.EqualFold(tresAlloc, "N/A") {
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
	sort.Strings(out)
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
