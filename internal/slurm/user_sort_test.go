package slurm

import "testing"

func TestSortUsersForDisplay(t *testing.T) {
	users := []UserSummary{
		{User: "alice", RunningGPU: 11, RunningCPU: 60, Running: 14, Pending: 3, PendingCPUJobs: 2, PendingGPUJobs: 1, PendingCPU: 128, PendingGPU: 2, PendingMemMB: 64000},
		{User: "bob", RunningGPU: 0, RunningCPU: 65, Running: 1, Pending: 1, PendingCPUJobs: 1, PendingGPUJobs: 0, PendingCPU: 256, PendingGPU: 0, PendingMemMB: 128000},
		{User: "carol", RunningGPU: 8, RunningCPU: 32, Running: 2, Pending: 2, PendingCPUJobs: 1, PendingGPUJobs: 1, PendingCPU: 64, PendingGPU: 1, PendingMemMB: 32000},
		{User: "dave", RunningGPU: 0, RunningCPU: 0, Running: 0, Pending: 10, PendingCPUJobs: 10, PendingGPUJobs: 0, PendingCPU: 400, PendingGPU: 0, PendingMemMB: 0},
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
