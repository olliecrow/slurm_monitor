package slurm

import (
	"context"
	"strings"
	"testing"
)

func TestCombinedCollectCommandExpandsArrayTasks(t *testing.T) {
	if !strings.Contains(combinedCollectCommand, "squeue -h -r ") {
		t.Fatalf("combined collect command must include squeue -r to expand arrays: %q", combinedCollectCommand)
	}
}

func TestSplitCombinedOutput(t *testing.T) {
	raw := "node-a\n__SLURM_MONITOR_SPLIT__\n1001|PENDING|alice|1|4G|N/A|gpu|job|Priority"
	nodes, queue, err := splitCombinedOutput(raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if nodes != "node-a" {
		t.Fatalf("unexpected nodes payload: %q", nodes)
	}
	if queue != "1001|PENDING|alice|1|4G|N/A|gpu|job|Priority" {
		t.Fatalf("unexpected queue payload: %q", queue)
	}
}

func TestFillPendingGPURequestCachePrunesStaleRoots(t *testing.T) {
	c := &Collector{
		pendingGPUByJobRoot: map[string]bool{
			"1001": true,
			"2002": false,
		},
	}

	queueRaw := "2002_1|PENDING|alice|1|4G|N/A|gpu|job|Priority"
	c.fillPendingGPURequestCache(context.Background(), queueRaw)

	if len(c.pendingGPUByJobRoot) != 1 {
		t.Fatalf("expected exactly one cached root after prune, got %d", len(c.pendingGPUByJobRoot))
	}
	if _, ok := c.pendingGPUByJobRoot["2002"]; !ok {
		t.Fatalf("expected active root to remain cached")
	}
	if _, ok := c.pendingGPUByJobRoot["1001"]; ok {
		t.Fatalf("expected stale root to be pruned")
	}
}
