package slurm

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var numPrefixRe = regexp.MustCompile(`^-?\d+`)
var gpuResourceRe = regexp.MustCompile(`^(gres/)?gpu(?::([^:=,()]+))?[:=]([0-9]+)`)

func parseAvailableResources(raw string) (AvailableResources, error) {
	var available AvailableResources
	seenNodes := make(map[string]struct{})
	for lineIndex, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := parseKVLine(line)
		nodeName := fields["NodeName"]
		if nodeName == "" {
			return AvailableResources{}, fmt.Errorf("node row %d: missing NodeName", lineIndex+1)
		}
		if _, seen := seenNodes[nodeName]; seen {
			continue
		}
		seenNodes[nodeName] = struct{}{}
		if fields["State"] == "" {
			return AvailableResources{}, fmt.Errorf("node row %d: missing State", lineIndex+1)
		}
		cpuTotalRaw := fields["CPUEfctv"]
		if cpuTotalRaw == "" {
			cpuTotalRaw = fields["CPUTot"]
		}
		if cpuTotalRaw == "" || fields["CPUAlloc"] == "" {
			return AvailableResources{}, fmt.Errorf("node row %d: missing CPU capacity fields", lineIndex+1)
		}

		available.TotalNodes++
		if !nodeIsSchedulable(fields["State"]) {
			continue
		}
		available.SchedulableNodes++
		available.CPU += max(0, parseInt(cpuTotalRaw)-parseInt(fields["CPUAlloc"]))

		gpuTotalRaw := fields["CfgTRES"]
		if parseGPUCount(gpuTotalRaw) == 0 {
			gpuTotalRaw = fields["Gres"]
		}
		gpuAllocRaw := fields["AllocTRES"]
		if parseGPUCount(gpuAllocRaw) == 0 {
			gpuAllocRaw = fields["GresUsed"]
		}
		available.GPU += max(0, parseGPUCount(gpuTotalRaw)-parseGPUCount(gpuAllocRaw))
	}
	return available, nil
}

func parseKVLine(line string) map[string]string {
	out := make(map[string]string)
	for _, token := range strings.Fields(line) {
		parts := strings.SplitN(token, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

func nodeIsSchedulable(rawState string) bool {
	state := strings.ToUpper(strings.TrimSpace(rawState))
	state = strings.TrimRight(state, "*~#%$@^!-")
	parts := strings.Split(state, "+")
	if parts[0] != "IDLE" && parts[0] != "MIXED" {
		return false
	}
	for _, flag := range parts[1:] {
		if flag != "DYNAMIC" && flag != "DYNAMIC_NORM" {
			return false
		}
	}
	return true
}

type queueData struct {
	Queue          QueueSummary
	Partitions     []PartitionSummary
	Users          []UserSummary
	PendingReasons []PendingReasonSummary
	Jobs           []JobSummary
}

type jobSummaryKey struct {
	jobID     string
	user      string
	partition string
	state     string
	reason    string
}

func parseQueueLines(raw string, pendingGPUCountByJobRoot map[string]int) (queueData, error) {
	lines := strings.Split(raw, "\n")
	users := make(map[string]*UserSummary)
	partitions := make(map[string]*PartitionSummary)
	pendingReasons := make(map[string]*PendingReasonSummary)
	jobs := make(map[jobSummaryKey]*JobSummary)
	var queue QueueSummary

	for lineIndex, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := splitQueueRow(line)
		if len(parts) < 8 {
			return queueData{}, fmt.Errorf("queue row %d: expected 8 fields, got %d", lineIndex+1, len(parts))
		}
		jobID := strings.TrimSpace(parts[0])
		state := strings.ToUpper(strings.TrimSpace(parts[1]))
		user := strings.TrimSpace(parts[2])
		partition := strings.TrimSpace(parts[3])
		cpuReq := parseInt(strings.TrimSpace(parts[4]))
		memReq := strings.TrimSpace(parts[5])
		gresReq := strings.TrimSpace(parts[6])
		reason := strings.TrimSpace(parts[7])
		if jobID == "" {
			jobID = "<unknown>"
		}
		if user == "" {
			user = "<unknown>"
		}
		if partition == "" {
			partition = "<unknown>"
		}

		if _, ok := users[user]; !ok {
			users[user] = &UserSummary{User: user}
		}
		if _, ok := partitions[partition]; !ok {
			partitions[partition] = &PartitionSummary{Name: partition}
		}

		stateClass := classifyQueueState(state)
		gpuReq := parseGPUCount(gresReq)
		isGPUJob := gpuReq > 0
		memReqMB := 0
		if stateClass == "pending" {
			if reason == "" || strings.EqualFold(reason, "N/A") {
				reason = "<unknown>"
			}
			memReqMB = parseMemRequestMB(memReq, cpuReq)
			if !isGPUJob {
				if fallbackGPUCount := pendingGPUCountByJobRoot[rootJobID(jobID)]; fallbackGPUCount > 0 {
					isGPUJob = true
					gpuReq = fallbackGPUCount
				}
			}
		} else {
			reason = ""
		}

		addQueueItem(&queue, stateClass, cpuReq, gpuReq, isGPUJob)
		addQueueItem(&partitions[partition].Queue, stateClass, cpuReq, gpuReq, isGPUJob)
		if stateClass == "pending" {
			reasonSummary := pendingReasons[reason]
			if reasonSummary == nil {
				reasonSummary = &PendingReasonSummary{Reason: reason}
				pendingReasons[reason] = reasonSummary
			}
			reasonSummary.Tasks++
			reasonSummary.CPU += cpuReq
			reasonSummary.GPU += gpuReq
		}

		switch stateClass {
		case "running":
			users[user].RunningCPU += cpuReq
			users[user].RunningGPU += gpuReq
			if isGPUJob {
				users[user].RunningGPUJobs++
			} else {
				users[user].RunningCPUJobs++
			}
		case "pending":
			if isGPUJob {
				users[user].PendingGPUJobs++
			} else {
				users[user].PendingCPUJobs++
			}
			users[user].PendingCPU += cpuReq
			users[user].PendingMemMB += memReqMB
			users[user].PendingGPU += gpuReq
		}
		jobKey := jobSummaryKey{
			jobID:     rootJobID(jobID),
			user:      user,
			partition: partition,
			state:     state,
			reason:    reason,
		}
		job := jobs[jobKey]
		if job == nil {
			job = &JobSummary{
				JobID:     jobKey.jobID,
				User:      user,
				Partition: partition,
				State:     state,
				Reason:    reason,
			}
			jobs[jobKey] = job
		}
		job.Tasks++
		job.CPU += cpuReq
		job.GPU += gpuReq
	}

	outUsers := make([]UserSummary, 0, len(users))
	for _, v := range users {
		outUsers = append(outUsers, *v)
	}
	outPartitions := make([]PartitionSummary, 0, len(partitions))
	for _, partition := range partitions {
		outPartitions = append(outPartitions, *partition)
	}
	outJobs := make([]JobSummary, 0, len(jobs))
	for _, job := range jobs {
		outJobs = append(outJobs, *job)
	}
	outPendingReasons := make([]PendingReasonSummary, 0, len(pendingReasons))
	for _, reason := range pendingReasons {
		outPendingReasons = append(outPendingReasons, *reason)
	}

	return queueData{
		Queue:          queue,
		Partitions:     outPartitions,
		Users:          outUsers,
		PendingReasons: outPendingReasons,
		Jobs:           outJobs,
	}, nil
}

func addQueueItem(queue *QueueSummary, stateClass string, cpu, gpu int, isGPUJob bool) {
	switch stateClass {
	case "running":
		if isGPUJob {
			queue.RunningGPUJobs++
		} else {
			queue.RunningCPUJobs++
		}
		queue.ResourceLoad.RunningCPU += cpu
		queue.ResourceLoad.RunningGPU += gpu
	case "pending":
		if isGPUJob {
			queue.PendingGPUJobs++
		} else {
			queue.PendingCPUJobs++
		}
		queue.ResourceLoad.PendingCPU += cpu
		queue.ResourceLoad.PendingGPU += gpu
	default:
		queue.Other++
	}
}

func splitQueueRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, "|")
	return strings.SplitN(line, "|", 8)
}

func rootJobID(jobID string) string {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ""
	}
	if idx := strings.IndexByte(jobID, '_'); idx > 0 {
		jobID = jobID[:idx]
	}
	if idx := strings.IndexByte(jobID, '.'); idx > 0 {
		jobID = jobID[:idx]
	}
	return jobID
}

func classifyQueueState(state string) string {
	switch {
	case strings.Contains(state, "PENDING"):
		return "pending"
	case strings.Contains(state, "RUNNING"):
		return "running"
	case strings.Contains(state, "COMPLETING"):
		return "running"
	case strings.Contains(state, "CONFIGURING"):
		return "running"
	default:
		return "other"
	}
}

func parseInt(v string) int {
	if v == "" {
		return 0
	}
	match := numPrefixRe.FindString(v)
	if match == "" {
		return 0
	}
	n, err := strconv.Atoi(match)
	if err != nil {
		return 0
	}
	return n
}

func parseGPUCount(tres string) int {
	if tres == "" {
		return 0
	}
	genericTotal := 0
	typedTotal := 0
	hasGeneric := false
	for _, part := range strings.Split(tres, ",") {
		part = strings.TrimSpace(part)
		matches := gpuResourceRe.FindStringSubmatch(part)
		if len(matches) != 4 {
			continue
		}
		count := parseInt(matches[3])
		if matches[2] != "" {
			typedTotal += count
			continue
		}
		hasGeneric = true
		genericTotal += count
	}
	if hasGeneric {
		return genericTotal
	}
	return typedTotal
}

func parseMemRequestMB(raw string, cpuCount int) int {
	if raw == "" || raw == "N/A" {
		return 0
	}
	// Slurm appends c/n for per-CPU/per-node memory requests.
	last := raw[len(raw)-1]
	unit := byte(0)
	numPart := raw
	perCPU := last == 'c' || last == 'C'
	switch last {
	case 'c', 'C', 'n', 'N':
		numPart = raw[:len(raw)-1]
	}
	if len(numPart) == 0 {
		return 0
	}
	last = numPart[len(numPart)-1]
	switch last {
	case 'K', 'k', 'M', 'm', 'G', 'g', 'T', 't':
		unit = last
		numPart = numPart[:len(numPart)-1]
	}
	value := parseInt(numPart)
	var megabytes int
	switch unit {
	case 'K', 'k':
		megabytes = value / 1024
	case 'M', 'm':
		megabytes = value
	case 'G', 'g':
		megabytes = value * 1024
	case 'T', 't':
		megabytes = value * 1024 * 1024
	default:
		megabytes = value
	}
	if perCPU && cpuCount > 0 {
		return megabytes * cpuCount
	}
	return megabytes
}
