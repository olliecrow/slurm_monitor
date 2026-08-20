package slurm

import "testing"

func TestSortPartitionsForDisplayPrioritizesPendingPressure(t *testing.T) {
	partitions := []PartitionSummary{
		{Name: "running-gpu", Queue: QueueSummary{RunningGPUJobs: 20, ResourceLoad: ResourceTotals{RunningGPU: 40}}},
		{Name: "pending-cpu", Queue: QueueSummary{PendingCPUJobs: 100, ResourceLoad: ResourceTotals{PendingCPU: 1000}}},
		{Name: "pending-gpu", Queue: QueueSummary{PendingGPUJobs: 1, ResourceLoad: ResourceTotals{PendingGPU: 1}}},
	}

	SortPartitionsForDisplay(partitions)
	if partitions[0].Name != "pending-gpu" || partitions[1].Name != "pending-cpu" || partitions[2].Name != "running-gpu" {
		t.Fatalf("unexpected partition order: %v", []string{partitions[0].Name, partitions[1].Name, partitions[2].Name})
	}
}

func TestSortJobsForDisplayPrioritizesWorkClassAndSize(t *testing.T) {
	jobs := []JobSummary{
		{JobID: "4", State: "RUNNING", CPU: 500},
		{JobID: "3", State: "PENDING", CPU: 100},
		{JobID: "2", State: "RUNNING", CPU: 10, GPU: 8},
		{JobID: "1", State: "PENDING", CPU: 4, GPU: 1},
		{JobID: "5", State: "PENDING", CPU: 8, GPU: 2},
		{JobID: "0", State: "SUSPENDED", CPU: 1000, GPU: 100},
	}

	SortJobsForDisplay(jobs)
	want := []string{"5", "1", "2", "3", "4", "0"}
	for i := range want {
		if jobs[i].JobID != want[i] {
			t.Fatalf("unexpected job order at %d: got %s want %s", i, jobs[i].JobID, want[i])
		}
	}
}
