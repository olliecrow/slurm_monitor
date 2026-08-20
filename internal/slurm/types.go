package slurm

import "time"

type QueueSummary struct {
	Other int

	RunningCPUJobs int
	RunningGPUJobs int
	PendingCPUJobs int
	PendingGPUJobs int

	ResourceLoad ResourceTotals
}

type UserSummary struct {
	User string

	RunningCPU int
	RunningGPU int

	RunningCPUJobs int
	RunningGPUJobs int

	PendingCPUJobs int
	PendingGPUJobs int

	PendingCPU   int
	PendingMemMB int
	PendingGPU   int
}

type PartitionSummary struct {
	Name  string
	Queue QueueSummary
}

type PendingReasonSummary struct {
	Reason string
	Jobs   int
	CPU    int
	GPU    int
}

// JobSummary groups array tasks from one root job when their user, partition,
// state, and pending reason match. Queue, partition, user, and reason counts
// remain task-granular.
type JobSummary struct {
	JobID     string
	User      string
	Partition string
	State     string
	Reason    string
	Tasks     int
	CPU       int
	GPU       int
}

type Snapshot struct {
	Queue          QueueSummary
	Partitions     []PartitionSummary
	Users          []UserSummary
	PendingReasons []PendingReasonSummary
	Jobs           []JobSummary
	CollectedAt    time.Time
}

type ResourceTotals struct {
	RunningCPU int
	PendingCPU int

	RunningGPU int
	PendingGPU int
}

func (q QueueSummary) TotalJobs() int {
	return q.RunningCPUJobs + q.RunningGPUJobs + q.PendingCPUJobs + q.PendingGPUJobs + q.Other
}

func (u UserSummary) RunningJobs() int {
	return u.RunningCPUJobs + u.RunningGPUJobs
}

func (u UserSummary) PendingJobs() int {
	return u.PendingCPUJobs + u.PendingGPUJobs
}
