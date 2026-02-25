package slurm

import "sort"

// SortUsersByPendingDemand orders users by pending demand impact first, then by
// stable identity to keep rendering deterministic.
func SortUsersByPendingDemand(users []UserSummary) {
	sort.Slice(users, func(i, j int) bool {
		if users[i].Pending != users[j].Pending {
			return users[i].Pending > users[j].Pending
		}
		if users[i].PendingGPUJobs != users[j].PendingGPUJobs {
			return users[i].PendingGPUJobs > users[j].PendingGPUJobs
		}
		if users[i].PendingCPUJobs != users[j].PendingCPUJobs {
			return users[i].PendingCPUJobs > users[j].PendingCPUJobs
		}
		if users[i].PendingMemMB != users[j].PendingMemMB {
			return users[i].PendingMemMB > users[j].PendingMemMB
		}
		if users[i].Running != users[j].Running {
			return users[i].Running > users[j].Running
		}
		return users[i].User < users[j].User
	})
}
