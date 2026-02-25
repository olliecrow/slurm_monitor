package slurm

import "testing"

func TestSortUsersByPendingDemand(t *testing.T) {
	users := []UserSummary{
		{User: "alice", Pending: 3, PendingCPUJobs: 2, PendingGPUJobs: 1, PendingMemMB: 64000},
		{User: "bob", Pending: 1, PendingCPUJobs: 1, PendingGPUJobs: 0, PendingMemMB: 128000},
		{User: "carol", Pending: 2, PendingCPUJobs: 1, PendingGPUJobs: 1, PendingMemMB: 32000},
		{User: "dave", Pending: 0, PendingCPUJobs: 0, PendingGPUJobs: 0, PendingMemMB: 0},
	}

	SortUsersByPendingDemand(users)
	if users[0].User != "alice" {
		t.Fatalf("expected alice first by pending count, got %s", users[0].User)
	}
	if users[1].User != "carol" {
		t.Fatalf("expected carol second by pending count, got %s", users[1].User)
	}
}
