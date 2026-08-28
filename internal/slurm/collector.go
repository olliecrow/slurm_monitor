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
	// -r expands each array task onto its own row, so task and resource totals
	// do not undercount arrays. tres-alloc supplies Slurm's running and pending
	// TRES values. The trailing colons prevent -O from truncating TRES and reasons.
	combinedCollectCommand = `scontrol show nodes --oneliner && printf '\n__SLURM_MONITOR_SPLIT__\n' && squeue -h -r -O "JobID:|,State:|,UserName:|,Partition:|,NumCPUs:|,MinMemory:|,tres-alloc:|,Reason:|"`

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
	available, err := parseAvailableResources(nodesRaw)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse available resources: %w", err)
	}
	probeCtx, cancelProbes := context.WithTimeout(ctx, c.commandTimeout)
	c.fillPendingGPURequestCache(probeCtx, queueRaw)
	cancelProbes()
	queueData, err := parseQueueLines(queueRaw, c.pendingGPUCountByJobRoot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse queue: %w", err)
	}

	return Snapshot{
		Queue:          queueData.Queue,
		Available:      available,
		Partitions:     queueData.Partitions,
		Users:          queueData.Users,
		PendingReasons: queueData.PendingReasons,
		Jobs:           queueData.Jobs,
		CollectedAt:    time.Now(),
	}, nil
}

func splitCombinedOutput(raw string) (nodes string, queue string, err error) {
	const marker = "__SLURM_MONITOR_SPLIT__"
	parts := strings.SplitN(raw, marker, 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected collector output format: split marker missing")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
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
	return parseGPUCount(reqTRES), nil
}

func extractPendingJobRoots(queueRaw string) []string {
	lines := strings.Split(queueRaw, "\n")
	set := make(map[string]struct{})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := splitQueueRow(line)
		if len(parts) < 8 {
			continue
		}
		state := strings.ToUpper(strings.TrimSpace(parts[1]))
		if !strings.Contains(state, "PENDING") {
			continue
		}
		tresAlloc := strings.TrimSpace(parts[6])
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
