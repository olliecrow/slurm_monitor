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
	Running int
	Pending int
	Other   int

	ByState      []StateCount
	ByPartition  []PartitionCount
	ByJobName    []NameCount
	PendingCause []NameCount
	ResourceLoad ResourceTotals
}

type UserSummary struct {
	User    string
	Running int
	Pending int

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

type StateCount struct {
	State string
	Count int
}

type PartitionCount struct {
	Partition string
	Running   int
	Pending   int
	Other     int
}

type NameCount struct {
	Name  string
	Count int
}

type ResourceTotals struct {
	RunningCPU int
	PendingCPU int

	RunningMemMB int
	PendingMemMB int

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
