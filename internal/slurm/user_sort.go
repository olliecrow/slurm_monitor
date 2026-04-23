package slurm

import "sort"

// SortUsersForDisplay keeps the biggest current holders near the top while
// still using pending demand as a tie-breaker.
func SortUsersForDisplay(users []UserSummary) {
	sort.Slice(users, func(i, j int) bool {
		if users[i].RunningGPU != users[j].RunningGPU {
			return users[i].RunningGPU > users[j].RunningGPU
		}
		if users[i].RunningCPU != users[j].RunningCPU {
			return users[i].RunningCPU > users[j].RunningCPU
		}
		if users[i].Running != users[j].Running {
			return users[i].Running > users[j].Running
		}
		if users[i].PendingGPU != users[j].PendingGPU {
			return users[i].PendingGPU > users[j].PendingGPU
		}
		if users[i].PendingCPU != users[j].PendingCPU {
			return users[i].PendingCPU > users[j].PendingCPU
		}
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
		return users[i].User < users[j].User
	})
}
