package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/olliecrow/slurm_monitor/internal/monitor"
	"github.com/olliecrow/slurm_monitor/internal/slurm"
)

type Options struct {
	Source  string
	Compact bool
	NoColor bool
	Updates <-chan monitor.Update
}

type Model struct {
	source  string
	compact bool
	updates <-chan monitor.Update

	width  int
	height int

	now time.Time

	state       monitor.State
	lastError   string
	lastSuccess time.Time
	nextRetry   time.Time
	pulseIndex  int
	snapshot    *slurm.Snapshot

	styles styles
}

type styles struct {
	title      lipgloss.Style
	dim        lipgloss.Style
	panel      lipgloss.Style
	tableHdr   lipgloss.Style
	value      lipgloss.Style
	chip       lipgloss.Style
	chipOK     lipgloss.Style
	chipWarn   lipgloss.Style
	chipBad    lipgloss.Style
	errorLabel lipgloss.Style
}

type updateMsg struct {
	update monitor.Update
}

type tickMsg struct {
	now time.Time
}

type channelClosedMsg struct{}

var pulseFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const (
	frameRightGutter           = 1
	groupedSummaryNameWidth    = 16
	groupedSummaryColumnGaps   = 4
	pendingReasonTaskWidth     = 14
	pendingReasonResourceWidth = 14
	pendingReasonColumnGaps    = 3
	viewportClipText           = "... output clipped to terminal height ..."
)

func NewModel(opts Options) Model {
	return Model{
		source:  opts.Source,
		compact: opts.Compact,
		updates: opts.Updates,
		now:     time.Now(),
		state:   monitor.StateReconnecting,
		styles:  defaultStyles(opts.NoColor),
	}
}

func defaultStyles(noColor bool) styles {
	basePanel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if noColor {
		return styles{
			title:      lipgloss.NewStyle().Bold(true),
			dim:        lipgloss.NewStyle(),
			panel:      basePanel,
			tableHdr:   lipgloss.NewStyle().Bold(true),
			value:      lipgloss.NewStyle().Bold(true),
			chip:       lipgloss.NewStyle().Bold(true),
			chipOK:     lipgloss.NewStyle().Bold(true),
			chipWarn:   lipgloss.NewStyle().Bold(true),
			chipBad:    lipgloss.NewStyle().Bold(true),
			errorLabel: lipgloss.NewStyle().Bold(true),
		}
	}

	return styles{
		title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1),
		dim:        lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		panel:      basePanel.BorderForeground(lipgloss.Color("61")),
		tableHdr:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("60")).Padding(0, 1),
		value:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")),
		chip:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Padding(0, 1),
		chipOK:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("28")).Padding(0, 1),
		chipWarn:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("232")).Background(lipgloss.Color("220")).Padding(0, 1),
		chipBad:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("160")).Padding(0, 1),
		errorLabel: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203")),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(waitForUpdate(m.updates), tickCmd())
}

func waitForUpdate(ch <-chan monitor.Update) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return channelClosedMsg{}
		}
		return updateMsg{update: update}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{now: t}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case updateMsg:
		m.state = msg.update.State
		m.lastError = msg.update.LastError
		m.lastSuccess = msg.update.LastSuccess
		m.nextRetry = msg.update.NextRetry
		if msg.update.Snapshot != nil {
			snap := *msg.update.Snapshot
			m.snapshot = &snap
			m.lastError = ""
		}
		return m, waitForUpdate(m.updates)
	case tickMsg:
		m.now = msg.now
		if len(pulseFrames) > 0 {
			m.pulseIndex = (m.pulseIndex + 1) % len(pulseFrames)
		}
		return m, tickCmd()
	case channelClosedMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) View() string {
	viewWidth := stabilizedFrameWidth(m.width)
	if viewWidth <= 0 || m.height <= 0 {
		return "initializing..."
	}
	m.width = viewWidth

	now := m.now
	if now.IsZero() {
		now = time.Now()
	}

	header := m.renderHeader(now)
	footer := m.styles.dim.Render("Ctrl+C to exit")
	headerLines := lineCount(header)
	footerLines := lineCount(footer)
	bodyHeight := m.height - headerLines - footerLines
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	var body string
	if m.snapshot == nil {
		body = m.styles.panel.Width(max(20, m.width-2)).Render("waiting for first successful snapshot...")
		body = clipToHeight(body, bodyHeight)
	} else {
		body = m.renderMain(bodyHeight)
	}

	parts := []string{header, body}
	top := lipgloss.JoinVertical(lipgloss.Left, parts...)
	joined := pinFooterToBottom(top, footer, m.height)
	return clipToViewport(joined, viewWidth, m.height)
}

func (m Model) renderHeader(now time.Time) string {
	statusText, statusChip := m.renderStatusText(now)
	pulse := pulseFrames[m.pulseIndex%len(pulseFrames)]
	statusText = pulse + " " + statusText
	ageText := "updated never"
	if !m.lastSuccess.IsZero() {
		ageText = "updated " + humanDuration(now.Sub(m.lastSuccess)) + " ago"
	}

	title := m.styles.title.Render("SLURM MONITOR")
	source := m.styles.value.Render(m.source)
	clock := m.styles.chip.Render(now.Format("15:04:05"))
	age := m.styles.chip.Render(ageText)
	right := statusChip.Render(statusText)
	left := title
	for _, candidate := range []string{
		title + "  " + source + "  " + age + "  " + clock,
		title + "  " + age + "  " + clock,
		title + "  " + age,
		title,
	} {
		if lipgloss.Width(candidate)+1+lipgloss.Width(right) <= m.width {
			left = candidate
			break
		}
	}
	line1 := joinWithPaddingKeepRight(left, right, m.width)
	if m.lastError == "" {
		return line1
	}
	line2 := truncateRunes(m.styles.errorLabel.Render("error: "+m.lastError), m.width)
	return line1 + "\n" + line2
}

func (m Model) renderStatusText(now time.Time) (string, lipgloss.Style) {
	if m.snapshot == nil && strings.TrimSpace(m.lastError) == "" {
		return "loading", m.styles.chipWarn
	}

	switch m.state {
	case monitor.StateConnected:
		return "connected", m.styles.chipOK
	case monitor.StateDisconnected:
		return "disconnected", m.styles.chipBad
	case monitor.StateDisconnectedRecovering:
		next := ""
		if !m.nextRetry.IsZero() && m.nextRetry.After(now) {
			next = fmt.Sprintf(" (retry in %s)", humanDuration(m.nextRetry.Sub(now)))
		}
		return "disconnected, recovering" + next, m.styles.chipBad
	default:
		next := ""
		if !m.nextRetry.IsZero() && m.nextRetry.After(now) {
			next = fmt.Sprintf(" (retry in %s)", humanDuration(m.nextRetry.Sub(now)))
		}
		return "reconnecting" + next, m.styles.chipWarn
	}
}

func (m Model) renderMain(maxHeight int) string {
	if m.snapshot == nil {
		return ""
	}
	inner := max(20, m.width-2)
	contentWidth := panelContentWidth(inner)
	expanded := !m.compact && contentWidth >= groupedSummaryMinContentWidth(m.snapshot)
	body := m.renderInsightsPanelWithBudget(panelContentHeight(maxHeight), expanded, contentWidth)
	return clipToHeight(m.styles.panel.Width(inner).Height(panelContentHeight(maxHeight)).Render(body), maxHeight)
}

func (m Model) renderInsightsPanelWithBudget(contentHeight int, expanded bool, contentWidth int) string {
	if m.snapshot == nil {
		return "scheduler insights\n(no data)"
	}
	if contentHeight <= 0 {
		return ""
	}

	lines := m.schedulerSummaryLines(m.snapshot.Queue, contentWidth)
	detailCaps := []int{
		detailLineCapacity(len(m.snapshot.Partitions), expanded),
		detailLineCapacity(len(m.snapshot.Users), expanded),
		detailLineCapacity(len(m.snapshot.PendingReasons), expanded),
	}
	activeDetailSections := 0
	for _, cap := range detailCaps {
		if cap > 0 {
			activeDetailSections++
		}
	}
	reservedDetailLines := min(contentHeight-1, activeDetailSections)
	if reservedDetailLines < 0 {
		reservedDetailLines = 0
	}
	summaryBudget := contentHeight - reservedDetailLines
	if len(lines) > summaryBudget {
		lines = clipLines(lines, summaryBudget)
	}

	separatorLines := 0
	if contentHeight-len(lines) >= 3*activeDetailSections {
		separatorLines = activeDetailSections
	}
	detailBudgets := allocateLineBudgets(contentHeight-len(lines)-separatorLines, detailCaps)
	if detailBudgets[0] > 0 {
		if separatorLines > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderPartitionLinesWithBudget(detailBudgets[0], expanded, contentWidth)...)
	}
	if detailBudgets[1] > 0 {
		if separatorLines > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderUserLinesWithBudget(detailBudgets[1], expanded, contentWidth)...)
	}
	if detailBudgets[2] > 0 {
		if separatorLines > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderPendingReasonLinesWithBudget(detailBudgets[2], expanded, contentWidth)...)
	}

	lines = clipLines(lines, contentHeight)
	lines = fitLinesToWidth(lines, contentWidth)
	return strings.Join(lines, "\n")
}

func (m Model) renderPendingReasonLinesWithBudget(rowBudget int, wide bool, contentWidth int) []string {
	if m.snapshot == nil || rowBudget <= 0 {
		return nil
	}
	reasons := append([]slurm.PendingReasonSummary(nil), m.snapshot.PendingReasons...)
	slurm.SortPendingReasonsForDisplay(reasons)
	tableHeader := wide && rowBudget >= 3
	visibleRows := visibleRowsForBudget(len(reasons), rowBudget, tableHeader)
	lines := []string{m.sectionTitle(viewTitle("Why jobs are pending", len(reasons), visibleRows))}
	if tableHeader {
		tableWidth := min(contentWidth, pendingReasonTableWidth(reasons[:visibleRows]))
		lines = append(lines, pendingReasonHeaderLine(tableWidth))
		for _, reason := range reasons[:visibleRows] {
			lines = append(lines, pendingReasonRowLine(reason, tableWidth))
		}
	} else {
		for _, reason := range reasons[:visibleRows] {
			lines = append(lines, compactPendingReasonSummaryLine(reason, contentWidth))
		}
	}
	return fitLinesToWidth(clipLines(lines, rowBudget), contentWidth)
}

func detailLineCapacity(rowCount int, expanded bool) int {
	if rowCount == 0 {
		return 0
	}
	if expanded {
		return rowCount + 2
	}
	return rowCount + 1
}

func allocateLineBudgets(total int, caps []int) []int {
	budgets := make([]int, len(caps))
	for total > 0 {
		advanced := false
		for i, cap := range caps {
			if total == 0 {
				break
			}
			if budgets[i] >= cap {
				continue
			}
			budgets[i]++
			total--
			advanced = true
		}
		if !advanced {
			break
		}
	}
	return budgets
}

func visibleRowsForBudget(totalRows, rowBudget int, tableHeader bool) int {
	overhead := 1
	if tableHeader {
		overhead++
	}
	if totalRows == 0 || rowBudget <= overhead {
		return 0
	}
	return min(totalRows, rowBudget-overhead)
}

func viewTitle(name string, totalRows, visibleRows int) string {
	hiddenRows := totalRows - visibleRows
	if hiddenRows <= 0 {
		return fmt.Sprintf("%s · %d", name, totalRows)
	}
	if visibleRows == 0 {
		return fmt.Sprintf("%s · %d hidden", name, hiddenRows)
	}
	return fmt.Sprintf("%s · %d shown · %d hidden", name, visibleRows, hiddenRows)
}

func (m Model) renderPartitionLinesWithBudget(rowBudget int, wide bool, contentWidth int) []string {
	if m.snapshot == nil || rowBudget <= 0 {
		return nil
	}
	partitions := append([]slurm.PartitionSummary(nil), m.snapshot.Partitions...)
	slurm.SortPartitionsForDisplay(partitions)
	tableHeader := wide && rowBudget >= 3
	visibleRows := visibleRowsForBudget(len(partitions), rowBudget, tableHeader)
	title := "Partitions"
	if !tableHeader {
		title += " · job counts"
	}
	lines := []string{m.sectionTitle(viewTitle(title, len(partitions), visibleRows))}
	if tableHeader {
		metricWidths := groupedSummaryMetricWidths(m.snapshot)
		lines = append(lines, groupedSummaryHeaderLine("partition", metricWidths))
		for _, partition := range partitions[:visibleRows] {
			lines = append(lines, groupedSummaryRowLine(partition.Name, partition.Queue, metricWidths))
		}
	} else {
		for _, partition := range partitions[:visibleRows] {
			lines = append(lines, compactGroupedSummaryRowLine(partition.Name, partition.Queue, contentWidth))
		}
	}
	return fitLinesToWidth(clipLines(lines, rowBudget), contentWidth)
}

func (m Model) renderUserLinesWithBudget(rowBudget int, expanded bool, contentWidth int) []string {
	if m.snapshot == nil || rowBudget <= 0 {
		return nil
	}
	users := append([]slurm.UserSummary(nil), m.snapshot.Users...)
	slurm.SortUsersForDisplay(users)

	totalUsers := len(users)
	tableHeader := expanded && rowBudget >= 3
	visibleRows := visibleRowsForBudget(totalUsers, rowBudget, tableHeader)
	visibleUsers := users[:visibleRows]
	title := "Users"
	if !tableHeader {
		title += " · job counts"
	}
	lines := []string{m.sectionTitle(viewTitle(title, totalUsers, visibleRows))}

	if tableHeader {
		metricWidths := groupedSummaryMetricWidths(m.snapshot)
		lines = append(lines, groupedSummaryHeaderLine("user", metricWidths))
		for _, u := range visibleUsers {
			lines = append(lines, groupedSummaryRowLine(u.User, userQueueSummary(u), metricWidths))
		}
		lines = clipLines(lines, rowBudget)
		return fitLinesToWidth(lines, contentWidth)
	}

	for _, u := range visibleUsers {
		lines = append(lines, compactGroupedSummaryRowLine(u.User, userQueueSummary(u), contentWidth))
	}
	lines = clipLines(lines, rowBudget)
	return fitLinesToWidth(lines, contentWidth)
}

func groupedSummaryHeaderLine(nameLabel string, metricWidths [4]int) string {
	headings := groupedSummaryHeadings()
	return fmt.Sprintf(
		"%-*s %-*s %-*s %-*s %-*s",
		groupedSummaryNameWidth, truncateRunes(nameLabel, groupedSummaryNameWidth),
		metricWidths[0], headings[0],
		metricWidths[1], headings[1],
		metricWidths[2], headings[2],
		metricWidths[3], headings[3],
	)
}

func groupedSummaryRowLine(name string, q slurm.QueueSummary, metricWidths [4]int) string {
	metrics := groupedSummaryMetrics(q)
	return fmt.Sprintf(
		"%-*s %-*s %-*s %-*s %-*s",
		groupedSummaryNameWidth, truncateRunes(name, groupedSummaryNameWidth),
		metricWidths[0], metrics[0],
		metricWidths[1], metrics[1],
		metricWidths[2], metrics[2],
		metricWidths[3], metrics[3],
	)
}

func groupedSummaryMinContentWidth(snapshot *slurm.Snapshot) int {
	metricWidths := groupedSummaryMetricWidths(snapshot)
	return groupedSummaryNameWidth + groupedSummaryColumnGaps + metricWidths[0] + metricWidths[1] + metricWidths[2] + metricWidths[3]
}

func groupedSummaryMetricWidths(snapshot *slurm.Snapshot) [4]int {
	headings := groupedSummaryHeadings()
	widths := [4]int{}
	for i, heading := range headings {
		widths[i] = lipgloss.Width(heading)
	}
	for _, partition := range snapshot.Partitions {
		for i, metric := range groupedSummaryMetrics(partition.Queue) {
			widths[i] = max(widths[i], lipgloss.Width(metric))
		}
	}
	for _, user := range snapshot.Users {
		for i, metric := range groupedSummaryMetrics(userQueueSummary(user)) {
			widths[i] = max(widths[i], lipgloss.Width(metric))
		}
	}
	return widths
}

func groupedSummaryHeadings() [4]string {
	return [4]string{"CPU-only jobs", "GPU jobs", "CPU cores (all jobs)", "GPUs"}
}

func groupedSummaryMetrics(q slurm.QueueSummary) [4]string {
	return [4]string{
		fmt.Sprintf("%d running, %d pending", q.RunningCPUJobs, q.PendingCPUJobs),
		fmt.Sprintf("%d running, %d pending", q.RunningGPUJobs, q.PendingGPUJobs),
		fmt.Sprintf("%d in use, %d requested", q.ResourceLoad.RunningCPU, q.ResourceLoad.PendingCPU),
		fmt.Sprintf("%d in use, %d requested", q.ResourceLoad.RunningGPU, q.ResourceLoad.PendingGPU),
	}
}

func compactGroupedSummaryRowLine(name string, q slurm.QueueSummary, contentWidth int) string {
	metrics := fmt.Sprintf(
		"CPU-only %d running/%d pending · GPU %d running/%d pending",
		q.RunningCPUJobs,
		q.PendingCPUJobs,
		q.RunningGPUJobs,
		q.PendingGPUJobs,
	)
	nameWidth := max(1, contentWidth-lipgloss.Width(metrics)-2)
	return fmt.Sprintf("%s: %s", truncateRunes(name, nameWidth), metrics)
}

func pendingReasonHeaderLine(contentWidth int) string {
	reasonWidth, taskWidth, resourceWidth := pendingReasonColumnWidths(contentWidth)
	return fmt.Sprintf("%-*s %*s %*s %*s", reasonWidth, "reason", taskWidth, "affected tasks", resourceWidth, "requested CPUs", resourceWidth, "requested GPUs")
}

func pendingReasonRowLine(reason slurm.PendingReasonSummary, contentWidth int) string {
	reasonWidth, taskWidth, resourceWidth := pendingReasonColumnWidths(contentWidth)
	return fmt.Sprintf("%-*s %*d %*d %*d", reasonWidth, truncatePendingReason(pendingReasonLabel(reason.Reason), reasonWidth), taskWidth, reason.Tasks, resourceWidth, reason.CPU, resourceWidth, reason.GPU)
}

func compactPendingReasonSummaryLine(reason slurm.PendingReasonSummary, contentWidth int) string {
	resources := fmt.Sprintf("%s, %s, %s", countNoun(reason.Tasks, "task"), countNoun(reason.CPU, "CPU"), countNoun(reason.GPU, "GPU"))
	labelWidth := max(0, contentWidth-lipgloss.Width(resources)-1)
	label := truncatePendingReason(pendingReasonLabel(reason.Reason), labelWidth)
	return joinWithPaddingKeepRight(label, resources, contentWidth)
}

func pendingReasonColumnWidths(contentWidth int) (int, int, int) {
	return max(18, contentWidth-pendingReasonTaskWidth-2*pendingReasonResourceWidth-pendingReasonColumnGaps), pendingReasonTaskWidth, pendingReasonResourceWidth
}

func pendingReasonTableWidth(reasons []slurm.PendingReasonSummary) int {
	reasonWidth := 18
	for _, reason := range reasons {
		reasonWidth = max(reasonWidth, lipgloss.Width(pendingReasonLabel(reason.Reason)))
	}
	return reasonWidth + pendingReasonTaskWidth + 2*pendingReasonResourceWidth + pendingReasonColumnGaps
}

func pendingReasonLabel(reason string) string {
	switch reason {
	case "BeginTime":
		return "Waiting for scheduled start time"
	case "Dependency":
		return "Waiting for dependency"
	case "DependencyNeverSatisfied":
		return "Dependency cannot be satisfied"
	case "JobArrayTaskLimit":
		return "Job array task limit reached"
	case "Priority":
		return "Waiting behind higher-priority jobs"
	case "ReqNodeNotAvail":
		return "Required node unavailable"
	case "Resources":
		return "Waiting for resources"
	}

	const unavailablePrefix = "ReqNodeNotAvail,"
	if strings.HasPrefix(reason, unavailablePrefix) {
		detail := strings.TrimSpace(strings.TrimPrefix(reason, unavailablePrefix))
		if detail != "" {
			return "Required node unavailable: " + detail
		}
		return "Required node unavailable"
	}
	return reason
}

func truncatePendingReason(reason string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(reason) <= maxWidth {
		return truncateRunes(reason, maxWidth)
	}

	truncated := truncateRunes(reason, maxWidth)
	words := strings.Fields(strings.TrimSuffix(truncated, "…"))
	if len(words) < 2 {
		return truncated
	}

	words = words[:len(words)-1]
	const trailingConnectors = " a an and are by for in is of on or the to with "
	for len(words) > 1 {
		last := strings.ToLower(strings.Trim(words[len(words)-1], ",.:;"))
		if !strings.Contains(trailingConnectors, " "+last+" ") {
			break
		}
		words = words[:len(words)-1]
	}
	candidate := strings.Join(words, " ") + "…"
	if lipgloss.Width(candidate) >= maxWidth/2 {
		return candidate
	}
	return truncated
}

func userQueueSummary(u slurm.UserSummary) slurm.QueueSummary {
	return slurm.QueueSummary{
		RunningCPUJobs: u.RunningCPUJobs,
		RunningGPUJobs: u.RunningGPUJobs,
		PendingCPUJobs: u.PendingCPUJobs,
		PendingGPUJobs: u.PendingGPUJobs,
		ResourceLoad: slurm.ResourceTotals{
			RunningCPU: u.RunningCPU,
			RunningGPU: u.RunningGPU,
			PendingCPU: u.PendingCPU,
			PendingGPU: u.PendingGPU,
		},
	}
}

func (m Model) schedulerSummaryLines(q slurm.QueueSummary, contentWidth int) []string {
	runningJobs := q.RunningCPUJobs + q.RunningGPUJobs
	pendingJobs := q.PendingCPUJobs + q.PendingGPUJobs
	title := fmt.Sprintf("Queue · %d jobs · %d running · %d pending", q.TotalJobs(), runningJobs, pendingJobs)
	if q.Other > 0 {
		title += fmt.Sprintf(" · %d other", q.Other)
	}
	lines := []string{m.sectionTitle(title)}
	lines = append(lines, groupedQueueLines(
		"Jobs",
		fmt.Sprintf("CPU-only: %d running, %d pending", q.RunningCPUJobs, q.PendingCPUJobs),
		fmt.Sprintf("GPU: %d running, %d pending", q.RunningGPUJobs, q.PendingGPUJobs),
		contentWidth,
	)...)
	return append(lines, groupedQueueLines(
		"Resources",
		fmt.Sprintf("CPU cores: %d in use, %d requested", q.ResourceLoad.RunningCPU, q.ResourceLoad.PendingCPU),
		fmt.Sprintf("GPUs: %d in use, %d requested", q.ResourceLoad.RunningGPU, q.ResourceLoad.PendingGPU),
		contentWidth,
	)...)
}

func groupedQueueLines(label, cpuText, gpuText string, contentWidth int) []string {
	combined := fmt.Sprintf("%s · %s  │  %s", label, cpuText, gpuText)
	if lipgloss.Width(combined) <= contentWidth {
		return []string{combined}
	}
	return []string{label + " · " + cpuText, strings.Repeat(" ", lipgloss.Width(label)+3) + gpuText}
}

func countNoun(count int, singular string) string {
	suffix := "s"
	if count == 1 {
		suffix = ""
	}
	return fmt.Sprintf("%d %s%s", count, singular, suffix)
}

func (m Model) sectionTitle(label string) string {
	return m.styles.tableHdr.Render(label)
}

func stabilizedFrameWidth(width int) int {
	if width <= 0 {
		return 0
	}
	if width <= frameRightGutter {
		return width
	}
	return width - frameRightGutter
}

func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return "<1s"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	return ansi.Truncate(s, maxRunes, "…")
}

func joinWithPaddingKeepRight(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	rightWidth := lipgloss.Width(right)
	if rightWidth >= width {
		return truncateRunes(right, width)
	}
	maxLeftWidth := width - rightWidth - 1
	if maxLeftWidth < 0 {
		maxLeftWidth = 0
	}
	left = truncateRunes(left, maxLeftWidth)
	leftWidth := lipgloss.Width(left)
	padding := width - leftWidth - rightWidth
	if padding < 1 {
		padding = 1
	}
	return left + strings.Repeat(" ", padding) + right
}

func clipToViewport(s string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	clipped := len(lines) > height
	if len(lines) > height {
		lines = lines[:height]
	}
	if clipped && len(lines) > 0 {
		lines[len(lines)-1] = truncateRunes(viewportClipText, width)
	}
	for i := range lines {
		lines[i] = truncateRunes(lines[i], width)
		if pad := width - lipgloss.Width(lines[i]); pad > 0 {
			lines[i] += strings.Repeat(" ", pad)
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func clipToHeight(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}

func pinFooterToBottom(top, footer string, height int) string {
	if height <= 0 {
		return ""
	}
	footerLines := []string{}
	if footer != "" {
		footerLines = strings.Split(footer, "\n")
	}
	topLines := []string{}
	if top != "" {
		topLines = strings.Split(top, "\n")
	}

	maxTopLines := height - len(footerLines)
	if maxTopLines < 0 {
		maxTopLines = 0
	}
	if len(topLines) > maxTopLines {
		topLines = topLines[:maxTopLines]
	}
	for len(topLines) < maxTopLines {
		topLines = append(topLines, "")
	}

	all := append(topLines, footerLines...)
	if len(all) == 0 {
		return ""
	}
	return strings.Join(all, "\n")
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func panelContentHeight(panelHeight int) int {
	return max(1, panelHeight-2)
}

func panelContentWidth(panelWidth int) int {
	return max(1, panelWidth-4)
}

func fitLinesToWidth(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = truncateRunes(line, width)
	}
	return out
}

func clipLines(lines []string, maxLines int) []string {
	if maxLines <= 0 || len(lines) == 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	return lines[:maxLines]
}
