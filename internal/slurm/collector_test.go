package slurm

import (
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
