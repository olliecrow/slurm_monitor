package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"slurm_monitor/internal/monitor"
	"slurm_monitor/internal/slurm"
)

func TestViewFitsViewportAcrossSizes(t *testing.T) {
	sizes := []struct {
		width  int
		height int
	}{
		{width: 72, height: 20},
		{width: 90, height: 24},
		{width: 110, height: 30},
		{width: 150, height: 42},
	}

	for _, size := range sizes {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			m := seededModel()
			m.width = size.width
			m.height = size.height
			out := m.View()
			assertViewportBounds(t, out, size.width, size.height)
		})
	}
}

func TestUpdateStoresLatestSnapshot(t *testing.T) {
	m := NewModel(Options{
		Source:  "ssh:test",
		Refresh: 2 * time.Second,
		Updates: make(chan monitor.Update),
	})
	snap := sampleSnapshot()

	next, _ := m.Update(updateMsg{update: monitor.Update{
		Snapshot:    &snap,
		State:       monitor.StateConnected,
		LastSuccess: snap.CollectedAt,
	}})
	got := next.(Model)
	if got.snapshot == nil {
		t.Fatalf("expected snapshot to be stored")
	}
	if got.lastError != "" {
		t.Fatalf("expected lastError cleared after successful snapshot")
	}
}

func TestHeaderContainsLiveClock(t *testing.T) {
	m := seededModel()
	t1 := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Second)

	h1 := m.renderHeader(t1)
	h2 := m.renderHeader(t2)
	if !strings.Contains(h1, "clock: 10:00:00") {
		t.Fatalf("expected header to include first clock value")
	}
	if !strings.Contains(h2, "clock: 10:00:01") {
		t.Fatalf("expected header to include second clock value")
	}
	if h1 == h2 {
		t.Fatalf("expected header to change between ticks")
	}
}

func TestQueueSummaryRendersWithoutBars(t *testing.T) {
	m := seededModel()
	out := m.renderQueuePanel(4, true)
	if strings.Contains(out, "█") || strings.Contains(out, "░") {
		t.Fatalf("expected queue summary without bar glyphs, got: %q", out)
	}
	if !strings.Contains(out, "running") || !strings.Contains(out, "pending") || !strings.Contains(out, "other") || !strings.Contains(out, "total") {
		t.Fatalf("expected queue summary labels in output, got: %q", out)
	}
}

func TestCompactViewIncludesPendingDemandColumnsWhenWidthAllows(t *testing.T) {
	m := seededModel()
	m.compact = true
	m.width = 90
	m.height = 36

	out := m.View()
	if !strings.Contains(out, "pendingCPUJobs") || !strings.Contains(out, "pendingGPUJobs") {
		t.Fatalf("expected compact view to include pending demand columns, got: %q", out)
	}
}

func TestClipToViewportPadsToFullFrame(t *testing.T) {
	out := clipToViewport("abc\ndef", 6, 4)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected exactly 4 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if lipgloss.Width(line) != 6 {
			t.Fatalf("expected line %d width 6, got %d", i+1, lipgloss.Width(line))
		}
	}
}

func seededModel() Model {
	now := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)
	snap := sampleSnapshot()
	m := NewModel(Options{
		Source:  "ssh:cluster_alias",
		Refresh: 2 * time.Second,
		Updates: make(chan monitor.Update),
	})
	m.state = monitor.StateConnected
	m.now = now
	m.lastSuccess = now
	m.snapshot = &snap
	m.width = 180
	m.height = 40
	return m
}

func sampleSnapshot() slurm.Snapshot {
	return slurm.Snapshot{
		CollectedAt: time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC),
		Nodes: []slurm.Node{
			{
				Name:       "gpu-a100-01",
				State:      "ALLOCATED",
				Partition:  "gpu",
				CPUAlloc:   96,
				CPUTotal:   128,
				CPUUtil:    73.3,
				HasCPU:     true,
				MemAllocMB: 512000,
				MemTotalMB: 768000,
				MemUtil:    70.0,
				HasMem:     true,
				GPUAlloc:   6,
				GPUTotal:   8,
				GPUUtil:    75.0,
				HasGPU:     true,
			},
			{
				Name:       "gpu-a100-02",
				State:      "MIXED",
				Partition:  "gpu",
				CPUAlloc:   64,
				CPUTotal:   128,
				CPUUtil:    51.4,
				HasCPU:     true,
				MemAllocMB: 420000,
				MemTotalMB: 768000,
				MemUtil:    54.7,
				HasMem:     true,
				GPUAlloc:   4,
				GPUTotal:   8,
				GPUUtil:    50.0,
				HasGPU:     true,
			},
			{
				Name:       "cpu-large-01",
				State:      "IDLE",
				Partition:  "cpu",
				CPUAlloc:   0,
				CPUTotal:   128,
				CPUUtil:    2.1,
				HasCPU:     true,
				MemAllocMB: 0,
				MemTotalMB: 512000,
				MemUtil:    11.0,
				HasMem:     true,
				GPUAlloc:   0,
				GPUTotal:   0,
				GPUUtil:    0,
				HasGPU:     false,
			},
		},
		Queue: slurm.QueueSummary{
			Running: 42,
			Pending: 5,
			Other:   1,
			ByState: []slurm.StateCount{
				{State: "RUNNING", Count: 42},
				{State: "PENDING", Count: 5},
				{State: "COMPLETING", Count: 1},
			},
			ByPartition: []slurm.PartitionCount{
				{Partition: "gpu", Running: 34, Pending: 4},
				{Partition: "cpu", Running: 8, Pending: 1},
			},
			ByJobName: []slurm.NameCount{
				{Name: "train_large", Count: 11},
				{Name: "preprocess", Count: 8},
			},
			PendingCause: []slurm.NameCount{
				{Name: "Priority", Count: 3},
				{Name: "Resources", Count: 2},
			},
			ResourceLoad: slurm.ResourceTotals{
				RunningCPU:   640,
				PendingCPU:   96,
				RunningMemMB: 1880000,
				PendingMemMB: 220000,
				RunningGPU:   38,
				PendingGPU:   8,
			},
		},
			Users: []slurm.UserSummary{
				{User: "alice", Running: 17, Pending: 3, PendingCPUJobs: 1, PendingGPUJobs: 2, PendingCPU: 96, PendingMemMB: 220000, PendingGPU: 8},
				{User: "bob", Running: 9, Pending: 1, PendingCPUJobs: 1, PendingGPUJobs: 0, PendingCPU: 32, PendingMemMB: 64000, PendingGPU: 0},
				{User: "carol", Running: 6, Pending: 1, PendingCPUJobs: 0, PendingGPUJobs: 1, PendingCPU: 16, PendingMemMB: 32000, PendingGPU: 1},
			},
		}
}

func assertViewportBounds(t *testing.T, s string, width int, height int) {
	t.Helper()
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		t.Fatalf("render exceeded height: got %d lines, max %d", len(lines), height)
	}
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			t.Fatalf("line %d exceeded width: got %d, max %d", i+1, lipgloss.Width(line), width)
		}
	}
}
