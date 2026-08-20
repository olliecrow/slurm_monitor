package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/olliecrow/slurm_monitor/internal/monitor"
	"github.com/olliecrow/slurm_monitor/internal/slurm"
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
	if !strings.Contains(h1, "refresh: <1s ago") {
		t.Fatalf("expected header to include refresh age wording")
	}
	if strings.Contains(h1, "utc 2026-02-25 10:00:00") {
		t.Fatalf("did not expect utc timestamp in header")
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
	if len(lines) != 1 {
		t.Fatalf("expected single-line header without errors, got %d lines", len(lines))
	}
	if !strings.Contains(lines[0], "connected") {
		t.Fatalf("expected status to remain visible in narrow header, got: %q", lines[0])
	}
	if lipgloss.Width(lines[0]) > m.width {
		t.Fatalf("expected narrow header line to fit width %d, got %d", m.width, lipgloss.Width(lines[0]))
	}
}

func TestHeaderShowsLoadingBeforeFirstSnapshot(t *testing.T) {
	m := NewModel(Options{
		Source:  "ssh:test",
		Updates: make(chan monitor.Update),
		NoColor: true,
	})
	m.width = 80
	m.height = 20

	h := m.renderHeader(time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC))
	if !strings.Contains(h, "loading") {
		t.Fatalf("expected startup header status to show loading, got: %q", h)
	}
	if strings.Contains(h, "reconnecting") {
		t.Fatalf("did not expect reconnecting during clean startup, got: %q", h)
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
	if strings.Contains(lines[1], "utc 2026-02-25 10:00:00") {
		t.Fatalf("did not expect utc timestamp on error line, got: %q", lines[1])
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
	if strings.Contains(lines[1], "utc 2026-02-25 10:00:00") {
		t.Fatalf("did not expect utc timestamp on long-error line, got: %q", lines[1])
	}
	if lipgloss.Width(lines[1]) > m.width {
		t.Fatalf("expected long-error line to fit width %d, got %d", m.width, lipgloss.Width(lines[1]))
	}
}

func TestSchedulerSummaryRendersJobsAndResourceDemand(t *testing.T) {
	m := seededModel()
	out := m.renderInsightsPanelWithBudget(24, true, 120)
	if strings.Contains(out, "█") || strings.Contains(out, "░") {
		t.Fatalf("expected scheduler summary without bar glyphs, got: %q", out)
	}
	for _, want := range []string{"running cpu jobs", "running gpu jobs", "pending cpu jobs", "pending gpu jobs", "other", "total", "running resources", "pending demand", "cpu=96", "gpu=8"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected scheduler summary label %q in output, got: %q", want, out)
		}
	}
}

func TestInsightsPanelShowsAllDetailViews(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.Partitions = []slurm.PartitionSummary{
		{Name: "gpu", Queue: slurm.QueueSummary{PendingGPUJobs: 2}},
		{Name: "cpu", Queue: slurm.QueueSummary{RunningCPUJobs: 3}},
	}
	m.snapshot.Jobs = []slurm.JobSummary{
		{JobID: "3001", User: "alice", Partition: "gpu", State: "PENDING", Tasks: 12, CPU: 48, GPU: 12},
		{JobID: "3002", User: "bob", Partition: "cpu", State: "RUNNING", Tasks: 1, CPU: 8},
	}
	m.snapshot.PendingReasons = []slurm.PendingReasonSummary{
		{Reason: "Resources", Jobs: 12, CPU: 48, GPU: 12},
		{Reason: "Priority", Jobs: 2, CPU: 8},
	}

	out := m.renderInsightsPanelWithBudget(28, true, 100)
	for _, want := range []string{"partition view", "gpu", "user view", "alice", "pending reasons", "Resources", "job view", "3001", "tasks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in combined insight panel, got: %q", want, out)
		}
	}
}

func TestInsightsPanelSharesTightBudgetAcrossAllDetailViews(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.Partitions = []slurm.PartitionSummary{
		{Name: "gpu", Queue: slurm.QueueSummary{PendingGPUJobs: 3}},
		{Name: "cpu", Queue: slurm.QueueSummary{PendingCPUJobs: 2}},
		{Name: "short", Queue: slurm.QueueSummary{RunningGPUJobs: 1}},
	}
	m.snapshot.Jobs = []slurm.JobSummary{
		{JobID: "3001", User: "alice", Partition: "gpu", State: "PENDING", Tasks: 2, GPU: 2},
		{JobID: "3002", User: "bob", Partition: "cpu", State: "PENDING", Tasks: 1, CPU: 8},
		{JobID: "3003", User: "carol", Partition: "short", State: "RUNNING", Tasks: 1, GPU: 1},
	}
	m.snapshot.PendingReasons = []slurm.PendingReasonSummary{
		{Reason: "Resources", Jobs: 3, GPU: 3},
		{Reason: "Priority", Jobs: 2, CPU: 16},
		{Reason: "Dependency", Jobs: 1, CPU: 4},
	}

	out := m.renderInsightsPanelWithBudget(17, true, 90)
	for _, want := range []string{"partition view", "gpu", "user view", "alice", "pending reasons", "Resources", "job view", "3001"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in tight combined insight panel, got: %q", want, out)
		}
	}
	for _, want := range []string{
		"partition view (top 1/3, +2 hidden)",
		"user view (top 1/3, +2 hidden)",
		"pending reasons (top 1/3, +2 hidden)",
		"job view (top 1/3, +2 hidden)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected deterministic clipping metadata %q, got: %q", want, out)
		}
	}
	if got := len(strings.Split(out, "\n")); got > 17 {
		t.Fatalf("expected at most 17 lines, got %d", got)
	}
}

func TestJobCompactStateLabelsAreUnambiguous(t *testing.T) {
	tests := map[string]string{
		"PENDING":     "PEND",
		"RUNNING":     "RUN",
		"COMPLETING":  "COMP",
		"CONFIGURING": "CONF",
	}
	for state, want := range tests {
		if got := shortJobState(state); got != want {
			t.Fatalf("shortJobState(%q)=%q want=%q", state, got, want)
		}
	}
}

func TestQueueSummaryCountsStayAligned(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.Queue = slurm.QueueSummary{
		RunningCPUJobs: 5,
		RunningGPUJobs: 40,
		PendingCPUJobs: 1,
		PendingGPUJobs: 55,
		Other:          0,
	}

	lines := []string{
		m.queueStatusLine("running cpu jobs", m.snapshot.Queue.RunningCPUJobs),
		m.queueStatusLine("running gpu jobs", m.snapshot.Queue.RunningGPUJobs),
		m.queueStatusLine("pending cpu jobs", m.snapshot.Queue.PendingCPUJobs),
		m.queueStatusLine("pending gpu jobs", m.snapshot.Queue.PendingGPUJobs),
		m.queueStatusLine("other", m.snapshot.Queue.Other),
		m.queueStatusLine("total", m.snapshot.Queue.TotalJobs()),
	}

	start := lastDigitIndex(lines[0])
	if start < 0 {
		t.Fatalf("expected first count in %q", lines[0])
	}
	for _, line := range lines[1:] {
		if got := lastDigitIndex(line); got != start {
			t.Fatalf("count column mismatch: first=%d got=%d line=%q all=%q", start, got, line, strings.Join(lines, "\n"))
		}
	}
}

func TestInsightsPanelBudgetKeepsEachDetailTitle(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	out := m.renderInsightsPanelWithBudget(13, true, 90)
	for _, want := range []string{"partition view", "user view", "pending reasons", "job view"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q title when scheduler budget is tight, got: %q", want, out)
		}
	}
}

func TestUserLinesBudgetTwoRowsShowsOneUser(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(2, true, 80)
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "user view (top 1/3, +2 hidden)") {
		t.Fatalf("expected one visible user in tight two-row budget, got: %q", out)
	}
	if !strings.Contains(out, "alice") {
		t.Fatalf("expected top user row in tight two-row budget, got: %q", out)
	}
	if strings.Contains(out, "640") || strings.Contains(out, "38") {
		t.Fatalf("expected no held cpu/gpu values in tight two-row budget, got: %q", out)
	}
	if !strings.Contains(out, "5") || !strings.Contains(out, "12") {
		t.Fatalf("expected running job counts in tight two-row budget, got: %q", out)
	}
}

func TestWideUserColumnsStayAligned(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(5, true, 120)
	if len(lines) < 3 {
		t.Fatalf("expected header plus rows, got: %q", strings.Join(lines, "\n"))
	}

	wantHeader := []string{"user", "runCPUj", "runGPUj", "pendCPUj", "pendGPUj", "runCPU", "pendCPU", "runGPU", "pendGPU"}
	if got := strings.Fields(lines[1]); !equalStrings(got, wantHeader) {
		t.Fatalf("unexpected expanded user header: got %v want %v", got, wantHeader)
	}
	for _, row := range lines[2:4] {
		if got := len(strings.Fields(row)); got != len(wantHeader) {
			t.Fatalf("expected %d aligned user columns, got %d in %q", len(wantHeader), got, row)
		}
	}
}

func TestExpandedPartitionColumnsShowJobCountsAndResources(t *testing.T) {
	partition := slurm.PartitionSummary{
		Name: "accelerated",
		Queue: slurm.QueueSummary{
			RunningCPUJobs: 1,
			RunningGPUJobs: 2,
			PendingCPUJobs: 3,
			PendingGPUJobs: 4,
			ResourceLoad: slurm.ResourceTotals{
				RunningCPU: 16,
				PendingCPU: 32,
				RunningGPU: 2,
				PendingGPU: 4,
			},
		},
	}
	wantHeader := []string{"partition", "runCPUj", "runGPUj", "pendCPUj", "pendGPUj", "runCPU", "pendCPU", "runGPU", "pendGPU"}
	if got := strings.Fields(widePartitionHeaderLine()); !equalStrings(got, wantHeader) {
		t.Fatalf("unexpected expanded partition header: got %v want %v", got, wantHeader)
	}
	if got := len(strings.Fields(widePartitionRowLine(partition))); got != len(wantHeader) {
		t.Fatalf("expected %d expanded partition columns, got %d", len(wantHeader), got)
	}
}

func TestWideJobRowShowsPendingReason(t *testing.T) {
	row := wideJobRowLine(slurm.JobSummary{
		JobID:     "123",
		User:      "alice",
		Partition: "gpu",
		State:     "PENDING",
		Reason:    "Resources",
		Tasks:     2,
		CPU:       8,
		GPU:       2,
	})
	if !strings.Contains(wideJobHeaderLine(), "reason") || !strings.Contains(row, "Resources") {
		t.Fatalf("expected wide job view to show pending reason, got header=%q row=%q", wideJobHeaderLine(), row)
	}
}

func TestUserColumnsDoNotShowHeldTotals(t *testing.T) {
	wideHeader := wideUserHeaderLine()
	compactHeader := compactUserHeaderLine()
	if strings.Contains(wideHeader, "heldCPU") || strings.Contains(wideHeader, "heldGPU") {
		t.Fatalf("wide user header should not show held totals: %q", wideHeader)
	}
	if strings.Contains(compactHeader, "hCPU") || strings.Contains(compactHeader, "hGPU") {
		t.Fatalf("compact user header should not show held totals: %q", compactHeader)
	}
}

func TestWideUserColumnsDistinguishJobsFromResources(t *testing.T) {
	header := wideUserHeaderLine()
	for _, want := range []string{"runCPUj", "runGPUj", "pendCPUj", "pendGPUj", "runCPU", "pendCPU", "runGPU", "pendGPU"} {
		if !strings.Contains(header, want) {
			t.Fatalf("expected distinct job and resource column %q, got %q", want, header)
		}
	}
}

func TestCompactUserColumnsStayAligned(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(5, false, 80)
	if len(lines) < 3 {
		t.Fatalf("expected compact header plus rows, got: %q", strings.Join(lines, "\n"))
	}

	header := lines[1]
	row := lines[2]

	assertColumnHasValue(t, header, row, "rCJ", "rGJ")
	assertColumnHasValue(t, header, row, "rGJ", "pCJ")
	assertColumnHasValue(t, header, row, "pCJ", "pGJ")
	assertColumnHasValue(t, header, row, "pGJ", "")
}

func TestUserLinesBudgetOneRowUsesHiddenOnlyLabel(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(1, true, 80)
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "user view (+3 hidden)") {
		t.Fatalf("expected hidden-only user label for one-row budget, got: %q", out)
	}
	if strings.Contains(out, "top 0/") {
		t.Fatalf("expected no top 0/N label, got: %q", out)
	}
}

func TestCompactViewKeepsCompactJobColumnsAndResourceSummary(t *testing.T) {
	m := seededModel()
	m.compact = true
	m.width = 90
	m.height = 36

	out := m.View()
	if strings.Contains(out, "heldCPU") || strings.Contains(out, "heldGPU") {
		t.Fatalf("expected compact view to hide held-resource columns, got: %q", out)
	}
	for _, want := range []string{"rCJ", "rGJ", "pCJ", "pGJ", "running resources", "pending demand"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected compact view to include %q, got: %q", want, out)
		}
	}
}

func TestUserPanelUsesAvailableHeightBeforeHidingUsers(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 96
	users := make([]slurm.UserSummary, 0, 15)
	for i := 0; i < 15; i++ {
		users = append(users, slurm.UserSummary{
			User:           fmt.Sprintf("user%02d", i),
			RunningCPUJobs: 1,
			RunningGPUJobs: 0,
			PendingCPUJobs: 30 - i,
			PendingGPUJobs: 0,
		})
	}
	m.snapshot.Users = users

	out := strings.Join(m.renderUserLinesWithBudget(17, true, 88), "\n")
	if strings.Contains(out, "hidden)") {
		t.Fatalf("did not expect hidden-user indicator when compact panel has enough height, got: %q", out)
	}
	if !strings.Contains(out, "user14") {
		t.Fatalf("expected lower-priority users to remain visible when height allows, got: %q", out)
	}
}

func TestUserViewShowsHiddenCountWhenCapped(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	lines := m.renderUserLinesWithBudget(3, true, 120)
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
			User:           fmt.Sprintf("user-%02d", i),
			RunningCPUJobs: 1,
			PendingCPUJobs: 1,
		})
	}
	m.width = 80
	m.height = 20

	out := m.View()
	if !strings.Contains(out, "user view (") || !strings.Contains(out, "hidden)") {
		t.Fatalf("expected user view hidden-count indicator in tight layout, got: %q", out)
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

func TestViewPinsExitHintToBottomRow(t *testing.T) {
	m := seededModel()
	m.width = 100
	m.height = 26
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Fatalf("expected %d lines, got %d", m.height, len(lines))
	}
	if !strings.Contains(lines[len(lines)-1], "Ctrl+C to exit") {
		t.Fatalf("expected exit hint on bottom row, got: %q", lines[len(lines)-1])
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
		Queue: slurm.QueueSummary{
			Other:          1,
			RunningCPUJobs: 14,
			RunningGPUJobs: 28,
			PendingCPUJobs: 2,
			PendingGPUJobs: 3,
			ResourceLoad: slurm.ResourceTotals{
				RunningCPU: 640,
				PendingCPU: 96,
				RunningGPU: 38,
				PendingGPU: 8,
			},
		},
		Partitions: []slurm.PartitionSummary{
			{Name: "gpu", Queue: slurm.QueueSummary{RunningGPUJobs: 28, PendingGPUJobs: 3, ResourceLoad: slurm.ResourceTotals{RunningCPU: 420, PendingCPU: 64, RunningGPU: 38, PendingGPU: 8}}},
			{Name: "cpu", Queue: slurm.QueueSummary{RunningCPUJobs: 14, PendingCPUJobs: 2, ResourceLoad: slurm.ResourceTotals{RunningCPU: 220, PendingCPU: 32}}},
		},
		Users: []slurm.UserSummary{
			{User: "alice", RunningCPU: 640, RunningGPU: 38, RunningCPUJobs: 5, RunningGPUJobs: 12, PendingCPUJobs: 1, PendingGPUJobs: 2, PendingCPU: 96, PendingMemMB: 220000, PendingGPU: 8},
			{User: "bob", RunningCPU: 220, RunningCPUJobs: 9, PendingCPUJobs: 1, PendingCPU: 32, PendingMemMB: 64000},
			{User: "carol", RunningCPU: 180, RunningGPU: 6, RunningCPUJobs: 2, RunningGPUJobs: 4, PendingGPUJobs: 1, PendingCPU: 16, PendingMemMB: 32000, PendingGPU: 1},
		},
		PendingReasons: []slurm.PendingReasonSummary{
			{Reason: "Resources", Jobs: 3, CPU: 64, GPU: 8},
			{Reason: "Priority", Jobs: 2, CPU: 32},
		},
		Jobs: []slurm.JobSummary{
			{JobID: "3001", User: "alice", Partition: "gpu", State: "PENDING", Reason: "Resources", Tasks: 3, CPU: 64, GPU: 8},
			{JobID: "3002", User: "bob", Partition: "cpu", State: "RUNNING", Tasks: 1, CPU: 16},
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

func assertColumnHasValue(t *testing.T, header string, row string, label string, nextLabel string) {
	t.Helper()

	start := strings.Index(header, label)
	if start < 0 {
		t.Fatalf("missing header label %q in %q", label, header)
	}

	end := len(row)
	if nextLabel != "" {
		next := strings.Index(header, nextLabel)
		if next < 0 {
			t.Fatalf("missing next header label %q in %q", nextLabel, header)
		}
		end = next
	}
	if start >= len(row) {
		t.Fatalf("row too short for label %q: %q", label, row)
	}
	if end > len(row) {
		end = len(row)
	}
	segment := strings.TrimSpace(row[start:end])
	if segment == "" {
		t.Fatalf("expected value under %q column, row=%q", label, row)
	}
}

func lastDigitIndex(s string) int {
	out := -1
	for i, r := range s {
		if r >= '0' && r <= '9' {
			out = i
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
