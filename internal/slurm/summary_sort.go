package slurm

import "sort"

// SortPartitionsForDisplay puts the partitions with the most queued GPU and
// CPU work first, followed by the partitions with the largest current load.
func SortPartitionsForDisplay(partitions []PartitionSummary) {
	sort.Slice(partitions, func(i, j int) bool {
		a := partitions[i].Queue
		b := partitions[j].Queue
		comparisons := [...][2]int{
			{a.PendingGPUJobs, b.PendingGPUJobs},
			{a.ResourceLoad.PendingGPU, b.ResourceLoad.PendingGPU},
			{a.PendingCPUJobs, b.PendingCPUJobs},
			{a.ResourceLoad.PendingCPU, b.ResourceLoad.PendingCPU},
			{a.RunningGPUJobs, b.RunningGPUJobs},
			{a.ResourceLoad.RunningGPU, b.ResourceLoad.RunningGPU},
			{a.RunningCPUJobs, b.RunningCPUJobs},
			{a.ResourceLoad.RunningCPU, b.ResourceLoad.RunningCPU},
		}
		for _, comparison := range comparisons {
			if comparison[0] != comparison[1] {
				return comparison[0] > comparison[1]
			}
		}
		return partitions[i].Name < partitions[j].Name
	})
}

// SortPendingReasonsForDisplay surfaces the reasons that strand the largest
// GPU and CPU requests, followed by the reasons that affect the most tasks.
func SortPendingReasonsForDisplay(reasons []PendingReasonSummary) {
	sort.Slice(reasons, func(i, j int) bool {
		a, b := reasons[i], reasons[j]
		if a.GPU != b.GPU {
			return a.GPU > b.GPU
		}
		if a.CPU != b.CPU {
			return a.CPU > b.CPU
		}
		if a.Jobs != b.Jobs {
			return a.Jobs > b.Jobs
		}
		return a.Reason < b.Reason
	})
}

// SortJobsForDisplay surfaces queued GPU work, running GPU work, queued CPU
// work, and running CPU work in that order. Larger grouped jobs sort first.
func SortJobsForDisplay(jobs []JobSummary) {
	sort.Slice(jobs, func(i, j int) bool {
		a, b := jobs[i], jobs[j]
		aRank, bRank := jobDisplayRank(a), jobDisplayRank(b)
		if aRank != bRank {
			return aRank < bRank
		}
		if a.GPU != b.GPU {
			return a.GPU > b.GPU
		}
		if a.CPU != b.CPU {
			return a.CPU > b.CPU
		}
		if a.Tasks != b.Tasks {
			return a.Tasks > b.Tasks
		}
		if a.JobID != b.JobID {
			return a.JobID < b.JobID
		}
		if a.User != b.User {
			return a.User < b.User
		}
		if a.Partition != b.Partition {
			return a.Partition < b.Partition
		}
		if a.State != b.State {
			return a.State < b.State
		}
		return a.Reason < b.Reason
	})
}

func jobDisplayRank(job JobSummary) int {
	switch classifyQueueState(job.State) {
	case "pending":
		if job.GPU > 0 {
			return 0
		}
		return 2
	case "running":
		if job.GPU > 0 {
			return 1
		}
		return 3
	default:
		return 4
	}
}
