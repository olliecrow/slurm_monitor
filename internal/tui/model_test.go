package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

func TestWindowResizeSwitchesResponsiveLayout(t *testing.T) {
	model := seededModel()

	resized, _ := model.Update(tea.WindowSizeMsg{Width: 72, Height: 20})
	compact := resized.(Model)
	compactView := compact.View()
	if !strings.Contains(compactView, ": running ") || strings.Contains(compactView, "resources in use") {
		t.Fatalf("expected compact layout after narrowing the same model, got %q", compactView)
	}
	assertViewportBounds(t, compactView, 71, 20)

	resized, _ = compact.Update(tea.WindowSizeMsg{Width: 115, Height: 30})
	expanded := resized.(Model)
	expandedView := expanded.View()
	if !strings.Contains(expandedView, "resources in use") {
		t.Fatalf("expected expanded layout after widening the same model, got %q", expandedView)
	}
	assertViewportBounds(t, expandedView, 114, 30)
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
	if !strings.Contains(h1, "10:00:00") {
		t.Fatalf("expected header to include first clock value")
	}
	if !strings.Contains(h2, "10:00:01") {
		t.Fatalf("expected header to include second clock value")
	}
	if !strings.Contains(h1, "updated <1s ago") {
		t.Fatalf("expected header to include update age wording")
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

func TestHeaderDropsWholeFieldsInsteadOfTruncatingValues(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 72
	m.source = "ssh:very-long-cluster-alias"

	header := m.renderHeader(m.now)
	for _, want := range []string{"SLURM MONITOR", "10:00:00", "updated <1s ago", "connected"} {
		if !strings.Contains(header, want) {
			t.Fatalf("expected compact header to preserve %q, got %q", want, header)
		}
	}
	if strings.Contains(header, "source:") || strings.Contains(header, "…") {
		t.Fatalf("expected compact header to omit source instead of truncating a value, got %q", header)
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
	for _, want := range []string{
		"Queue · 48 jobs · 42 running · 5 pending · 1 other",
		schedulerActivityLine("Running", 14, 28, 640, 38),
		schedulerActivityLine("Pending", 2, 3, 96, 8),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected scheduler summary row %q in output, got: %q", want, out)
		}
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 4 || lines[2] != "" || !strings.Contains(lines[3], "Partitions") {
		t.Fatalf("expected a blank line between the compact queue summary and partitions, got: %q", out)
	}
}

func TestSchedulerSummaryUsesOneActivityLineOnlyWhenItFits(t *testing.T) {
	m := seededModel()
	wide := m.schedulerSummaryLines(m.snapshot.Queue, 120)
	if len(wide) != 2 || !strings.Contains(wide[1], "  │  Pending") {
		t.Fatalf("expected wide queue activity on one line, got: %q", wide)
	}
	compact := m.schedulerSummaryLines(m.snapshot.Queue, 61)
	if len(compact) != 3 || !strings.HasPrefix(compact[1], "Running") || !strings.HasPrefix(compact[2], "Pending") {
		t.Fatalf("expected compact queue activity on two lines, got: %q", compact)
	}
}

func TestInsightsPanelShowsAllAggregateViews(t *testing.T) {
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
		{Reason: "Resources", Tasks: 12, CPU: 48, GPU: 12},
		{Reason: "Priority", Tasks: 2, CPU: 8},
	}

	out := m.renderInsightsPanelWithBudget(28, true, 100)
	for _, want := range []string{"Partitions", "gpu", "Users", "alice", "Why jobs are pending", "Waiting for resources", "tasks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in combined insight panel, got: %q", want, out)
		}
	}
	for _, unwanted := range []string{"Jobs", "3001", "3002"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("did not expect individual job content %q in the TUI, got: %q", unwanted, out)
		}
	}
	if got := strings.Count(out, "\n\n"); got != 3 {
		t.Fatalf("expected three blank section separators, got %d in %q", got, out)
	}
}

func TestInsightsPanelSharesTightBudgetAcrossAggregateViews(t *testing.T) {
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
		{Reason: "Resources", Tasks: 3, GPU: 3},
		{Reason: "Priority", Tasks: 2, CPU: 16},
		{Reason: "Dependency", Tasks: 1, CPU: 4},
	}

	out := m.renderInsightsPanelWithBudget(17, true, 90)
	for _, want := range []string{"Partitions", "gpu", "Users", "alice", "Why jobs are pending", "Waiting for resources"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in tight combined insight panel, got: %q", want, out)
		}
	}
	for _, want := range []string{
		"Partitions · 2 shown · 1 hidden",
		"Users · 2 shown · 1 hidden",
		"Why jobs are pending · 1 shown · 2 hidden",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected deterministic clipping metadata %q, got: %q", want, out)
		}
	}
	if got := strings.Count(out, "\n\n"); got != 3 {
		t.Fatalf("expected spacing between all sections, got %d separators in %q", got, out)
	}
}

func TestSchedulerActivityLinesExplainJobsAndResources(t *testing.T) {
	if got, want := schedulerActivityLine("Running", 5, 40, 1234, 55), "Running · 5 CPU-only + 40 GPU jobs · 1234 CPUs + 55 GPUs"; got != want {
		t.Fatalf("running row=%q want=%q", got, want)
	}
	if got, want := schedulerActivityLine("Pending", 1, 55, 98765, 100), "Pending · 1 CPU-only + 55 GPU jobs · 98765 CPUs + 100 GPUs"; got != want {
		t.Fatalf("pending row=%q want=%q", got, want)
	}
}

func TestPendingReasonLabelsUsePlainLanguageAndPreserveUnknownReasons(t *testing.T) {
	tests := map[string]string{
		"Dependency":               "Waiting for dependency",
		"DependencyNeverSatisfied": "Dependency cannot be satisfied",
		"JobArrayTaskLimit":        "Job array task limit reached",
		"Priority":                 "Waiting behind higher-priority jobs",
		"ReqNodeNotAvail":          "Required node unavailable",
		"ReqNodeNotAvail, Reserved for maintenance":  "Required node unavailable: Reserved for maintenance",
		"ReqNodeNotAvail,":                           "Required node unavailable",
		"Resources":                                  "Waiting for resources",
		"Nodes required for job are DOWN or DRAINED": "Nodes required for job are DOWN or DRAINED",
	}
	for input, want := range tests {
		if got := pendingReasonLabel(input); got != want {
			t.Errorf("pendingReasonLabel(%q)=%q want %q", input, got, want)
		}
	}
}

func TestCountNounUsesSingularOnlyForOne(t *testing.T) {
	for _, test := range []struct {
		count int
		want  string
	}{
		{count: 0, want: "0 tasks"},
		{count: 1, want: "1 task"},
		{count: 2, want: "2 tasks"},
	} {
		if got := countNoun(test.count, "task"); got != test.want {
			t.Errorf("countNoun(%d)=%q want %q", test.count, got, test.want)
		}
	}
}

func TestPendingReasonTruncationKeepsWholeWords(t *testing.T) {
	reason := "Nodes required for job are DOWN, DRAINED or reserved"
	if got, want := truncatePendingReason(reason, 46), "Nodes required for job are DOWN, DRAINED…"; got != want {
		t.Fatalf("truncatePendingReason()=%q want %q", got, want)
	}
	if got := truncatePendingReason("SingleUnbrokenReasonName", 12); got != "SingleUnbro…" {
		t.Fatalf("expected unbroken reason to use safe rune truncation, got %q", got)
	}
}

func TestInsightsPanelBudgetKeepsOneRowAndSpacingForEachAggregateView(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	out := m.renderInsightsPanelWithBudget(13, true, 90)
	for _, want := range []string{"Partitions", "gpu", "Users", "alice", "Why jobs are pending", "Waiting for resources"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in compact insight budget, got: %q", want, out)
		}
	}
	if !strings.Contains(out, "3 tasks, 64 CPUs, 8 GPUs") {
		t.Fatalf("expected self-describing pending-reason detail, got %q", out)
	}
	if got := strings.Count(out, "\n\n"); got != 3 {
		t.Fatalf("expected spacing between all sections, got %d separators in %q", got, out)
	}
}

func TestInsightsPanelUsesSpacingForDataOnVeryTightTerminals(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	out := m.renderInsightsPanelWithBudget(9, true, 90)
	for _, want := range []string{"gpu", "alice", "Waiting for resources"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected tight layout to retain data row %q, got %q", want, out)
		}
	}
	if strings.Contains(out, "\n\n") {
		t.Fatalf("did not expect blank separators to replace data in a very tight layout, got %q", out)
	}
}

func TestUserLinesBudgetTwoRowsShowsOneUser(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(2, true, 80)
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "Users · job counts · 1 shown · 2 hidden") {
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

func TestWideUserColumnsUsePlainGroupedLabels(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(5, true, 120)
	if len(lines) < 3 {
		t.Fatalf("expected header plus rows, got: %q", strings.Join(lines, "\n"))
	}

	for _, want := range []string{"running jobs", "pending jobs", "resources in use", "resources requested"} {
		if !strings.Contains(lines[1], want) {
			t.Fatalf("expected expanded user header %q, got %q", want, lines[1])
		}
	}
	for _, row := range lines[2:4] {
		for _, want := range []string{"CPU-only", "GPU", "CPUs", "GPUs"} {
			if !strings.Contains(row, want) {
				t.Fatalf("expected expanded user row to include %q, got %q", want, row)
			}
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
	header := groupedSummaryHeaderLine("partition", 140)
	row := groupedSummaryRowLine(partition.Name, partition.Queue, 140)
	for _, want := range []string{"running jobs", "pending jobs", "resources in use", "resources requested"} {
		if !strings.Contains(header, want) {
			t.Fatalf("expected expanded partition header %q, got %q", want, header)
		}
	}
	for _, want := range []string{"accelerated", "CPU-only 1, GPU 2", "CPU-only 3, GPU 4", "CPUs 16, GPUs 2", "CPUs 32, GPUs 4"} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected expanded partition row %q, got %q", want, row)
		}
	}
}

func TestWideAggregateTablesUseNaturalWidth(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	const availableWidth = 174
	wantWidth := groupedSummaryMinContentWidth(m.snapshot)

	partitionLines := m.renderPartitionLinesWithBudget(5, true, availableWidth)
	userLines := m.renderUserLinesWithBudget(5, true, availableWidth)
	for name, lines := range map[string][]string{"partition": partitionLines, "user": userLines} {
		if got := lipgloss.Width(lines[1]); got != wantWidth {
			t.Fatalf("expected %s table width %d, got %d in %q", name, wantWidth, got, lines[1])
		}
		if got := lipgloss.Width(lines[1]); got >= availableWidth {
			t.Fatalf("expected %s table not to stretch across %d columns, got %d", name, availableWidth, got)
		}
	}
}

func TestWidePendingReasonTableUsesNaturalWidth(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.PendingReasons = []slurm.PendingReasonSummary{
		{Reason: "Dependency", Tasks: 4, CPU: 16},
		{Reason: "DependencyNeverSatisfied", Tasks: 2, GPU: 2},
	}
	const availableWidth = 174
	wantWidth := pendingReasonTableWidth(m.snapshot.PendingReasons)

	lines := m.renderPendingReasonLinesWithBudget(4, true, availableWidth)
	if got := lipgloss.Width(lines[1]); got != wantWidth {
		t.Fatalf("expected pending-reason table width %d, got %d in %q", wantWidth, got, lines[1])
	}
	if got := lipgloss.Width(lines[1]); got >= availableWidth {
		t.Fatalf("expected pending-reason table not to stretch across %d columns, got %d", availableWidth, got)
	}
}

func TestUserColumnsDoNotShowHeldTotals(t *testing.T) {
	wideHeader := groupedSummaryHeaderLine("user", 120)
	compactRow := compactGroupedSummaryRowLine("alice", userQueueSummary(sampleSnapshot().Users[0]))
	if strings.Contains(wideHeader, "heldCPU") || strings.Contains(wideHeader, "heldGPU") {
		t.Fatalf("wide user header should not show held totals: %q", wideHeader)
	}
	if strings.Contains(compactRow, "640") || strings.Contains(compactRow, "38") {
		t.Fatalf("compact user row should not show resource totals: %q", compactRow)
	}
}

func TestWideUserColumnsDistinguishJobsFromResources(t *testing.T) {
	header := groupedSummaryHeaderLine("user", 120)
	row := groupedSummaryRowLine("alice", userQueueSummary(sampleSnapshot().Users[0]), 120)
	for _, want := range []string{"running jobs", "pending jobs", "resources in use", "resources requested"} {
		if !strings.Contains(header, want) {
			t.Fatalf("expected distinct job and resource group %q, got %q", want, header)
		}
	}
	for _, want := range []string{"CPU-only", "GPU", "CPUs", "GPUs"} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected explicit job and resource unit %q, got %q", want, row)
		}
	}
}

func TestCompactUserRowsAreSelfDescribingAndHeaderless(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(5, false, 80)
	out := strings.Join(lines, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected title plus three user rows, got: %q", out)
	}
	if strings.Contains(out, "CPU-only run") {
		t.Fatalf("did not expect a compact table header, got: %q", out)
	}
	for _, want := range []string{"Users · job counts · 3", "alice: running 5 CPU-only, 12 GPU; pending 1 CPU-only, 2 GPU", "bob", "carol"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected self-describing compact content %q, got: %q", want, out)
		}
	}
}

func TestUserLinesBudgetOneRowUsesHiddenOnlyLabel(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(1, true, 80)
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "Users · job counts · 3 hidden") {
		t.Fatalf("expected hidden-only user label for one-row budget, got: %q", out)
	}
	if strings.Contains(out, "0 shown") {
		t.Fatalf("expected no top 0/N label, got: %q", out)
	}
}

func TestCompactViewKeepsExplicitJobCountsAndResourceSummary(t *testing.T) {
	m := seededModel()
	m.compact = true
	m.width = 90
	m.height = 36

	out := m.View()
	if strings.Contains(out, "heldCPU") || strings.Contains(out, "heldGPU") {
		t.Fatalf("expected compact view to hide held-resource columns, got: %q", out)
	}
	for _, want := range []string{"Queue ·", "Running ·", "Pending ·", "CPU-only", "GPU jobs", ": running ", "; pending "} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected compact view to include %q, got: %q", want, out)
		}
	}
}

func TestCompactStandardViewportShowsEveryAggregateView(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 72
	m.height = 20
	m.snapshot.Partitions = []slurm.PartitionSummary{{Name: "part-row"}}
	m.snapshot.Users = []slurm.UserSummary{{User: "user-row"}}
	m.snapshot.PendingReasons = []slurm.PendingReasonSummary{{Reason: "reason-row", Tasks: 1}}
	m.snapshot.Jobs = []slurm.JobSummary{{JobID: "job-row", User: "user-row", Partition: "part-row", State: "PENDING", Tasks: 1}}

	out := m.View()
	for _, want := range []string{"part-row", "user-row", "reason-row"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected compact 72x20 view to include %q, got: %q", want, out)
		}
	}
	if strings.Contains(out, "job-row") {
		t.Fatalf("did not expect the compact TUI to include individual jobs, got: %q", out)
	}
	if strings.Contains(out, viewportClipText) {
		t.Fatalf("did not expect global viewport clipping at 72x20, got: %q", out)
	}
}

func TestCompactStandardViewportUsesHeaderSpaceForData(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 72
	m.height = 20

	out := m.View()
	for _, want := range []string{"gpu:", "cpu:", "alice:", "bob:", "carol:", "Waiting for resources", "Waiting behind higher-priority jobs"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected compact 72x20 view to include %q, got: %q", want, out)
		}
	}
	if strings.Contains(out, "job 3001") || strings.Contains(out, "job 3002") {
		t.Fatalf("did not expect individual jobs in compact view, got: %q", out)
	}
	for _, unwanted := range []string{"CPU-only run", "GPU run", "CPU-only pending", "GPU pending", viewportClipText, "…"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("did not expect compact 72x20 view to include %q, got: %q", unwanted, out)
		}
	}
}

func TestWideViewUsesPlainLabelsAndAvailableReasonWidth(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 180
	m.height = 42
	reason := "Nodes required for job are DOWN, DRAINED or reserved for jobs in higher priority partitions"
	m.snapshot.Partitions = []slurm.PartitionSummary{{Name: "accelerated", Queue: m.snapshot.Queue}}
	m.snapshot.Users = []slurm.UserSummary{{User: "researcher", RunningCPUJobs: 2, RunningGPUJobs: 3, PendingCPUJobs: 4, PendingGPUJobs: 5, RunningCPU: 16, RunningGPU: 3, PendingCPU: 32, PendingGPU: 5}}
	m.snapshot.PendingReasons = []slurm.PendingReasonSummary{{Reason: reason, Tasks: 4, CPU: 32, GPU: 5}}
	m.snapshot.Jobs = []slurm.JobSummary{{JobID: "12345", User: "researcher", Partition: "accelerated", State: "PENDING", Reason: reason, Tasks: 4, CPU: 32, GPU: 5}}

	out := m.View()
	for _, want := range []string{
		"Queue ·",
		"CPU-only",
		"Partitions",
		"running jobs",
		"pending jobs",
		"resources in use",
		"resources requested",
		"Users",
		"Why jobs are pending",
		"affected tasks",
		"requested CPUs",
		"requested GPUs",
		reason,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected wide view to include %q, got %q", want, out)
		}
	}
	if strings.Contains(out, "12345") || strings.Contains(out, "job ID") {
		t.Fatalf("did not expect the wide TUI to include individual jobs, got %q", out)
	}
	for _, unwanted := range []string{"runCPUj", "runGPUj", "pendCPUj", "pendGPUj", "cpuJobs", "gpuJobs", " PEND "} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("did not expect abbreviation %q in wide view, got %q", unwanted, out)
		}
	}
}

func TestExpandedLayoutStartsAfterGroupedMetricsFit(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.height = 30
	queue := slurm.QueueSummary{PendingCPUJobs: 1454, PendingGPUJobs: 98}
	m.snapshot.Partitions = []slurm.PartitionSummary{{Name: "short", Queue: queue}}
	m.snapshot.Users = []slurm.UserSummary{{User: "researcher", PendingCPUJobs: 1454, PendingGPUJobs: 98}}

	m.width = 110
	compact := m.View()
	if strings.Contains(compact, "resources in use") || strings.Contains(compact, "…") {
		t.Fatalf("expected width 110 to keep the complete compact layout, got %q", compact)
	}

	m.width = 111
	expanded := m.View()
	if !strings.Contains(expanded, "resources in use") || strings.Count(expanded, "CPU-only 1454, GPU 98") != 2 {
		t.Fatalf("expected width 111 to show complete grouped metrics, got %q", expanded)
	}
	if strings.Contains(expanded, "…") {
		t.Fatalf("expected width 111 grouped metrics without truncation, got %q", expanded)
	}

	queue.PendingCPUJobs = 12345
	m.snapshot.Partitions[0].Queue = queue
	m.snapshot.Users[0].PendingCPUJobs = 12345
	m.width = 114
	compact = m.View()
	if strings.Contains(compact, "resources in use") || strings.Contains(compact, "…") {
		t.Fatalf("expected larger metrics to delay the expanded layout, got %q", compact)
	}

	m.width = 115
	expanded = m.View()
	if !strings.Contains(expanded, "resources in use") || strings.Count(expanded, "CPU-only 12345, GPU 98") != 2 {
		t.Fatalf("expected expanded layout when larger metrics fit, got %q", expanded)
	}
	if strings.Contains(expanded, "…") {
		t.Fatalf("expected larger grouped metrics without truncation, got %q", expanded)
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
	if strings.Contains(out, "hidden") {
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

	if !strings.Contains(out, "Users · 1 shown · 2 hidden") {
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
	if !strings.Contains(out, "Users ·") || !strings.Contains(out, "hidden") {
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
	for _, line := range lines {
		if !strings.Contains(line, "╭") {
			continue
		}
		if got := lipgloss.Width(strings.TrimRight(line, " ")); got != 89 {
			t.Fatalf("expected panel to use stabilized width 89, got %d", got)
		}
		return
	}
	t.Fatal("expected rendered panel border")
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
	if !strings.HasPrefix(lines[len(lines)-2], "╰") {
		t.Fatalf("expected the panel to fill the body above the footer, got: %q", lines[len(lines)-2])
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
			{Reason: "Resources", Tasks: 3, CPU: 64, GPU: 8},
			{Reason: "Priority", Tasks: 2, CPU: 32},
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
