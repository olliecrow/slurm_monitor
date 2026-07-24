package slurm

import "time"

type Node struct {
	Name      string
	State     string
	Partition string

	CPUAlloc int
	CPUTotal int
	CPUUtil  float64
	HasCPU   bool

	MemAllocMB int
	MemTotalMB int
	MemUtil    float64
	HasMem     bool

	GPUAlloc int
	GPUTotal int
	GPUUtil  float64
	HasGPU   bool
}

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

type Snapshot struct {
	Nodes       []Node
	Queue       QueueSummary
	Users       []UserSummary
	CollectedAt time.Time
}

type ResourceTotals struct {
	RunningCPU int
	PendingCPU int

	RunningGPU int
	PendingGPU int
}

type Aggregate struct {
	CPUAlloc int
	CPUTotal int

	MemAllocMB int
	MemTotalMB int

	GPUAlloc int
	GPUTotal int
}

func (s Snapshot) Totals() Aggregate {
	var out Aggregate
	for _, n := range s.Nodes {
		out.CPUAlloc += n.CPUAlloc
		out.CPUTotal += n.CPUTotal
		out.MemAllocMB += n.MemAllocMB
		out.MemTotalMB += n.MemTotalMB
		out.GPUAlloc += n.GPUAlloc
		out.GPUTotal += n.GPUTotal
	}
	return out
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
