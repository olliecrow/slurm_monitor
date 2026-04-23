package slurm

import "testing"

func TestParseNodeLineBasic(t *testing.T) {
	line := "NodeName=node001 State=IDLE CPUTot=64 CPUAlloc=32 CPULoad=16.00 RealMemory=256000 AllocMem=128000 FreeMem=96000 Partitions=main CfgTRES=cpu=64,mem=256000M,billing=64,gres/gpu=4 AllocTRES=cpu=32,mem=128000M,billing=32,gres/gpu=2"
	node, err := parseNodeLine(line)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if node.Name != "node001" {
		t.Fatalf("unexpected node name %q", node.Name)
	}
	if node.CPUAlloc != 32 || node.CPUTotal != 64 {
		t.Fatalf("unexpected cpu alloc/total: %d/%d", node.CPUAlloc, node.CPUTotal)
	}
	if !node.HasCPU {
		t.Fatalf("expected cpu util available")
	}
	if node.GPUAlloc != 2 || node.GPUTotal != 4 {
		t.Fatalf("unexpected gpu alloc/total: %d/%d", node.GPUAlloc, node.GPUTotal)
	}
	if !node.HasGPU {
		t.Fatalf("expected gpu util availability for non-zero GPU total")
	}
}

func TestParseQueueLines(t *testing.T) {
	raw := "" +
		"1001|RUNNING|alice|8|20G|cpu=8,mem=20G,gres/gpu=1|train|jobA|None\n" +
		"1002|PENDING|alice|4|10G|N/A|train|jobB|Priority\n" +
		"1003|COMPLETING|bob|2|5000M|cpu=2,mem=5000M,gres/gpu=2|dev|jobC|None\n" +
		"1004|PENDING|carol|1|4G|N/A|dev|jobD|Resources\n"
	queue, users := parseQueueLines(raw, nil)
	if queue.Running != 2 || queue.Pending != 2 {
		t.Fatalf("unexpected queue counts: running=%d pending=%d", queue.Running, queue.Pending)
	}
	if queue.RunningCPUJobs != 0 || queue.RunningGPUJobs != 2 {
		t.Fatalf("unexpected queue running cpu/gpu job split: %d/%d", queue.RunningCPUJobs, queue.RunningGPUJobs)
	}
	if queue.PendingCPUJobs != 2 || queue.PendingGPUJobs != 0 {
		t.Fatalf("unexpected queue pending cpu/gpu job split: %d/%d", queue.PendingCPUJobs, queue.PendingGPUJobs)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}
	userMap := make(map[string]UserSummary, len(users))
	for _, u := range users {
		userMap[u.User] = u
	}
	alice, ok := userMap["alice"]
	if !ok {
		t.Fatalf("expected alice user summary")
	}
	if alice.RunningCPU != 8 || alice.RunningGPU != 1 {
		t.Fatalf("unexpected alice running cpu/gpu totals: %d/%d", alice.RunningCPU, alice.RunningGPU)
	}
	if alice.PendingCPU != 4 || alice.PendingMemMB != 10240 || alice.PendingGPU != 0 {
		t.Fatalf("unexpected alice pending demand cpu/mem/gpu: %d/%d/%d", alice.PendingCPU, alice.PendingMemMB, alice.PendingGPU)
	}
	if alice.RunningCPUJobs != 0 || alice.RunningGPUJobs != 1 {
		t.Fatalf("unexpected alice running cpu/gpu job split: %d/%d", alice.RunningCPUJobs, alice.RunningGPUJobs)
	}
	if alice.PendingCPUJobs != 1 || alice.PendingGPUJobs != 0 {
		t.Fatalf("unexpected alice pending cpu/gpu job split: %d/%d", alice.PendingCPUJobs, alice.PendingGPUJobs)
	}
	carol, ok := userMap["carol"]
	if !ok {
		t.Fatalf("expected carol user summary")
	}
	if carol.PendingCPU != 1 || carol.PendingMemMB != 4096 || carol.PendingGPU != 0 {
		t.Fatalf("unexpected carol pending demand cpu/mem/gpu: %d/%d/%d", carol.PendingCPU, carol.PendingMemMB, carol.PendingGPU)
	}
	if carol.PendingCPUJobs != 1 || carol.PendingGPUJobs != 0 {
		t.Fatalf("unexpected carol pending cpu/gpu job split: %d/%d", carol.PendingCPUJobs, carol.PendingGPUJobs)
	}
	for _, u := range users {
		if u.RunningCPUJobs+u.RunningGPUJobs != u.Running {
			t.Fatalf("running cpu/gpu jobs must sum to running for %s: cpu=%d gpu=%d running=%d", u.User, u.RunningCPUJobs, u.RunningGPUJobs, u.Running)
		}
		if u.PendingCPUJobs+u.PendingGPUJobs != u.Pending {
			t.Fatalf("pending cpu/gpu jobs must sum to pending for %s: cpu=%d gpu=%d pending=%d", u.User, u.PendingCPUJobs, u.PendingGPUJobs, u.Pending)
		}
	}
	if queue.RunningCPUJobs+queue.RunningGPUJobs != queue.Running {
		t.Fatalf("queue running cpu/gpu jobs must sum to running: cpu=%d gpu=%d running=%d", queue.RunningCPUJobs, queue.RunningGPUJobs, queue.Running)
	}
	if queue.PendingCPUJobs+queue.PendingGPUJobs != queue.Pending {
		t.Fatalf("queue pending cpu/gpu jobs must sum to pending: cpu=%d gpu=%d pending=%d", queue.PendingCPUJobs, queue.PendingGPUJobs, queue.Pending)
	}
	if queue.ResourceLoad.RunningGPU != 3 {
		t.Fatalf("unexpected running gpu total: %d", queue.ResourceLoad.RunningGPU)
	}
	if len(queue.ByState) == 0 || len(queue.ByPartition) == 0 {
		t.Fatalf("expected non-empty queue mix summaries")
	}
	if len(queue.ByJobName) == 0 {
		t.Fatalf("expected job name mix")
	}
	if len(queue.PendingCause) == 0 {
		t.Fatalf("expected pending causes")
	}
}

func TestParseMemFromTRES(t *testing.T) {
	if got := parseMemMBFromTRES("cpu=8,mem=12G,billing=8"); got != 12288 {
		t.Fatalf("unexpected mem parse: %d", got)
	}
}

func TestParseMemRequestMB(t *testing.T) {
	if got := parseMemRequestMB("20G"); got != 20480 {
		t.Fatalf("unexpected mem request: %d", got)
	}
	if got := parseMemRequestMB("245090M"); got != 245090 {
		t.Fatalf("unexpected mem request: %d", got)
	}
	if got := parseMemRequestMB("500Mc"); got != 500 {
		t.Fatalf("unexpected mem request with suffix: %d", got)
	}
}

func TestParseGPUReq(t *testing.T) {
	if got := parseGPUReq("gres/gpu:2"); got != 2 {
		t.Fatalf("unexpected gpu req: %d", got)
	}
	if got := parseGPUReq("gres/gpu:a100:4,gres/gpu:1"); got != 5 {
		t.Fatalf("unexpected gpu req composite: %d", got)
	}
	if got := parseGPUReq("cpu=8,mem=32G,gres/gpu=2,gres/gpu:a100=4"); got != 6 {
		t.Fatalf("unexpected gpu req from tres style string: %d", got)
	}
}

func TestPendingGPUJobsClassifiedByGPURequest(t *testing.T) {
	raw := "" +
		"2001|PENDING|alice|8|20G|cpu=8,mem=20G,gres/gpu=2|train|gpuJob|Resources\n" +
		"2002|PENDING|alice|4|10G|N/A|train|cpuJob|Priority\n"
	_, users := parseQueueLines(raw, nil)
	if len(users) != 1 {
		t.Fatalf("expected one user, got %d", len(users))
	}
	u := users[0]
	if u.Pending != 2 {
		t.Fatalf("expected 2 pending, got %d", u.Pending)
	}
	if u.RunningCPUJobs != 0 || u.RunningGPUJobs != 0 {
		t.Fatalf("expected no running jobs, got cpu/gpu %d/%d", u.RunningCPUJobs, u.RunningGPUJobs)
	}
	if u.PendingCPUJobs != 1 || u.PendingGPUJobs != 1 {
		t.Fatalf("expected cpu/gpu pending split 1/1, got %d/%d", u.PendingCPUJobs, u.PendingGPUJobs)
	}
}

func TestPendingGPUJobsFallbackByRootJobMap(t *testing.T) {
	raw := "" +
		"37820_1|PENDING|alice|4|64G|N/A|train|mercantile|Priority\n" +
		"37820_2|PENDING|alice|4|64G|N/A|train|mercantile|Priority\n" +
		"37821_1|PENDING|alice|4|64G|N/A|train|cpuJob|Priority\n"

	queue, users := parseQueueLines(raw, map[string]int{"37820": 2})
	if len(users) != 1 {
		t.Fatalf("expected one user, got %d", len(users))
	}
	u := users[0]
	if u.Pending != 3 {
		t.Fatalf("expected 3 pending jobs, got %d", u.Pending)
	}
	if u.PendingGPUJobs != 2 || u.PendingCPUJobs != 1 {
		t.Fatalf("expected pending gpu/cpu jobs 2/1, got %d/%d", u.PendingGPUJobs, u.PendingCPUJobs)
	}
	if u.PendingGPU != 4 {
		t.Fatalf("expected exact pending gpu demand 4, got %d", u.PendingGPU)
	}
	if queue.ResourceLoad.PendingGPU != 4 {
		t.Fatalf("expected queue pending gpu total 4, got %d", queue.ResourceLoad.PendingGPU)
	}
}

func TestParseMemUtil(t *testing.T) {
	pct, ok := parseMemUtil("0", 1024)
	if !ok {
		t.Fatalf("expected FreeMem=0 to be treated as valid")
	}
	if pct != 100 {
		t.Fatalf("expected 100%% utilization for FreeMem=0, got %.2f", pct)
	}

	if pct, ok := parseMemUtil("N/A", 1024); ok || pct != 0 {
		t.Fatalf("expected N/A FreeMem to be unavailable, got pct=%.2f ok=%v", pct, ok)
	}
}

func TestCleanNodeStatePreservesDrainAndDownFlags(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "mixed+drain", want: "MIXED+DRAIN"},
		{in: "idle+down*", want: "IDLE+DOWN"},
		{in: "alloc*", want: "ALLOC"},
	}
	for _, tt := range tests {
		if got := cleanNodeState(tt.in); got != tt.want {
			t.Fatalf("cleanNodeState(%q)=%q want=%q", tt.in, got, tt.want)
		}
	}
}
