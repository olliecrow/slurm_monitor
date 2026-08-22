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
	if !strings.Contains(compactView, "CPU-only") || !strings.Contains(compactView, "Users · jobs: running / pending") {
		t.Fatalf("expected compact layout after narrowing the same model, got %q", compactView)
	}
	assertViewportBounds(t, compactView, 71, 20)

	wideWidth := aggregateGridLayoutForSnapshot(compact.snapshot, 1_000).width() + 1 + dashboardFrameHorizontalOverhead
	resized, _ = compact.Update(tea.WindowSizeMsg{Width: wideWidth, Height: 30})
	expanded := resized.(Model)
	expandedView := expanded.View()
	if strings.Contains(expandedView, "Users · jobs: running / pending") || !strings.Contains(expandedView, "CPU cores") || !strings.Contains(expandedView, "Requested") {
		t.Fatalf("expected expanded layout after widening the same model, got %q", expandedView)
	}
	assertViewportBounds(t, expandedView, wideWidth-1, 30)
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
		"All partitions",
		"14",
		"28",
		"640",
		"38",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected scheduler summary row %q in output, got: %q", want, out)
		}
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 4 || !strings.Contains(lines[1], "Available now") || lines[2] != "" || !strings.Contains(lines[3], "Partitions") {
		t.Fatalf("expected a blank line between the queue summary and partitions, got: %q", out)
	}
}

func TestSchedulerSummaryIncludesResourcesOnlyWhenNeeded(t *testing.T) {
	m := seededModel()
	wide := m.schedulerSummaryLines(m.snapshot, false, 120)
	if len(wide) != 2 || !strings.Contains(wide[0], "42 running") || !strings.Contains(wide[1], "Available now") || strings.Contains(strings.Join(wide, "\n"), "Resources") {
		t.Fatalf("expected the wide grid to carry detailed queue totals plus availability, got: %q", wide)
	}
	compact := m.schedulerSummaryLines(m.snapshot, true, 40)
	if len(compact) != 4 || compact[1] != "Resources · in use / requested / free" || !strings.HasPrefix(compact[2], "CPU cores ·") || !strings.HasPrefix(compact[3], "GPUs ·") {
		t.Fatalf("expected compact mode to retain queue-wide resource totals, got: %q", compact)
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

	out := m.renderInsightsPanelWithBudget(28, true, 140)
	for _, want := range []string{"Partitions", "gpu", "Users", "alice", "Why jobs are pending", "Waiting for resources", "tasks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in combined insight panel, got: %q", want, out)
		}
	}
	for _, unwanted := range []string{"job ID", "3001", "3002"} {
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
		"Partitions · jobs: running / pending · 2 shown · 1 hidden",
		"Users · 1 shown · 2 hidden",
		"Why jobs are pending · 2 shown · 1 hidden",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected deterministic clipping metadata %q, got: %q", want, out)
		}
	}
	if got := strings.Count(out, "\n\n"); got != 3 {
		t.Fatalf("expected spacing between all sections, got %d separators in %q", got, out)
	}
}

func TestResourceSummaryKeepsCPUAndGPUValuesDistinct(t *testing.T) {
	resources := slurm.ResourceTotals{RunningCPU: 1_646, PendingCPU: 1_061, RunningGPU: 93, PendingGPU: 111}
	available := slurm.AvailableResources{CPU: 384, GPU: 7}
	if got, want := strings.Join(resourceSummaryLines(resources, available, 160), "\n"), "Resources · CPU cores · 1,646 in use · 1,061 requested · 384 available   GPUs · 93 in use · 111 requested · 7 available"; got != want {
		t.Fatalf("wide rows=%q want=%q", got, want)
	}
	if got, want := strings.Join(resourceSummaryLines(resources, available, 110), "\n"), "Resources · in use / requested / free · CPU cores · 1,646 / 1,061 / 384 · GPUs · 93 / 111 / 7"; got != want {
		t.Fatalf("concise rows=%q want=%q", got, want)
	}
	if got, want := strings.Join(resourceSummaryLines(resources, available, 80), "\n"), "Resources · CPU cores · 1,646 in use · 1,061 requested · 384 available\n            GPUs · 93 in use · 111 requested · 7 available"; got != want {
		t.Fatalf("narrow rows=%q want=%q", got, want)
	}
	if got, want := strings.Join(resourceSummaryLines(resources, available, 40), "\n"), "Resources · in use / requested / free\nCPU cores · 1,646 / 1,061 / 384\nGPUs · 93 / 111 / 7"; got != want {
		t.Fatalf("very narrow rows=%q want=%q", got, want)
	}
}

func TestAvailabilitySummarySplitsWithoutLosingValues(t *testing.T) {
	available := slurm.AvailableResources{CPU: 1_040, GPU: 87}
	if got, want := strings.Join(availabilitySummaryLines(available, 80), "\n"), "Available now · CPU cores · 1,040   GPUs · 87"; got != want {
		t.Fatalf("wide rows=%q want=%q", got, want)
	}
	if got, want := strings.Join(availabilitySummaryLines(available, 30), "\n"), "Available now\nCPU cores · 1,040 available\nGPUs · 87 available"; got != want {
		t.Fatalf("narrow rows=%q want=%q", got, want)
	}
}

func TestPendingReasonLabelsUsePlainLanguageAndPreserveUnknownReasons(t *testing.T) {
	tests := map[string]string{
		"BeginTime":                "Waiting for scheduled start time",
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
	if !strings.Contains(out, "Waiting for resources") || !strings.Contains(out, "3 tasks, 64 CPUs, 8 GPUs") {
		t.Fatalf("expected a clear pending-reason detail, got %q", out)
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

func TestTightWideViewRetainsQueueResourceTotals(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	out := m.renderInsightsPanelWithBudget(9, true, 99)
	for _, want := range []string{
		"CPU cores · 640 / 96 / 384",
		"GPUs · 38 / 8 / 7",
		"gpu",
		"alice",
		"Waiting for resources",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected tight wide view to retain %q, got %q", want, out)
		}
	}
	if strings.Contains(out, viewportClipText) {
		t.Fatalf("did not expect viewport clipping in budgeted output, got %q", out)
	}
}

func TestWideViewWithoutPartitionsRetainsQueueResourceTotals(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.Partitions = nil

	out := m.renderInsightsPanelWithBudget(24, true, 120)
	for _, want := range []string{
		"Resources · CPU cores · 640 in use · 96 requested",
		"GPUs · 38 in use · 8 requested",
		"Users",
		"Why jobs are pending",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected partition-free wide view to retain %q, got %q", want, out)
		}
	}
}

func TestUserLinesBudgetTwoRowsShowsOneUser(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	lines := m.renderUserLinesWithBudget(2, true, 80)
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "Users · jobs: running / pending · 1 shown · 2 hidden") {
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

	lines := m.renderUserLinesWithBudget(6, true, 120)
	if len(lines) < 4 {
		t.Fatalf("expected header plus rows, got: %q", strings.Join(lines, "\n"))
	}

	for _, want := range []string{"CPU-only jobs", "GPU jobs", "CPU cores", "GPUs"} {
		if !strings.Contains(lines[1], want) {
			t.Fatalf("expected expanded user header %q, got %q", want, lines[1])
		}
	}
	for _, want := range []string{"Running", "Pending", "In use", "Requested"} {
		if !strings.Contains(lines[2], want) {
			t.Fatalf("expected expanded user subheader to include %q, got %q", want, lines[2])
		}
	}
	for _, row := range lines[3:5] {
		for _, unwanted := range []string{"running", "pending", "in use", "requested"} {
			if strings.Contains(row, unwanted) {
				t.Fatalf("did not expect repeated prose %q in numeric row %q", unwanted, row)
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
	layout := aggregateGridLayoutForSnapshot(&slurm.Snapshot{Partitions: []slurm.PartitionSummary{partition}}, 120)
	header := layout.groupHeaderLine() + "\n" + layout.subheaderLine("Partition")
	row := layout.rowLine(partition.Name, partition.Queue)
	for _, want := range []string{"CPU-only jobs", "GPU jobs", "CPU cores", "GPUs"} {
		if !strings.Contains(header, want) {
			t.Fatalf("expected expanded partition header %q, got %q", want, header)
		}
	}
	for _, want := range []string{"accelerated", "1", "3", "2", "4", "16", "32"} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected expanded partition row %q, got %q", want, row)
		}
	}
}

func TestWideAggregateTablesUseNaturalWidth(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	const availableWidth = 174
	wantWidth := aggregateGridLayoutForSnapshot(m.snapshot, availableWidth).width()

	partitionLines := m.renderPartitionLinesWithBudget(6, true, availableWidth)
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

func TestAggregateGridRightAlignsEveryNumericColumn(t *testing.T) {
	queue := slurm.QueueSummary{
		RunningCPUJobs: 101,
		PendingCPUJobs: 202,
		RunningGPUJobs: 303,
		PendingGPUJobs: 404,
		ResourceLoad: slurm.ResourceTotals{
			RunningCPU: 505,
			PendingCPU: 606,
			RunningGPU: 707,
			PendingGPU: 808,
		},
	}
	snapshot := &slurm.Snapshot{Queue: queue}
	layout := aggregateGridLayoutForSnapshot(snapshot, 120)
	row := layout.rowLine("All partitions", queue)
	values := aggregateValues(queue)

	rightEdge := layout.nameWidth + aggregateNameGap
	for i, value := range values {
		rightEdge += layout.valueWidth[i]
		if got := strings.Index(row, value) + lipgloss.Width(value); got != rightEdge {
			t.Fatalf("column %d right edge=%d want=%d in %q", i, got, rightEdge, row)
		}
		if i == len(values)-1 {
			continue
		}
		if i%2 == 1 {
			rightEdge += aggregateGroupGap
		} else {
			rightEdge += aggregateColumnGap
		}
	}
	if got := lipgloss.Width(row); got != layout.width() {
		t.Fatalf("row width=%d want=%d", got, layout.width())
	}
}

func TestAggregateGridBreakpointIsStableForNormalDigitGrowth(t *testing.T) {
	small := &slurm.Snapshot{Queue: slurm.QueueSummary{RunningCPUJobs: 1, ResourceLoad: slurm.ResourceTotals{RunningCPU: 1}}}
	large := &slurm.Snapshot{Queue: slurm.QueueSummary{RunningCPUJobs: 999_999, ResourceLoad: slurm.ResourceTotals{RunningCPU: 999_999}}}
	if got, want := aggregateGridLayoutForSnapshot(large, 120).width(), aggregateGridLayoutForSnapshot(small, 120).width(); got != want {
		t.Fatalf("normal digit growth changed the responsive breakpoint: got %d want %d", got, want)
	}
}

func TestFormatCountUsesThousandsSeparators(t *testing.T) {
	for value, want := range map[int]string{0: "0", 999: "999", 1_000: "1,000", 1_234_567: "1,234,567"} {
		if got := formatCount(value); got != want {
			t.Errorf("formatCount(%d)=%q want %q", value, got, want)
		}
	}
}

func TestWidePendingReasonTableAlignsWithAggregateGrid(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.snapshot.PendingReasons = []slurm.PendingReasonSummary{
		{Reason: "Dependency", Tasks: 4, CPU: 16},
		{Reason: "DependencyNeverSatisfied", Tasks: 2, GPU: 2},
	}
	const availableWidth = 174
	wantWidth := max(pendingReasonTableWidth(m.snapshot.PendingReasons), aggregateGridLayoutForSnapshot(m.snapshot, availableWidth).width())

	lines := m.renderPendingReasonLinesWithBudget(4, true, availableWidth)
	if got := lipgloss.Width(lines[1]); got != wantWidth {
		t.Fatalf("expected pending-reason table width %d, got %d in %q", wantWidth, got, lines[1])
	}
	if got := lipgloss.Width(lines[1]); got >= availableWidth {
		t.Fatalf("expected pending-reason table not to stretch across %d columns, got %d", availableWidth, got)
	}
}

func TestUserColumnsDoNotShowHeldTotals(t *testing.T) {
	snapshot := sampleSnapshot()
	layout := aggregateGridLayoutForSnapshot(&snapshot, 120)
	wideHeader := layout.groupHeaderLine()
	compactRow := compactAggregateRowLine("alice", userQueueSummary(snapshot.Users[0]), layout, 80)
	if strings.Contains(wideHeader, "heldCPU") || strings.Contains(wideHeader, "heldGPU") {
		t.Fatalf("wide user header should not show held totals: %q", wideHeader)
	}
	if strings.Contains(compactRow, "640") || strings.Contains(compactRow, "38") {
		t.Fatalf("compact user row should not show resource totals: %q", compactRow)
	}
}

func TestCompactAggregateRowPreservesCompleteMetricsBeforeLongNames(t *testing.T) {
	q := slurm.QueueSummary{PendingCPUJobs: 1, RunningGPUJobs: 24, PendingGPUJobs: 23}
	snapshot := &slurm.Snapshot{Queue: q, Users: []slurm.UserSummary{{User: "long-research-username", PendingCPUJobs: 1, RunningGPUJobs: 24, PendingGPUJobs: 23}}}
	row := compactAggregateRowLine("long-research-username", q, aggregateGridLayoutForSnapshot(snapshot, 65), 65)
	if lipgloss.Width(row) > 65 {
		t.Fatalf("expected compact row to fit 65 columns, got %d in %q", lipgloss.Width(row), row)
	}
	if !strings.Contains(row, "CPU-only 0 / 1") || !strings.Contains(row, "GPU 24 / 23") {
		t.Fatalf("expected complete CPU and GPU metrics, got %q", row)
	}
	if !strings.Contains(row, "…") {
		t.Fatalf("expected the name to shorten before the metrics, got %q", row)
	}
}

func TestWideUserColumnsDistinguishJobsFromResources(t *testing.T) {
	snapshot := sampleSnapshot()
	layout := aggregateGridLayoutForSnapshot(&snapshot, 120)
	header := layout.groupHeaderLine() + "\n" + layout.subheaderLine("User")
	row := layout.rowLine("alice", userQueueSummary(snapshot.Users[0]))
	for _, want := range []string{"CPU-only jobs", "GPU jobs", "CPU cores", "GPUs"} {
		if !strings.Contains(header, want) {
			t.Fatalf("expected distinct job and resource group %q, got %q", want, header)
		}
	}
	for _, want := range []string{"Running", "Pending", "In use", "Requested"} {
		if !strings.Contains(header, want) {
			t.Fatalf("expected explicit job and resource unit %q, got %q", want, header)
		}
	}
	if strings.Contains(row, "running") || strings.Contains(row, "requested") {
		t.Fatalf("expected values to remain numeric and aligned, got %q", row)
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
	if strings.Contains(out, "CPU cores") {
		t.Fatalf("did not expect a compact table header, got: %q", out)
	}
	for _, want := range []string{"Users · jobs: running / pending · 3", "alice", "CPU-only", "GPU", "bob", "carol"} {
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
	if !strings.Contains(out, "Users · jobs: running / pending · 3 hidden") {
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
	for _, want := range []string{"Queue ·", "Resources ·", "CPU cores ·", "GPUs ·", "CPU-only", "GPU", "running / pending"} {
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
	for _, want := range []string{"gpu", "cpu", "alice", "carol", "Users · jobs: running / pending · 2 shown · 1 hidden", "Waiting for resources"} {
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
		"All partitions",
		"Partitions",
		"CPU-only jobs",
		"GPU jobs",
		"CPU cores",
		"GPUs",
		"Users",
		"Why jobs are pending",
		"Affected tasks",
		"Requested CPUs",
		"Requested GPUs",
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

func TestExpandedLayoutStartsWhenNumericGridFits(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.height = 30
	queue := slurm.QueueSummary{PendingCPUJobs: 1454, PendingGPUJobs: 98}
	m.snapshot.Partitions = []slurm.PartitionSummary{{Name: "short", Queue: queue}}
	m.snapshot.Users = []slurm.UserSummary{{User: "researcher", PendingCPUJobs: 1454, PendingGPUJobs: 98}}

	firstExpandedWidth := aggregateGridLayoutForSnapshot(m.snapshot, 1_000).width() + 1 + dashboardFrameHorizontalOverhead
	m.width = firstExpandedWidth - 1
	compact := m.View()
	if !strings.Contains(compact, "Users · jobs: running / pending") || strings.Contains(compact, "…") {
		t.Fatalf("expected the last narrow width to keep the complete compact layout, got %q", compact)
	}

	m.width = firstExpandedWidth
	expanded := m.View()
	if strings.Contains(expanded, "Users · jobs: running / pending") || !strings.Contains(expanded, "1,454") {
		t.Fatalf("expected the first wide width to show complete grouped metrics, got %q", expanded)
	}
	if strings.Contains(expanded, "…") {
		t.Fatalf("expected width 111 grouped metrics without truncation, got %q", expanded)
	}

	queue.PendingCPUJobs = 12345
	m.snapshot.Partitions[0].Queue = queue
	m.snapshot.Users[0].PendingCPUJobs = 12345
	secondExpandedWidth := aggregateGridLayoutForSnapshot(m.snapshot, 1_000).width() + 1 + dashboardFrameHorizontalOverhead
	m.width = secondExpandedWidth - 1
	compact = m.View()
	if !strings.Contains(compact, "Users · jobs: running / pending") || strings.Contains(compact, "…") {
		t.Fatalf("expected larger metrics to delay the expanded layout, got %q", compact)
	}

	m.width = secondExpandedWidth
	expanded = m.View()
	if strings.Contains(expanded, "Users · jobs: running / pending") || !strings.Contains(expanded, "12,345") {
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

	out := strings.Join(m.renderUserLinesWithBudget(18, true, 96), "\n")
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

	if !strings.Contains(out, "Users · jobs: running / pending · 2 shown · 1 hidden") {
		t.Fatalf("expected capped user view label, got: %q", out)
	}
	if strings.Count(out, "alice") != 1 || strings.Count(out, "carol") != 1 {
		t.Fatalf("expected the top two users to render when capped, got: %q", out)
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

func TestViewUsesContentHeightFrameAtStabilizedWidth(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 90
	m.height = 24
	out := m.View()
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) != 89 {
			t.Fatalf("expected line %d width 89 after viewport stabilization, got %d", i+1, lipgloss.Width(line))
		}
	}
	for _, border := range []string{"╭", "╮", "╰", "╯", "│"} {
		if !strings.Contains(out, border) {
			t.Fatalf("expected dashboard frame glyph %q in %q", border, out)
		}
	}
	if !strings.HasPrefix(lines[1], "╭") {
		t.Fatalf("expected frame to begin directly below the header, got %q", lines[1])
	}
	if strings.Contains(lines[len(lines)-2], "│") {
		t.Fatalf("expected unused height outside the content-height frame, got %q", lines[len(lines)-2])
	}
}

func TestDashboardFrameUsesBalancedPaddingAndConnectedDivider(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)

	out := m.frameDashboard("Queue\n\nPartitions", 16)
	if got, want := out, "╭──────────────────╮\n│ Queue            │\n├──────────────────┤\n│ Partitions       │\n╰──────────────────╯"; got != want {
		t.Fatalf("frameDashboard()=%q want %q", got, want)
	}
}

func TestDashboardFrameConnectsRoomySectionsWithoutAddingHeight(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 90
	m.height = 24

	out := m.View()
	if got := strings.Count(out, "├"); got != 3 {
		t.Fatalf("expected three connected section dividers, got %d in %q", got, out)
	}
	if got := strings.Count(out, "┤"); got != 3 {
		t.Fatalf("expected three connected divider ends, got %d in %q", got, out)
	}
}

func TestDashboardFrameOmitsDividersWhenTightLayoutNeedsData(t *testing.T) {
	m := seededModel()
	m.styles = defaultStyles(true)
	m.width = 100
	m.height = 12

	out := m.View()
	if strings.Contains(out, "├") || strings.Contains(out, "┤") {
		t.Fatalf("did not expect dividers to replace data in a tight layout, got %q", out)
	}
}

func TestDashboardContentSizeReservesFrameAndPadding(t *testing.T) {
	tests := []struct {
		width, height         int
		wantWidth, wantHeight int
		wantFramed            bool
	}{
		{width: 71, height: 18, wantWidth: 67, wantHeight: 16, wantFramed: true},
		{width: 4, height: 10, wantWidth: 4, wantHeight: 10, wantFramed: false},
		{width: 20, height: 2, wantWidth: 20, wantHeight: 2, wantFramed: false},
	}
	for _, test := range tests {
		width, height, framed := dashboardContentSize(test.width, test.height)
		if width != test.wantWidth || height != test.wantHeight || framed != test.wantFramed {
			t.Fatalf("dashboardContentSize(%d, %d)=(%d, %d, %t) want (%d, %d, %t)", test.width, test.height, width, height, framed, test.wantWidth, test.wantHeight, test.wantFramed)
		}
	}
}

func TestLoadingViewKeepsDashboardFrame(t *testing.T) {
	m := NewModel(Options{Source: "local", Updates: make(chan monitor.Update), NoColor: true})
	m.width = 72
	m.height = 20

	out := m.View()
	for _, want := range []string{"╭", "Waiting for the first successful snapshot...", "╯"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected loading view to retain %q, got %q", want, out)
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
	if strings.TrimSpace(lines[len(lines)-2]) != "" {
		t.Fatalf("expected calm space below the content-height frame, got: %q", lines[len(lines)-2])
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
		Available: slurm.AvailableResources{CPU: 384, GPU: 7, SchedulableNodes: 6, TotalNodes: 8},
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
