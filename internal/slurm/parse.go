package slurm

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var numPrefixRe = regexp.MustCompile(`^-?\d+`)
var gpuReqRe = regexp.MustCompile(`gpu(?::[a-zA-Z0-9_-]+)?:([0-9]+)`)

func parseNodeLines(raw string) ([]Node, error) {
	lines := strings.Split(raw, "\n")
	out := make([]Node, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		node, err := parseNodeLine(line)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func parseNodeLine(line string) (Node, error) {
	fields := parseKVLine(line)
	name := fields["NodeName"]
	if name == "" {
		return Node{}, fmt.Errorf("missing NodeName in line: %s", line)
	}

	cpuAlloc := parseInt(fields["CPUAlloc"])
	cpuTotal := parseInt(fields["CPUTot"])
	memAlloc := parseInt(fields["AllocMem"])
	memTotal := parseInt(fields["RealMemory"])

	if memAlloc == 0 {
		memAlloc = parseMemMBFromTRES(fields["AllocTRES"])
	}

	cpuUtil, hasCPU := parseCPUUtil(fields["CPULoad"], cpuTotal)
	memUtil, hasMem := parseMemUtil(fields["FreeMem"], memTotal)

	gpuAlloc := parseGPUCount(fields["AllocTRES"])
	gpuTotal := parseGPUCount(fields["CfgTRES"])
	gpuUtil, hasGPU := allocUtilPct(gpuAlloc, gpuTotal)

	state := cleanNodeState(fields["State"])
	if state == "" {
		state = "UNKNOWN"
	}

	return Node{
		Name:       name,
		State:      state,
		Partition:  fields["Partitions"],
		CPUAlloc:   cpuAlloc,
		CPUTotal:   cpuTotal,
		CPUUtil:    cpuUtil,
		HasCPU:     hasCPU,
		MemAllocMB: memAlloc,
		MemTotalMB: memTotal,
		MemUtil:    memUtil,
		HasMem:     hasMem,
		GPUAlloc:   gpuAlloc,
		GPUTotal:   gpuTotal,
		GPUUtil:    gpuUtil,
		HasGPU:     hasGPU,
	}, nil
}

func parseQueueLines(raw string, pendingGPUByJobRoot map[string]bool) (QueueSummary, []UserSummary) {
	lines := strings.Split(raw, "\n")
	users := make(map[string]*UserSummary)
	partitionMap := make(map[string]*PartitionCount)
	stateMap := make(map[string]int)
	jobNameMap := make(map[string]int)
	pendingReasonMap := make(map[string]int)
	var queue QueueSummary

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 9)
		if len(parts) < 9 {
			continue
		}
		jobID := strings.TrimSpace(parts[0])
		state := strings.ToUpper(strings.TrimSpace(parts[1]))
		user := strings.TrimSpace(parts[2])
		cpuReq := parseInt(strings.TrimSpace(parts[3]))
		memReqMB := parseMemRequestMB(strings.TrimSpace(parts[4]))
		gresReq := strings.TrimSpace(parts[5])
		partition := strings.TrimSpace(parts[6])
		jobName := strings.TrimSpace(parts[7])
		reason := strings.TrimSpace(parts[8])
		if user == "" {
			user = "<unknown>"
		}
		if partition == "" {
			partition = "<unknown>"
		}
		if jobName == "" || jobName == "N/A" {
			jobName = "<unnamed>"
		}

		if _, ok := users[user]; !ok {
			users[user] = &UserSummary{User: user}
		}
		if _, ok := partitionMap[partition]; !ok {
			partitionMap[partition] = &PartitionCount{Partition: partition}
		}

		stateClass := classifyQueueState(state)
		stateMap[state]++
		jobNameMap[jobName]++
		gpuReq := parseGPUReq(gresReq)

		switch stateClass {
		case "running":
			queue.Running++
			users[user].Running++
			partitionMap[partition].Running++
			queue.ResourceLoad.RunningCPU += cpuReq
			queue.ResourceLoad.RunningMemMB += memReqMB
			queue.ResourceLoad.RunningGPU += gpuReq
		case "pending":
			queue.Pending++
			users[user].Pending++
			isGPUJob := gpuReq > 0
			if !isGPUJob {
				if pendingGPUByJobRoot[rootJobID(jobID)] {
					isGPUJob = true
				}
			}
			if isGPUJob {
				users[user].PendingGPUJobs++
			} else {
				users[user].PendingCPUJobs++
			}
			users[user].PendingCPU += cpuReq
			users[user].PendingMemMB += memReqMB
			users[user].PendingGPU += gpuReq
			partitionMap[partition].Pending++
			queue.ResourceLoad.PendingCPU += cpuReq
			queue.ResourceLoad.PendingMemMB += memReqMB
			queue.ResourceLoad.PendingGPU += gpuReq
			if reason == "" {
				reason = "<unknown>"
			}
			pendingReasonMap[reason]++
		default:
			queue.Other++
			partitionMap[partition].Other++
		}
	}

	outUsers := make([]UserSummary, 0, len(users))
	for _, v := range users {
		outUsers = append(outUsers, *v)
	}
	SortUsersByPendingDemand(outUsers)

	queue.ByState = mapToStateCounts(stateMap)
	queue.ByPartition = mapToPartitionCounts(partitionMap)
	queue.ByJobName = mapToNameCounts(jobNameMap)
	queue.PendingCause = mapToNameCounts(pendingReasonMap)

	return queue, outUsers
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

func parseKVLine(line string) map[string]string {
	out := make(map[string]string)
	for _, token := range strings.Fields(line) {
		parts := strings.SplitN(token, "=", 2)
		if len(parts) != 2 {
			continue
		}
		out[parts[0]] = parts[1]
	}
	return out
}

func cleanNodeState(v string) string {
	if v == "" {
		return ""
	}
	v = strings.Split(v, "+")[0]
	v = strings.Split(v, "*")[0]
	return strings.ToUpper(strings.TrimSpace(v))
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

func parseFloat(v string) (float64, bool) {
	if v == "" || v == "N/A" || v == "(null)" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func parseCPUUtil(cpuLoadRaw string, cpuTotal int) (float64, bool) {
	load, ok := parseFloat(cpuLoadRaw)
	if !ok || cpuTotal <= 0 {
		return 0, false
	}
	pct := (load / float64(cpuTotal)) * 100.0
	if math.IsNaN(pct) || math.IsInf(pct, 0) {
		return 0, false
	}
	if pct < 0 {
		pct = 0
	}
	return pct, true
}

func parseMemUtil(freeMemRaw string, totalMem int) (float64, bool) {
	if totalMem <= 0 {
		return 0, false
	}
	trimmed := strings.TrimSpace(freeMemRaw)
	if trimmed == "" || trimmed == "N/A" || trimmed == "(null)" {
		return 0, false
	}

	match := numPrefixRe.FindString(trimmed)
	if match == "" {
		return 0, false
	}
	freeMem, err := strconv.Atoi(match)
	if err != nil || freeMem < 0 {
		return 0, false
	}

	used := totalMem - freeMem
	if used < 0 {
		used = 0
	}
	pct := (float64(used) / float64(totalMem)) * 100.0
	return pct, true
}

func allocUtilPct(alloc, total int) (float64, bool) {
	if total <= 0 {
		return 0, false
	}
	if alloc < 0 {
		alloc = 0
	}
	pct := (float64(alloc) / float64(total)) * 100.0
	if pct > 100.0 {
		pct = 100.0
	}
	return pct, true
}

func parseGPUCount(tres string) int {
	if tres == "" {
		return 0
	}
	total := 0
	for _, part := range strings.Split(tres, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		if !strings.HasPrefix(key, "gres/gpu") {
			continue
		}
		total += parseInt(strings.TrimSpace(kv[1]))
	}
	return total
}

func parseMemMBFromTRES(tres string) int {
	if tres == "" {
		return 0
	}
	for _, part := range strings.Split(tres, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if strings.TrimSpace(kv[0]) != "mem" {
			continue
		}
		val := strings.TrimSpace(kv[1])
		if val == "" {
			return 0
		}
		unit := val[len(val)-1]
		num := parseInt(val)
		switch unit {
		case 'K', 'k':
			return num / 1024
		case 'M', 'm':
			return num
		case 'G', 'g':
			return num * 1024
		case 'T', 't':
			return num * 1024 * 1024
		default:
			return num
		}
	}
	return 0
}

func parseMemRequestMB(raw string) int {
	if raw == "" || raw == "N/A" {
		return 0
	}
	// Slurm may append c/n for per-cpu/per-node semantics; we treat value as MB-equivalent scalar.
	last := raw[len(raw)-1]
	unit := byte(0)
	numPart := raw
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
	switch unit {
	case 'K', 'k':
		return value / 1024
	case 'M', 'm':
		return value
	case 'G', 'g':
		return value * 1024
	case 'T', 't':
		return value * 1024 * 1024
	default:
		return value
	}
}

func parseGPUReq(raw string) int {
	if raw == "" || raw == "N/A" {
		return 0
	}
	matches := gpuReqRe.FindAllStringSubmatch(raw, -1)
	total := 0
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		total += parseInt(m[1])
	}
	return total
}

func mapToStateCounts(m map[string]int) []StateCount {
	out := make([]StateCount, 0, len(m))
	for state, count := range m {
		out = append(out, StateCount{State: state, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].State < out[j].State
	})
	return out
}

func mapToPartitionCounts(m map[string]*PartitionCount) []PartitionCount {
	out := make([]PartitionCount, 0, len(m))
	for _, p := range m {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		iTotal := out[i].Running + out[i].Pending + out[i].Other
		jTotal := out[j].Running + out[j].Pending + out[j].Other
		if iTotal != jTotal {
			return iTotal > jTotal
		}
		return out[i].Partition < out[j].Partition
	})
	return out
}

func mapToNameCounts(m map[string]int) []NameCount {
	out := make([]NameCount, 0, len(m))
	for name, count := range m {
		out = append(out, NameCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}
