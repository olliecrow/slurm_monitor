package slurm

import (
	"strings"
	"testing"
)

func TestParseQueueLines(t *testing.T) {
	raw := "" +
		"1001|RUNNING|alice|gpu|8|20G|cpu=8,mem=20G,gres/gpu=1|None\n" +
		"1002|PENDING|alice|cpu|4|10G|N/A|Priority\n" +
		"1003|COMPLETING|bob|gpu|2|5000M|cpu=2,mem=5000M,gres/gpu=2|None\n" +
		"1004|PENDING|carol|cpu|1|4G|N/A|Resources\n"
	data, err := parseQueueLines(raw, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	queue, users := data.Queue, data.Users
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
	if queue.TotalJobs() != 4 {
		t.Fatalf("unexpected total jobs: %d", queue.TotalJobs())
	}
	if queue.ResourceLoad.RunningGPU != 3 {
		t.Fatalf("unexpected running gpu total: %d", queue.ResourceLoad.RunningGPU)
	}
}

func TestParseQueueLinesRejectsMalformedRow(t *testing.T) {
	_, err := parseQueueLines("1001|RUNNING|alice", nil)
	if err == nil {
		t.Fatal("expected malformed queue row error")
	}
	if !strings.Contains(err.Error(), "expected 8 fields, got 3") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseQueueLinesAcceptsTrailingDelimiter(t *testing.T) {
	data, err := parseQueueLines("1001|RUNNING|alice|gpu|8|20G|cpu=8,mem=20G,gres/gpu=1|None|", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	queue := data.Queue
	if queue.RunningGPUJobs != 1 || queue.ResourceLoad.RunningGPU != 1 {
		t.Fatalf("unexpected GPU queue totals: jobs=%d resources=%d", queue.RunningGPUJobs, queue.ResourceLoad.RunningGPU)
	}
}

func TestParseQueueLinesBuildsPartitionAndGroupedJobSummaries(t *testing.T) {
	raw := "" +
		"3001_1|PENDING|alice|gpu|4|16G|cpu=4,mem=16G,gres/gpu=1|Resources\n" +
		"3001_2|PENDING|alice|gpu|4|16G|cpu=4,mem=16G,gres/gpu=1|Resources\n" +
		"3002|RUNNING|bob|cpu|8|32G|cpu=8,mem=32G|None\n"

	data, err := parseQueueLines(raw, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(data.Partitions) != 2 {
		t.Fatalf("expected two partitions, got %d", len(data.Partitions))
	}
	partitionByName := make(map[string]PartitionSummary, len(data.Partitions))
	for _, partition := range data.Partitions {
		partitionByName[partition.Name] = partition
	}
	if got := partitionByName["gpu"].Queue.PendingGPUJobs; got != 2 {
		t.Fatalf("expected two task-granular pending GPU jobs, got %d", got)
	}
	if got := partitionByName["gpu"].Queue.ResourceLoad.PendingGPU; got != 2 {
		t.Fatalf("expected two pending GPUs in partition total, got %d", got)
	}
	if got := partitionByName["cpu"].Queue.RunningCPUJobs; got != 1 {
		t.Fatalf("expected one running CPU job, got %d", got)
	}

	if len(data.Jobs) != 2 {
		t.Fatalf("expected two grouped root jobs, got %d", len(data.Jobs))
	}
	jobsByID := make(map[string]JobSummary, len(data.Jobs))
	for _, job := range data.Jobs {
		jobsByID[job.JobID] = job
	}
	arrayJob := jobsByID["3001"]
	if arrayJob.Tasks != 2 || arrayJob.CPU != 8 || arrayJob.GPU != 2 {
		t.Fatalf("unexpected grouped array totals: %+v", arrayJob)
	}
	if arrayJob.User != "alice" || arrayJob.Partition != "gpu" || arrayJob.State != "PENDING" {
		t.Fatalf("unexpected grouped array identity: %+v", arrayJob)
	}
}

func TestParseQueueLinesKeepsDifferentArrayTaskStatesSeparate(t *testing.T) {
	raw := "" +
		"4001_1|RUNNING|alice|gpu|4|16G|cpu=4,mem=16G,gres/gpu=1|None\n" +
		"4001_2|PENDING|alice|gpu|4|16G|cpu=4,mem=16G,gres/gpu=1|Resources\n"

	data, err := parseQueueLines(raw, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(data.Jobs) != 2 {
		t.Fatalf("expected separate running and pending groups, got %d", len(data.Jobs))
	}
	states := make(map[string]int, len(data.Jobs))
	for _, job := range data.Jobs {
		if job.JobID != "4001" || job.Tasks != 1 {
			t.Fatalf("unexpected mixed-state group: %+v", job)
		}
		states[job.State]++
	}
	if states["RUNNING"] != 1 || states["PENDING"] != 1 {
		t.Fatalf("unexpected mixed-state groups: %v", states)
	}
}

func TestParseMemRequestMB(t *testing.T) {
	if got := parseMemRequestMB("20G", 1); got != 20480 {
		t.Fatalf("unexpected mem request: %d", got)
	}
	if got := parseMemRequestMB("245090M", 1); got != 245090 {
		t.Fatalf("unexpected mem request: %d", got)
	}
	if got := parseMemRequestMB("500Mc", 4); got != 2000 {
		t.Fatalf("unexpected per-cpu mem request: %d", got)
	}
	if got := parseMemRequestMB("500Mn", 4); got != 500 {
		t.Fatalf("unexpected per-node mem request: %d", got)
	}
}

func TestParseGPUCount(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "generic gres", raw: "gres/gpu:2", want: 2},
		{name: "typed gres", raw: "gres/gpu:a100:4,gres/gpu:h100:1", want: 5},
		{name: "generic tres", raw: "cpu=8,mem=32G,gres/gpu=2", want: 2},
		{name: "typed tres", raw: "gres/gpu:a100=4,gres/gpu:h100=1", want: 5},
		{name: "generic total takes precedence over typed breakdown", raw: "gres/gpu=5,gres/gpu:a100=4,gres/gpu:h100=1", want: 5},
		{name: "gpu accounting metrics are excluded", raw: "gres/gpumem=4096M,gres/gpuutil=100", want: 0},
		{name: "allocation suffix", raw: "gres/gpu:2(IDX:0-1)", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseGPUCount(tt.raw); got != tt.want {
				t.Fatalf("parseGPUCount(%q)=%d want=%d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestPendingGPUJobsClassifiedByGPURequest(t *testing.T) {
	raw := "" +
		"2001|PENDING|alice|gpu|8|20G|cpu=8,mem=20G,gres/gpu=2|Resources\n" +
		"2002|PENDING|alice|cpu|4|10G|N/A|Priority\n"
	data, err := parseQueueLines(raw, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	users := data.Users
	if len(users) != 1 {
		t.Fatalf("expected one user, got %d", len(users))
	}
	u := users[0]
	if u.PendingJobs() != 2 {
		t.Fatalf("expected 2 pending, got %d", u.PendingJobs())
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
		"37820_1|PENDING|alice|gpu|4|64G|N/A|Resources\n" +
		"37820_2|PENDING|alice|gpu|4|64G|N/A|Resources\n" +
		"37821_1|PENDING|alice|cpu|4|64G|N/A|Priority\n"

	data, err := parseQueueLines(raw, map[string]int{"37820": 2})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	queue, users := data.Queue, data.Users
	if len(users) != 1 {
		t.Fatalf("expected one user, got %d", len(users))
	}
	u := users[0]
	if u.PendingJobs() != 3 {
		t.Fatalf("expected 3 pending jobs, got %d", u.PendingJobs())
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
	reasons := make(map[string]PendingReasonSummary, len(data.PendingReasons))
	for _, reason := range data.PendingReasons {
		reasons[reason.Reason] = reason
	}
	if reasons["Resources"].GPU != 4 {
		t.Fatalf("expected fallback GPU demand in reason summary, got %+v", reasons["Resources"])
	}
}

func TestParseQueueLinesAggregatesPendingReasons(t *testing.T) {
	raw := "" +
		"5001_1|PENDING|alice|gpu|4|16G|cpu=4,gres/gpu=1|Resources\n" +
		"5001_2|PENDING|alice|gpu|4|16G|cpu=4,gres/gpu=1|Resources\n" +
		"5002|PENDING|bob|cpu|8|32G|N/A|Priority\n" +
		"5003|PENDING|bob|cpu|2|8G|N/A|N/A\n" +
		"5004|RUNNING|carol|cpu|16|64G|cpu=16|None\n"

	data, err := parseQueueLines(raw, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	got := make(map[string]PendingReasonSummary, len(data.PendingReasons))
	for _, reason := range data.PendingReasons {
		got[reason.Reason] = reason
	}
	if got["Resources"] != (PendingReasonSummary{Reason: "Resources", Jobs: 2, CPU: 8, GPU: 2}) {
		t.Fatalf("unexpected Resources summary: %+v", got["Resources"])
	}
	if got["Priority"] != (PendingReasonSummary{Reason: "Priority", Jobs: 1, CPU: 8}) {
		t.Fatalf("unexpected Priority summary: %+v", got["Priority"])
	}
	if got["<unknown>"].Jobs != 1 || len(got) != 3 {
		t.Fatalf("unexpected reason summaries: %+v", got)
	}
}

func TestParseQueueLinesKeepsDifferentPendingReasonsSeparate(t *testing.T) {
	raw := "" +
		"6001_1|PENDING|alice|gpu|4|16G|cpu=4,gres/gpu=1|Resources\n" +
		"6001_2|PENDING|alice|gpu|4|16G|cpu=4,gres/gpu=1|Priority\n"

	data, err := parseQueueLines(raw, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(data.Jobs) != 2 {
		t.Fatalf("expected two pending-reason groups, got %d", len(data.Jobs))
	}
}
