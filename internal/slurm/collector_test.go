package slurm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/olliecrow/slurm_monitor/internal/transport"
)

func TestCombinedCollectCommandExpandsArrayTasks(t *testing.T) {
	if strings.Contains(combinedCollectCommand, ";") {
		t.Fatalf("combined collect command must stop after a failed read: %q", combinedCollectCommand)
	}
	if !strings.Contains(combinedCollectCommand, "squeue -h -r ") {
		t.Fatalf("combined collect command must include squeue -r to expand arrays: %q", combinedCollectCommand)
	}
	if !strings.Contains(combinedCollectCommand, "tres-alloc:|") {
		t.Fatalf("combined collect command must include untruncated tres-alloc output: %q", combinedCollectCommand)
	}
	if strings.Contains(combinedCollectCommand, "%b") {
		t.Fatalf("combined collect command must not rely on %%b for GPU totals: %q", combinedCollectCommand)
	}
}

func TestSplitCombinedOutput(t *testing.T) {
	raw := "node-a\n__SLURM_MONITOR_SPLIT__\n1001|PENDING|alice|1|4G|N/A"
	nodes, queue, err := splitCombinedOutput(raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if nodes != "node-a" {
		t.Fatalf("unexpected nodes payload: %q", nodes)
	}
	if queue != "1001|PENDING|alice|1|4G|N/A" {
		t.Fatalf("unexpected queue payload: %q", queue)
	}
}

func TestFillPendingGPURequestCachePrunesStaleRoots(t *testing.T) {
	c := &Collector{
		pendingGPUCountByJobRoot: map[string]int{
			"1001": 2,
			"2002": 0,
		},
	}

	queueRaw := "2002_1|PENDING|alice|1|4G|N/A"
	c.fillPendingGPURequestCache(context.Background(), queueRaw)

	if len(c.pendingGPUCountByJobRoot) != 1 {
		t.Fatalf("expected exactly one cached root after prune, got %d", len(c.pendingGPUCountByJobRoot))
	}
	if _, ok := c.pendingGPUCountByJobRoot["2002"]; !ok {
		t.Fatalf("expected active root to remain cached")
	}
	if _, ok := c.pendingGPUCountByJobRoot["1001"]; ok {
		t.Fatalf("expected stale root to be pruned")
	}
}

func TestExtractPendingJobRootsAcceptsTrailingDelimiter(t *testing.T) {
	roots := extractPendingJobRoots("2002_1|PENDING|alice|1|4G|N/A|")
	if len(roots) != 1 || roots[0] != "2002" {
		t.Fatalf("unexpected pending roots: %v", roots)
	}
}

type probeCountingTransport struct {
	calls int
}

func (t *probeCountingTransport) Run(context.Context, string) (transport.RunResult, error) {
	t.calls++
	return transport.RunResult{Stdout: "JobId=1 ReqTRES=cpu=1,gres/gpu=1"}, nil
}

func (t *probeCountingTransport) Describe() string {
	return "probe-counter"
}

func TestFillPendingGPURequestCacheOnlyProbesMissingDetailsWithinLimit(t *testing.T) {
	var lines []string
	lines = append(lines, "1|PENDING|alice|1|4G|cpu=1,mem=4G,gres/gpu=1")
	lines = append(lines, "2|PENDING|alice|1|4G|cpu=1,mem=4G")
	for i := 3; i < 3+maxPendingGPUProbesPerCollect+4; i++ {
		lines = append(lines, fmt.Sprintf("%d|PENDING|alice|1|4G|N/A", i))
	}

	tr := &probeCountingTransport{}
	collector := NewCollector(tr, time.Second)
	collector.fillPendingGPURequestCache(context.Background(), strings.Join(lines, "\n"))

	if tr.calls != maxPendingGPUProbesPerCollect {
		t.Fatalf("expected %d bounded fallback probes, got %d", maxPendingGPUProbesPerCollect, tr.calls)
	}
	if _, ok := collector.pendingGPUCountByJobRoot["1"]; ok {
		t.Fatalf("job with complete GPU details must not be probed")
	}
	if _, ok := collector.pendingGPUCountByJobRoot["2"]; ok {
		t.Fatalf("job with complete CPU-only details must not be probed")
	}
}
