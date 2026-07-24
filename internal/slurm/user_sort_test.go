package slurm

import "testing"

func TestSortUsersForDisplay(t *testing.T) {
	users := []UserSummary{
		{User: "alice", RunningGPU: 11, RunningCPU: 60, RunningCPUJobs: 5, RunningGPUJobs: 9, PendingCPUJobs: 2, PendingGPUJobs: 1, PendingCPU: 128, PendingGPU: 2, PendingMemMB: 64000},
		{User: "bob", RunningGPU: 0, RunningCPU: 65, RunningCPUJobs: 1, PendingCPUJobs: 1, PendingCPU: 256, PendingMemMB: 128000},
		{User: "carol", RunningGPU: 8, RunningCPU: 32, RunningCPUJobs: 1, RunningGPUJobs: 1, PendingCPUJobs: 1, PendingGPUJobs: 1, PendingCPU: 64, PendingGPU: 1, PendingMemMB: 32000},
		{User: "dave", PendingCPUJobs: 10, PendingCPU: 400},
	}

	SortUsersForDisplay(users)
	if users[0].User != "alice" {
		t.Fatalf("expected alice first by held gpu, got %s", users[0].User)
	}
	if users[1].User != "carol" {
		t.Fatalf("expected carol second by held gpu, got %s", users[1].User)
	}
	if users[2].User != "bob" {
		t.Fatalf("expected bob ahead of pure-pending user by held cpu, got %s", users[2].User)
	}
}
