package tui

import (
	"fmt"
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
	if !strings.Contains(h1, "refresh 2s") {
		t.Fatalf("expected header to include refresh cadence")
	}
	if !strings.Contains(h1, "utc 2026-02-25 10:00:00") {
		t.Fatalf("expected header to include utc timestamp")
	}
	if h1 == h2 {
		t.Fatalf("expected header to change between ticks")
	}
}

func TestHeaderKeepsStatusVisibleAtNarrowWidth(t *testing.T) {
	m := seededModel()
	m.width = 56
	m.source = strings.Repeat("cluster-source-", 6)

	h := m.renderHeader(m.now)
	lines := strings.Split(h, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two-line header, got %d lines", len(lines))
	}
	if !strings.Contains(lines[0], "connected") {
		t.Fatalf("expected status to remain visible in narrow header, got: %q", lines[0])
	}
	if lipgloss.Width(lines[0]) > m.width {
		t.Fatalf("expected narrow header line to fit width %d, got %d", m.width, lipgloss.Width(lines[0]))
	}
}

func TestHeaderDoesNotIncludeNodeAlert(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.Nodes[0].State = "MIXED+DRAIN"
	m.snapshot.Nodes[1].State = "IDLE+DOWN"

	h := m.renderHeader(m.now)
	if strings.Contains(h, "node alert:") {
		t.Fatalf("expected header to omit node alert, got: %q", h)
	}
}

func TestHeaderErrorLineShowsErrorAndRespectsWidth(t *testing.T) {
	m := seededModel()
	m.width = 120
	m.lastError = "transport timeout"

	h := m.renderHeader(m.now)
	lines := strings.Split(h, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two-line header, got %d lines", len(lines))
	}
	if !strings.Contains(lines[1], "error:") {
		t.Fatalf("expected second line to include error label, got: %q", lines[1])
	}
	if !strings.Contains(lines[1], "utc 2026-02-25 10:00:00") {
		t.Fatalf("expected second line to retain utc timestamp with error, got: %q", lines[1])
	}
	if lipgloss.Width(lines[1]) > m.width {
		t.Fatalf("expected error line to fit width %d, got %d", m.width, lipgloss.Width(lines[1]))
	}
}

func TestHeaderErrorLineLongMessageStillFitsWidth(t *testing.T) {
	m := seededModel()
	m.width = 80
	m.lastError = "transport timeout while fetching snapshot from remote cluster host"

	h := m.renderHeader(m.now)
	lines := strings.Split(h, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two-line header, got %d lines", len(lines))
	}
	if !strings.Contains(lines[1], "error:") {
		t.Fatalf("expected second line to include error label, got: %q", lines[1])
	}
	if !strings.Contains(lines[1], "utc 2026-02-25 10:00:00") {
		t.Fatalf("expected long-error line to retain utc timestamp, got: %q", lines[1])
	}
	if lipgloss.Width(lines[1]) > m.width {
		t.Fatalf("expected long-error line to fit width %d, got %d", m.width, lipgloss.Width(lines[1]))
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

func TestQueuePanelBudgetKeepsUserTitleWhenOnlyOneLineRemains(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	out := m.renderQueuePanelWithBudget(6, 16, true, true, 70)
	if !strings.Contains(out, "user view") {
		t.Fatalf("expected user section title even when queue budget is tight, got: %q", out)
	}
}

func TestUserLinesBudgetTwoRowsShowsOneUser(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(10, 2, true, 80)
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "user view (top 1/3, +2 hidden)") {
		t.Fatalf("expected one visible user in tight two-row budget, got: %q", out)
	}
	if !strings.Contains(out, "alice") {
		t.Fatalf("expected top user row in tight two-row budget, got: %q", out)
	}
}

func TestUserLinesBudgetOneRowUsesHiddenOnlyLabel(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(10, 1, true, 80)
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "user view (+3 hidden)") {
		t.Fatalf("expected hidden-only user label for one-row budget, got: %q", out)
	}
	if strings.Contains(out, "top 0/") {
		t.Fatalf("expected no top 0/N label, got: %q", out)
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

func TestNodeTableShowsNodeAlert(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.Nodes[0].State = "MIXED+DRAIN"

	out := m.renderNodeTable(10)
	if !strings.Contains(out, "node alert: drain=1") {
		t.Fatalf("expected node table to include drain alert, got: %q", out)
	}
}

func TestNodeTableBudgetKeepsAlertAndTotalInTightSpace(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.Nodes[0].State = "MIXED+DRAIN"

	out := m.renderNodeTableWithBudget(4, 16, true, 60)
	if !strings.Contains(out, "node summary") {
		t.Fatalf("expected node summary title in tight budget, got: %q", out)
	}
	if !strings.Contains(out, "node alert: drain=1") {
		t.Fatalf("expected node alert in tight budget, got: %q", out)
	}
	if !strings.Contains(out, "TOTAL") {
		t.Fatalf("expected TOTAL row in tight budget, got: %q", out)
	}
}

func TestNodeTableShowsHiddenCountAndTotalWhenCapped(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	out := m.renderNodeTable(1)

	if !strings.Contains(out, "node summary (top 1/3, +2 hidden)") {
		t.Fatalf("expected capped node summary label, got: %q", out)
	}
	if !strings.Contains(out, "TOTAL") {
		t.Fatalf("expected TOTAL row to remain visible when node rows are capped, got: %q", out)
	}
}

func TestWideNodeTableShowsUntruncatedDrainState(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 180
	m.snapshot.Nodes[0].State = "MIXED+DRAIN"

	out := m.renderNodeTable(10)
	if !strings.Contains(out, "MIXED+DRAIN") {
		t.Fatalf("expected wide table to show full drain state, got: %q", out)
	}
}

func TestUserViewShowsHiddenCountWhenCapped(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	lines := m.renderUserLines(1, true)
	out := strings.Join(lines, "\n")

	if !strings.Contains(out, "user view (top 1/3, +2 hidden)") {
		t.Fatalf("expected capped user view label, got: %q", out)
	}
	if strings.Count(out, "alice") != 1 {
		t.Fatalf("expected only top user to render when capped, got: %q", out)
	}
}

func TestViewShowsHiddenUserIndicatorInTightLayout(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	for i := 0; i < 20; i++ {
		m.snapshot.Users = append(m.snapshot.Users, slurm.UserSummary{
			User:    fmt.Sprintf("user-%02d", i),
			Running: 1,
			Pending: 1,
		})
	}
	m.width = 80
	m.height = 20

	out := m.View()
	if !strings.Contains(out, "user view (") || !strings.Contains(out, "hidden)") {
		t.Fatalf("expected user view hidden-count indicator in tight layout, got: %q", out)
	}
}

func TestViewKeepsNodeAlertAndTotalInTightLayout(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.Nodes[0].State = "MIXED+DRAIN"
	m.width = 80
	m.height = 20

	out := m.View()
	if !strings.Contains(out, "node summary") {
		t.Fatalf("expected node summary title in tight layout, got: %q", out)
	}
	if !strings.Contains(out, "node alert: drain=1") {
		t.Fatalf("expected node alert in tight layout, got: %q", out)
	}
	if !strings.Contains(out, "TOTAL") {
		t.Fatalf("expected TOTAL row in tight layout, got: %q", out)
	}
}

func TestViewUsesStabilizedFrameWidth(t *testing.T) {
	m := seededModel()
	m.width = 90
	m.height = 24
	out := m.View()
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > 89 {
			t.Fatalf("expected line %d width <= 89 after right-gutter stabilization, got %d", i+1, lipgloss.Width(line))
		}
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
	if strings.Contains(out, viewportClipText) {
		t.Fatalf("did not expect clip marker when content fits viewport, got: %q", out)
	}
}

func TestClipToViewportMarksTerminalHeightClipping(t *testing.T) {
	out := clipToViewport("a\nb\nc\nd\ne", 48, 3)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[2], viewportClipText) {
		t.Fatalf("expected last visible row to contain clip marker, got: %q", lines[2])
	}
	for i, line := range lines {
		if lipgloss.Width(line) != 48 {
			t.Fatalf("expected line %d width 48, got %d", i+1, lipgloss.Width(line))
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
