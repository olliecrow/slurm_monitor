package tui

import (
	"fmt"
	"strconv"
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
	heading    lipgloss.Style
	section    lipgloss.Style
	total      lipgloss.Style
	value      lipgloss.Style
	statusOK   lipgloss.Style
	statusWarn lipgloss.Style
	statusBad  lipgloss.Style
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
	frameRightGutter                 = 1
	dashboardHorizontalPadding       = 1
	dashboardFrameHorizontalOverhead = 2 + 2*dashboardHorizontalPadding
	aggregateNameMinWidth            = 16
	aggregateNameMaxWidth            = 24
	aggregateNameGap                 = 2
	aggregateColumnGap               = 1
	aggregateGroupGap                = 3
	widePartitionMinimumLines        = 5
	pendingReasonTaskWidth           = 14
	pendingReasonResourceWidth       = 14
	pendingReasonColumnGaps          = 3
	viewportClipText                 = "... output clipped to terminal height ..."
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
	if noColor {
		return styles{
			title:      lipgloss.NewStyle().Bold(true),
			dim:        lipgloss.NewStyle(),
			heading:    lipgloss.NewStyle(),
			section:    lipgloss.NewStyle().Bold(true),
			total:      lipgloss.NewStyle().Bold(true),
			value:      lipgloss.NewStyle(),
			statusOK:   lipgloss.NewStyle().Bold(true),
			statusWarn: lipgloss.NewStyle().Bold(true),
			statusBad:  lipgloss.NewStyle().Bold(true),
			errorLabel: lipgloss.NewStyle().Bold(true),
		}
	}

	return styles{
		title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		dim:        lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		heading:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		section:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")),
		total:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")),
		value:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		statusOK:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
		statusWarn: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")),
		statusBad:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203")),
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

	body := m.renderDashboard(bodyHeight)
	top := lipgloss.JoinVertical(lipgloss.Left, header, body)
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
	clock := m.styles.dim.Render(now.Format("15:04:05"))
	age := m.styles.dim.Render(ageText)
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
		return "loading", m.styles.statusWarn
	}

	switch m.state {
	case monitor.StateConnected:
		return "connected", m.styles.statusOK
	case monitor.StateDisconnected:
		return "disconnected", m.styles.statusBad
	case monitor.StateDisconnectedRecovering:
		next := ""
		if !m.nextRetry.IsZero() && m.nextRetry.After(now) {
			next = fmt.Sprintf(" (retry in %s)", humanDuration(m.nextRetry.Sub(now)))
		}
		return "disconnected, recovering" + next, m.styles.statusBad
	default:
		next := ""
		if !m.nextRetry.IsZero() && m.nextRetry.After(now) {
			next = fmt.Sprintf(" (retry in %s)", humanDuration(m.nextRetry.Sub(now)))
		}
		return "reconnecting" + next, m.styles.statusWarn
	}
}

func (m Model) renderDashboard(maxHeight int) string {
	contentWidth, contentHeight, framed := dashboardContentSize(m.width, maxHeight)
	var content string
	if m.snapshot == nil {
		content = "Waiting for the first successful snapshot..."
	} else {
		expanded := !m.compact && aggregateGridLayoutForSnapshot(m.snapshot, contentWidth).width() <= contentWidth
		content = m.renderInsightsPanelWithBudget(contentHeight, expanded, contentWidth)
	}
	content = clipToHeight(content, contentHeight)
	if framed {
		return m.frameDashboard(content, contentWidth)
	}
	return content
}

func dashboardContentSize(frameWidth, maxHeight int) (int, int, bool) {
	contentWidth := frameWidth - dashboardFrameHorizontalOverhead
	contentHeight := maxHeight - 2
	if contentWidth < 1 || contentHeight < 1 {
		return max(1, frameWidth), max(1, maxHeight), false
	}
	return contentWidth, contentHeight, true
}

func (m Model) frameDashboard(content string, contentWidth int) string {
	border := func(s string) string { return m.styles.dim.Render(s) }
	horizontalWidth := contentWidth + 2*dashboardHorizontalPadding
	lines := []string{border("╭" + strings.Repeat("─", horizontalWidth) + "╮")}
	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			lines = append(lines, border("├"+strings.Repeat("─", horizontalWidth)+"┤"))
			continue
		}
		line = truncateRunes(line, contentWidth)
		line += strings.Repeat(" ", max(0, contentWidth-lipgloss.Width(line)))
		lines = append(lines,
			border("│")+
				strings.Repeat(" ", dashboardHorizontalPadding)+
				line+
				strings.Repeat(" ", dashboardHorizontalPadding)+
				border("│"),
		)
	}
	lines = append(lines, border("╰"+strings.Repeat("─", horizontalWidth)+"╯"))
	return strings.Join(lines, "\n")
}

func (m Model) renderInsightsPanelWithBudget(contentHeight int, expanded bool, contentWidth int) string {
	if m.snapshot == nil {
		return "scheduler insights\n(no data)"
	}
	if contentHeight <= 0 {
		return ""
	}

	detailCaps := []int{
		partitionLineCapacity(len(m.snapshot.Partitions), expanded),
		userLineCapacity(len(m.snapshot.Users), expanded),
		pendingReasonLineCapacity(len(m.snapshot.PendingReasons), expanded),
	}
	lines := m.schedulerSummaryLines(m.snapshot, !expanded, contentWidth)
	lines, separatorLines, detailBudgets := allocateInsightLineBudgets(contentHeight, lines, detailCaps)
	if expanded && (len(m.snapshot.Partitions) == 0 || detailBudgets[0] < widePartitionMinimumLines) {
		lines = m.schedulerSummaryLines(m.snapshot, true, contentWidth)
		lines, separatorLines, detailBudgets = allocateInsightLineBudgets(contentHeight, lines, detailCaps)
	}

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

func allocateInsightLineBudgets(contentHeight int, summaryLines []string, detailCaps []int) ([]string, int, []int) {
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
	if len(summaryLines) > summaryBudget {
		summaryLines = clipLines(summaryLines, summaryBudget)
	}

	separatorLines := 0
	if contentHeight-len(summaryLines) >= 3*activeDetailSections {
		separatorLines = activeDetailSections
	}
	detailBudgets := allocateLineBudgets(contentHeight-len(summaryLines)-separatorLines, detailCaps)
	return summaryLines, separatorLines, detailBudgets
}

func (m Model) renderPendingReasonLinesWithBudget(rowBudget int, wide bool, contentWidth int) []string {
	if m.snapshot == nil || rowBudget <= 0 {
		return nil
	}
	reasons := append([]slurm.PendingReasonSummary(nil), m.snapshot.PendingReasons...)
	slurm.SortPendingReasonsForDisplay(reasons)
	tableHeader := wide && rowBudget >= 3
	overhead := 1
	if tableHeader {
		overhead = 2
	}
	visibleRows := visibleRowsForBudget(len(reasons), rowBudget, overhead)
	lines := []string{m.sectionTitle(viewTitle("Why jobs are pending", len(reasons), visibleRows))}
	if tableHeader {
		tableWidth := min(contentWidth, max(pendingReasonTableWidth(reasons[:visibleRows]), aggregateGridLayoutForSnapshot(m.snapshot, contentWidth).width()))
		lines = append(lines, m.styles.heading.Render(pendingReasonHeaderLine(tableWidth)))
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

func partitionLineCapacity(rowCount int, expanded bool) int {
	if rowCount == 0 {
		return 0
	}
	if expanded {
		return rowCount + 4
	}
	return rowCount + 2
}

func userLineCapacity(rowCount int, expanded bool) int {
	if rowCount == 0 {
		return 0
	}
	if expanded {
		return rowCount + 3
	}
	return rowCount + 1
}

func pendingReasonLineCapacity(rowCount int, expanded bool) int {
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

func visibleRowsForBudget(totalRows, rowBudget, overhead int) int {
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
	tableHeader := wide && rowBudget >= widePartitionMinimumLines
	overhead := 2
	if tableHeader {
		overhead = 4
	}
	visibleRows := visibleRowsForBudget(len(partitions), rowBudget, overhead)
	title := "Partitions"
	if !tableHeader {
		title += " · jobs: running / pending"
	}
	lines := []string{m.sectionTitle(viewTitle(title, len(partitions), visibleRows))}
	layout := aggregateGridLayoutForSnapshot(m.snapshot, contentWidth)
	if tableHeader {
		lines = append(lines, m.styles.heading.Render(layout.groupHeaderLine()))
		lines = append(lines, m.styles.heading.Render(layout.subheaderLine("Partition")))
		lines = append(lines, m.styles.total.Render(layout.rowLine("All partitions", m.snapshot.Queue)))
		for _, partition := range partitions[:visibleRows] {
			lines = append(lines, layout.rowLine(partition.Name, partition.Queue))
		}
	} else {
		if rowBudget >= 2 {
			lines = append(lines, m.styles.total.Render(compactAggregateRowLine("All partitions", m.snapshot.Queue, layout, contentWidth)))
		}
		for _, partition := range partitions[:visibleRows] {
			lines = append(lines, compactAggregateRowLine(partition.Name, partition.Queue, layout, contentWidth))
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
	tableHeader := expanded && rowBudget >= 4
	overhead := 1
	if tableHeader {
		overhead = 3
	}
	visibleRows := visibleRowsForBudget(totalUsers, rowBudget, overhead)
	visibleUsers := users[:visibleRows]
	title := "Users"
	if !tableHeader {
		title += " · jobs: running / pending"
	}
	lines := []string{m.sectionTitle(viewTitle(title, totalUsers, visibleRows))}
	layout := aggregateGridLayoutForSnapshot(m.snapshot, contentWidth)

	if tableHeader {
		lines = append(lines, m.styles.heading.Render(layout.groupHeaderLine()))
		lines = append(lines, m.styles.heading.Render(layout.subheaderLine("User")))
		for _, u := range visibleUsers {
			lines = append(lines, layout.rowLine(u.User, userQueueSummary(u)))
		}
		lines = clipLines(lines, rowBudget)
		return fitLinesToWidth(lines, contentWidth)
	}

	for _, u := range visibleUsers {
		lines = append(lines, compactAggregateRowLine(u.User, userQueueSummary(u), layout, contentWidth))
	}
	lines = clipLines(lines, rowBudget)
	return fitLinesToWidth(lines, contentWidth)
}

type aggregateGridLayout struct {
	nameWidth     int
	valueWidth    [8]int
	jobValueWidth [4]int
}

var aggregateSubheadings = [8]string{
	"Running", "Pending",
	"Running", "Pending",
	"In use", "Requested",
	"In use", "Requested",
}

var aggregateGroupHeadings = [4]string{"CPU-only jobs", "GPU jobs", "CPU cores", "GPUs"}

func aggregateGridLayoutForSnapshot(snapshot *slurm.Snapshot, contentWidth int) aggregateGridLayout {
	layout := aggregateGridLayout{nameWidth: aggregateNameMinWidth}
	for i, heading := range aggregateSubheadings {
		layout.valueWidth[i] = lipgloss.Width(heading)
	}
	for i := range layout.jobValueWidth {
		layout.jobValueWidth[i] = 1
	}

	preferredNameWidth := aggregateNameMinWidth
	include := func(name string, q slurm.QueueSummary) {
		preferredNameWidth = max(preferredNameWidth, min(aggregateNameMaxWidth, lipgloss.Width(name)))
		values := aggregateValues(q)
		for i, value := range values {
			layout.valueWidth[i] = max(layout.valueWidth[i], lipgloss.Width(value))
			if i < len(layout.jobValueWidth) {
				layout.jobValueWidth[i] = max(layout.jobValueWidth[i], lipgloss.Width(value))
			}
		}
	}
	include("All partitions", snapshot.Queue)
	for _, partition := range snapshot.Partitions {
		include(partition.Name, partition.Queue)
	}
	for _, user := range snapshot.Users {
		include(user.User, userQueueSummary(user))
	}
	for group := 0; group < len(aggregateGroupHeadings); group++ {
		pairWidth := max(layout.valueWidth[2*group], layout.valueWidth[2*group+1])
		layout.valueWidth[2*group] = pairWidth
		layout.valueWidth[2*group+1] = pairWidth
	}
	for group := 0; group < 2; group++ {
		pairWidth := max(layout.jobValueWidth[2*group], layout.jobValueWidth[2*group+1])
		layout.jobValueWidth[2*group] = pairWidth
		layout.jobValueWidth[2*group+1] = pairWidth
	}

	layout.nameWidth = preferredNameWidth
	if layout.width() > contentWidth {
		layout.nameWidth = aggregateNameMinWidth
	}
	return layout
}

func (l aggregateGridLayout) width() int {
	width := l.nameWidth + aggregateNameGap
	for i, valueWidth := range l.valueWidth {
		width += valueWidth
		if i == len(l.valueWidth)-1 {
			continue
		}
		if i%2 == 1 {
			width += aggregateGroupGap
		} else {
			width += aggregateColumnGap
		}
	}
	return width
}

func (l aggregateGridLayout) groupHeaderLine() string {
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", l.nameWidth+aggregateNameGap))
	for group, label := range aggregateGroupHeadings {
		if group > 0 {
			b.WriteString(strings.Repeat(" ", aggregateGroupGap))
		}
		pairWidth := l.valueWidth[2*group] + aggregateColumnGap + l.valueWidth[2*group+1]
		b.WriteString(centerText(label, pairWidth))
	}
	return b.String()
}

func (l aggregateGridLayout) subheaderLine(nameLabel string) string {
	values := aggregateSubheadings
	return l.line(nameLabel, values, false)
}

func (l aggregateGridLayout) rowLine(name string, q slurm.QueueSummary) string {
	return l.line(name, aggregateValues(q), true)
}

func (l aggregateGridLayout) line(name string, values [8]string, rightAlign bool) string {
	var b strings.Builder
	b.WriteString(padRight(truncateRunes(name, l.nameWidth), l.nameWidth))
	b.WriteString(strings.Repeat(" ", aggregateNameGap))
	for i, value := range values {
		if i > 0 {
			gap := aggregateColumnGap
			if i%2 == 0 {
				gap = aggregateGroupGap
			}
			b.WriteString(strings.Repeat(" ", gap))
		}
		if rightAlign {
			b.WriteString(padLeft(value, l.valueWidth[i]))
		} else {
			b.WriteString(centerText(value, l.valueWidth[i]))
		}
	}
	return b.String()
}

func aggregateValues(q slurm.QueueSummary) [8]string {
	return [8]string{
		formatCount(q.RunningCPUJobs),
		formatCount(q.PendingCPUJobs),
		formatCount(q.RunningGPUJobs),
		formatCount(q.PendingGPUJobs),
		formatCount(q.ResourceLoad.RunningCPU),
		formatCount(q.ResourceLoad.PendingCPU),
		formatCount(q.ResourceLoad.RunningGPU),
		formatCount(q.ResourceLoad.PendingGPU),
	}
}

func compactAggregateRowLine(name string, q slurm.QueueSummary, layout aggregateGridLayout, contentWidth int) string {
	values := aggregateValues(q)
	metrics := fmt.Sprintf(
		"CPU-only %s / %s   GPU %s / %s",
		padLeft(values[0], layout.jobValueWidth[0]),
		padLeft(values[1], layout.jobValueWidth[1]),
		padLeft(values[2], layout.jobValueWidth[2]),
		padLeft(values[3], layout.jobValueWidth[3]),
	)
	nameWidth := min(layout.nameWidth, max(1, contentWidth-lipgloss.Width(metrics)-aggregateNameGap))
	return padRight(truncateRunes(name, nameWidth), nameWidth) + strings.Repeat(" ", aggregateNameGap) + metrics
}

func pendingReasonHeaderLine(contentWidth int) string {
	reasonWidth, taskWidth, resourceWidth := pendingReasonColumnWidths(contentWidth)
	return fmt.Sprintf("%-*s %*s %*s %*s", reasonWidth, "Reason", taskWidth, "Affected tasks", resourceWidth, "Requested CPUs", resourceWidth, "Requested GPUs")
}

func pendingReasonRowLine(reason slurm.PendingReasonSummary, contentWidth int) string {
	reasonWidth, taskWidth, resourceWidth := pendingReasonColumnWidths(contentWidth)
	return fmt.Sprintf("%-*s %*s %*s %*s", reasonWidth, truncatePendingReason(pendingReasonLabel(reason.Reason), reasonWidth), taskWidth, formatCount(reason.Tasks), resourceWidth, formatCount(reason.CPU), resourceWidth, formatCount(reason.GPU))
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

func (m Model) schedulerSummaryLines(snapshot *slurm.Snapshot, includeResources bool, contentWidth int) []string {
	q := snapshot.Queue
	runningJobs := q.RunningCPUJobs + q.RunningGPUJobs
	pendingJobs := q.PendingCPUJobs + q.PendingGPUJobs
	title := fmt.Sprintf("Queue · %s jobs · %s running · %s pending", formatCount(q.TotalJobs()), formatCount(runningJobs), formatCount(pendingJobs))
	if q.Other > 0 {
		title += fmt.Sprintf(" · %s other", formatCount(q.Other))
	}
	lines := []string{m.sectionTitle(title)}
	if !includeResources {
		return append(lines, availabilitySummaryLines(snapshot.Available, contentWidth)...)
	}
	return append(lines, resourceSummaryLines(q.ResourceLoad, snapshot.Available, contentWidth)...)
}

func availabilitySummaryLines(available slurm.AvailableResources, contentWidth int) []string {
	cpuText := fmt.Sprintf("CPU cores · %s", formatCount(available.CPU))
	gpuText := fmt.Sprintf("GPUs · %s", formatCount(available.GPU))
	combined := fmt.Sprintf("Available now · %s   %s", cpuText, gpuText)
	if lipgloss.Width(combined) <= contentWidth {
		return []string{combined}
	}
	return []string{
		"Available now",
		cpuText + " available",
		gpuText + " available",
	}
}

func resourceSummaryLines(resources slurm.ResourceTotals, available slurm.AvailableResources, contentWidth int) []string {
	runningCPU := formatCount(resources.RunningCPU)
	pendingCPU := formatCount(resources.PendingCPU)
	availableCPU := formatCount(available.CPU)
	runningGPU := formatCount(resources.RunningGPU)
	pendingGPU := formatCount(resources.PendingGPU)
	availableGPU := formatCount(available.GPU)
	cpuText := fmt.Sprintf("CPU cores · %s in use · %s requested · %s available", runningCPU, pendingCPU, availableCPU)
	gpuText := fmt.Sprintf("GPUs · %s in use · %s requested · %s available", runningGPU, pendingGPU, availableGPU)
	combined := fmt.Sprintf("Resources · %s   %s", cpuText, gpuText)
	if lipgloss.Width(combined) <= contentWidth {
		return []string{combined}
	}
	concise := fmt.Sprintf(
		"Resources · in use / requested / free · CPU cores · %s / %s / %s · GPUs · %s / %s / %s",
		runningCPU, pendingCPU, availableCPU, runningGPU, pendingGPU, availableGPU,
	)
	if lipgloss.Width(concise) <= contentWidth {
		return []string{concise}
	}
	split := []string{"Resources · " + cpuText, strings.Repeat(" ", lipgloss.Width("Resources · ")) + gpuText}
	if lipgloss.Width(split[0]) <= contentWidth && lipgloss.Width(split[1]) <= contentWidth {
		return split
	}
	return []string{
		"Resources · in use / requested / free",
		fmt.Sprintf("CPU cores · %s / %s / %s", runningCPU, pendingCPU, availableCPU),
		fmt.Sprintf("GPUs · %s / %s / %s", runningGPU, pendingGPU, availableGPU),
	}
}

func countNoun(count int, singular string) string {
	suffix := "s"
	if count == 1 {
		suffix = ""
	}
	return fmt.Sprintf("%s %s%s", formatCount(count), singular, suffix)
}

func formatCount(value int) string {
	raw := strconv.Itoa(value)
	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = strings.TrimPrefix(raw, "-")
	}
	if len(raw) <= 3 {
		return sign + raw
	}

	firstGroup := len(raw) % 3
	if firstGroup == 0 {
		firstGroup = 3
	}
	var b strings.Builder
	b.WriteString(sign)
	b.WriteString(raw[:firstGroup])
	for i := firstGroup; i < len(raw); i += 3 {
		b.WriteByte(',')
		b.WriteString(raw[i : i+3])
	}
	return b.String()
}

func padLeft(text string, width int) string {
	return strings.Repeat(" ", max(0, width-lipgloss.Width(text))) + text
}

func padRight(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

func centerText(text string, width int) string {
	space := max(0, width-lipgloss.Width(text))
	left := space / 2
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", space-left)
}

func (m Model) sectionTitle(label string) string {
	return m.styles.section.Render(label)
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
