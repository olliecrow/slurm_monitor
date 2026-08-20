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
	label      lipgloss.Style
	value      lipgloss.Style
	ok         lipgloss.Style
	warn       lipgloss.Style
	bad        lipgloss.Style
	chip       lipgloss.Style
	chipOK     lipgloss.Style
	chipWarn   lipgloss.Style
	chipBad    lipgloss.Style
	errorLabel lipgloss.Style
	accent     lipgloss.Style
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
	frameRightGutter = 1
	viewportClipText = "... output clipped to terminal height ..."
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
			label:      lipgloss.NewStyle().Bold(true),
			value:      lipgloss.NewStyle().Bold(true),
			ok:         lipgloss.NewStyle().Bold(true),
			warn:       lipgloss.NewStyle().Bold(true),
			bad:        lipgloss.NewStyle().Bold(true),
			chip:       lipgloss.NewStyle().Bold(true),
			chipOK:     lipgloss.NewStyle().Bold(true),
			chipWarn:   lipgloss.NewStyle().Bold(true),
			chipBad:    lipgloss.NewStyle().Bold(true),
			errorLabel: lipgloss.NewStyle().Bold(true),
			accent:     lipgloss.NewStyle().Bold(true),
		}
	}

	return styles{
		title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1),
		dim:        lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		panel:      basePanel.BorderForeground(lipgloss.Color("61")),
		tableHdr:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("60")).Padding(0, 1),
		label:      lipgloss.NewStyle().Foreground(lipgloss.Color("109")),
		value:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")),
		ok:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
		warn:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
		bad:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		chip:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Padding(0, 1),
		chipOK:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("28")).Padding(0, 1),
		chipWarn:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("232")).Background(lipgloss.Color("220")).Padding(0, 1),
		chipBad:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("160")).Padding(0, 1),
		errorLabel: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203")),
		accent:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")),
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
	separatorLines := 1
	if m.height <= headerLines+footerLines+4 {
		separatorLines = 0
	}
	bodyHeight := m.height - headerLines - footerLines - separatorLines
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	var body string
	if m.snapshot == nil {
		body = m.styles.panel.Width(max(20, m.width-6)).Render("waiting for first successful snapshot...")
		body = clipToHeight(body, bodyHeight)
	} else {
		body = m.renderMain(bodyHeight)
	}

	parts := []string{header}
	if separatorLines > 0 {
		parts = append(parts, "")
	}
	parts = append(parts, body)
	top := lipgloss.JoinVertical(lipgloss.Left, parts...)
	joined := pinFooterToBottom(top, footer, m.height)
	return clipToViewport(joined, viewWidth, m.height)
}

func (m Model) renderHeader(now time.Time) string {
	statusText, _, statusChip := m.renderStatusText(now)
	pulse := pulseFrames[m.pulseIndex%len(pulseFrames)]
	statusText = pulse + " " + statusText
	ageText := "refresh: never"
	if !m.lastSuccess.IsZero() {
		ageText = "refresh: " + humanDuration(now.Sub(m.lastSuccess)) + " ago"
	}

	title := m.styles.title.Render(" SLURM MONITOR ")
	source := m.styles.label.Render("source: ") + m.styles.value.Render(m.source)
	clock := m.styles.chip.Render("clock: " + now.Format("15:04:05"))
	age := m.styles.chip.Render(ageText)
	right := statusChip.Render(statusText)
	left := title
	for _, candidate := range []string{
		title + "  " + source + "  " + clock + " " + age,
		title + "  " + clock + " " + age,
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

func (m Model) renderStatusText(now time.Time) (string, lipgloss.Style, lipgloss.Style) {
	if m.snapshot == nil && strings.TrimSpace(m.lastError) == "" {
		return "loading", m.styles.warn, m.styles.chipWarn
	}

	switch m.state {
	case monitor.StateConnected:
		return "connected", m.styles.ok, m.styles.chipOK
	case monitor.StateDisconnected:
		return "disconnected", m.styles.bad, m.styles.chipBad
	case monitor.StateDisconnectedRecovering:
		next := ""
		if !m.nextRetry.IsZero() && m.nextRetry.After(now) {
			next = fmt.Sprintf(" (retry in %s)", humanDuration(m.nextRetry.Sub(now)))
		}
		return "disconnected, recovering" + next, m.styles.bad, m.styles.chipBad
	default:
		next := ""
		if !m.nextRetry.IsZero() && m.nextRetry.After(now) {
			next = fmt.Sprintf(" (retry in %s)", humanDuration(m.nextRetry.Sub(now)))
		}
		return "reconnecting" + next, m.styles.warn, m.styles.chipWarn
	}
}

func (m Model) renderMain(maxHeight int) string {
	if m.snapshot == nil {
		return ""
	}
	inner := max(20, m.width-6)
	contentWidth := panelContentWidth(inner)
	expanded := !m.compact && contentWidth >= 100
	body := m.renderInsightsPanelWithBudget(panelContentHeight(maxHeight), expanded, contentWidth)
	return clipToHeight(m.styles.panel.Width(inner).Render(body), maxHeight)
}

func (m Model) renderInsightsPanelWithBudget(contentHeight int, expanded bool, contentWidth int) string {
	if m.snapshot == nil {
		return "scheduler insights\n(no data)"
	}
	if contentHeight <= 0 {
		return ""
	}

	q := m.snapshot.Queue
	lines := []string{
		m.sectionTitle("scheduler overview"),
		schedulerSummaryHeaderLine(),
		schedulerSummaryRowLine("running", q.RunningCPUJobs, q.RunningGPUJobs, q.ResourceLoad.RunningCPU, q.ResourceLoad.RunningGPU),
		schedulerSummaryRowLine("pending", q.PendingCPUJobs, q.PendingGPUJobs, q.ResourceLoad.PendingCPU, q.ResourceLoad.PendingGPU),
		fmt.Sprintf("other jobs: %d  |  total jobs: %d", q.Other, q.TotalJobs()),
	}
	detailCaps := []int{
		detailLineCapacity(len(m.snapshot.Partitions)),
		detailLineCapacity(len(m.snapshot.Users)),
		detailLineCapacity(len(m.snapshot.PendingReasons)),
		detailLineCapacity(len(m.snapshot.Jobs)),
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

	detailBudgets := allocateLineBudgets(contentHeight-len(lines), detailCaps)
	if detailBudgets[0] > 0 {
		lines = append(lines, m.renderPartitionLinesWithBudget(detailBudgets[0], expanded, contentWidth)...)
	}
	if detailBudgets[1] > 0 {
		lines = append(lines, m.renderUserLinesWithBudget(detailBudgets[1], expanded, contentWidth)...)
	}
	if detailBudgets[2] > 0 {
		lines = append(lines, m.renderPendingReasonLinesWithBudget(detailBudgets[2], expanded, contentWidth)...)
	}
	if detailBudgets[3] > 0 {
		lines = append(lines, m.renderJobLinesWithBudget(detailBudgets[3], expanded, contentWidth)...)
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
	visibleRows := visibleRowsForBudget(len(reasons), rowBudget)
	lines := []string{m.sectionTitle(viewTitle("why jobs are pending", len(reasons), visibleRows))}
	if rowBudget == 1 {
		return fitLinesToWidth(lines, contentWidth)
	}
	if rowBudget == 2 {
		if visibleRows == 1 {
			lines = append(lines, compactPendingReasonSummaryLine(reasons[0], contentWidth))
		}
		return fitLinesToWidth(lines, contentWidth)
	}
	if wide {
		lines = append(lines, pendingReasonHeaderLine(contentWidth, true))
		for _, reason := range reasons[:visibleRows] {
			lines = append(lines, pendingReasonRowLine(reason, contentWidth, true))
		}
	} else {
		lines = append(lines, pendingReasonHeaderLine(contentWidth, false))
		for _, reason := range reasons[:visibleRows] {
			lines = append(lines, pendingReasonRowLine(reason, contentWidth, false))
		}
	}
	return fitLinesToWidth(clipLines(lines, rowBudget), contentWidth)
}

func detailLineCapacity(rowCount int) int {
	if rowCount == 0 {
		return 0
	}
	return rowCount + 2
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

func visibleRowsForBudget(totalRows, rowBudget int) int {
	if totalRows == 0 || rowBudget <= 1 {
		return 0
	}
	if rowBudget == 2 {
		return min(totalRows, 1)
	}
	return min(totalRows, rowBudget-2)
}

func viewTitle(name string, totalRows, visibleRows int) string {
	hiddenRows := totalRows - visibleRows
	if hiddenRows <= 0 {
		return name
	}
	if visibleRows == 0 {
		return fmt.Sprintf("%s (%d hidden)", name, hiddenRows)
	}
	return fmt.Sprintf("%s (showing %d of %d; %d hidden)", name, visibleRows, totalRows, hiddenRows)
}

func (m Model) renderPartitionLinesWithBudget(rowBudget int, wide bool, contentWidth int) []string {
	if m.snapshot == nil || rowBudget <= 0 {
		return nil
	}
	partitions := append([]slurm.PartitionSummary(nil), m.snapshot.Partitions...)
	slurm.SortPartitionsForDisplay(partitions)
	visibleRows := visibleRowsForBudget(len(partitions), rowBudget)
	lines := []string{m.sectionTitle(viewTitle("partitions", len(partitions), visibleRows))}
	if rowBudget == 1 {
		return fitLinesToWidth(lines, contentWidth)
	}
	if rowBudget == 2 {
		if visibleRows == 1 {
			lines = append(lines, compactPartitionRowLine(partitions[0]))
		}
		return fitLinesToWidth(lines, contentWidth)
	}
	if wide {
		lines = append(lines, groupedSummaryHeaderLine("partition", contentWidth))
		for _, partition := range partitions[:visibleRows] {
			lines = append(lines, groupedSummaryRowLine(partition.Name, partition.Queue, contentWidth))
		}
	} else {
		lines = append(lines, compactPartitionHeaderLine())
		for _, partition := range partitions[:visibleRows] {
			lines = append(lines, compactPartitionRowLine(partition))
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
	visibleRows := visibleRowsForBudget(totalUsers, rowBudget)
	visibleUsers := users[:visibleRows]
	lines := []string{m.sectionTitle(viewTitle("users", totalUsers, visibleRows))}
	if rowBudget == 1 {
		return fitLinesToWidth(lines, contentWidth)
	}
	if rowBudget == 2 {
		if len(visibleUsers) == 1 {
			lines = append(lines, compactUserRowLine(visibleUsers[0]))
		}
		return fitLinesToWidth(lines, contentWidth)
	}

	if expanded {
		lines = append(lines, groupedSummaryHeaderLine("user", contentWidth))
		for _, u := range visibleUsers {
			lines = append(lines, groupedSummaryRowLine(u.User, userQueueSummary(u), contentWidth))
		}
		lines = clipLines(lines, rowBudget)
		return fitLinesToWidth(lines, contentWidth)
	}

	lines = append(lines, compactUserHeaderLine())
	for _, u := range visibleUsers {
		lines = append(lines, compactUserRowLine(u))
	}
	lines = clipLines(lines, rowBudget)
	return fitLinesToWidth(lines, contentWidth)
}

func (m Model) renderJobLinesWithBudget(rowBudget int, wide bool, contentWidth int) []string {
	if m.snapshot == nil || rowBudget <= 0 {
		return nil
	}
	jobs := append([]slurm.JobSummary(nil), m.snapshot.Jobs...)
	slurm.SortJobsForDisplay(jobs)
	visibleRows := visibleRowsForBudget(len(jobs), rowBudget)
	lines := []string{m.sectionTitle(viewTitle("jobs", len(jobs), visibleRows))}
	if rowBudget == 1 {
		return fitLinesToWidth(lines, contentWidth)
	}
	if rowBudget == 2 {
		if visibleRows == 1 {
			lines = append(lines, compactJobSummaryLine(jobs[0], contentWidth))
		}
		return fitLinesToWidth(lines, contentWidth)
	}
	if wide {
		lines = append(lines, wideJobHeaderLine(contentWidth))
		for _, job := range jobs[:visibleRows] {
			lines = append(lines, wideJobRowLine(job, contentWidth))
		}
	} else {
		lines = append(lines, compactJobHeaderLine())
		for _, job := range jobs[:visibleRows] {
			lines = append(lines, compactJobRowLine(job))
		}
	}
	return fitLinesToWidth(clipLines(lines, rowBudget), contentWidth)
}

func groupedSummaryHeaderLine(nameLabel string, contentWidth int) string {
	nameWidth, metricWidth := groupedSummaryColumnWidths(contentWidth)
	return fmt.Sprintf(
		"%-*s %-*s %-*s %-*s %-*s",
		nameWidth, truncateRunes(nameLabel, nameWidth),
		metricWidth, truncateRunes("running jobs", metricWidth),
		metricWidth, truncateRunes("pending jobs", metricWidth),
		metricWidth, truncateRunes("resources in use", metricWidth),
		metricWidth, truncateRunes("resources requested", metricWidth),
	)
}

func groupedSummaryRowLine(name string, q slurm.QueueSummary, contentWidth int) string {
	nameWidth, metricWidth := groupedSummaryColumnWidths(contentWidth)
	runningJobs := fmt.Sprintf("CPU-only %d, GPU %d", q.RunningCPUJobs, q.RunningGPUJobs)
	pendingJobs := fmt.Sprintf("CPU-only %d, GPU %d", q.PendingCPUJobs, q.PendingGPUJobs)
	runningResources := fmt.Sprintf("CPUs %d, GPUs %d", q.ResourceLoad.RunningCPU, q.ResourceLoad.RunningGPU)
	pendingResources := fmt.Sprintf("CPUs %d, GPUs %d", q.ResourceLoad.PendingCPU, q.ResourceLoad.PendingGPU)
	return fmt.Sprintf(
		"%-*s %-*s %-*s %-*s %-*s",
		nameWidth, truncateRunes(name, nameWidth),
		metricWidth, truncateRunes(runningJobs, metricWidth),
		metricWidth, truncateRunes(pendingJobs, metricWidth),
		metricWidth, truncateRunes(runningResources, metricWidth),
		metricWidth, truncateRunes(pendingResources, metricWidth),
	)
}

func groupedSummaryColumnWidths(contentWidth int) (int, int) {
	const nameWidth = 16
	return nameWidth, max(1, (contentWidth-nameWidth-4)/4)
}

func compactPartitionHeaderLine() string {
	return fmt.Sprintf("%-10s %12s %7s %16s %11s", "partition", "CPU-only run", "GPU run", "CPU-only pending", "GPU pending")
}

func compactPartitionRowLine(partition slurm.PartitionSummary) string {
	q := partition.Queue
	return fmt.Sprintf("%-10s %12d %7d %16d %11d", truncateRunes(partition.Name, 10), q.RunningCPUJobs, q.RunningGPUJobs, q.PendingCPUJobs, q.PendingGPUJobs)
}

func wideJobHeaderLine(contentWidth int) string {
	reasonWidth := wideJobReasonWidth(contentWidth)
	return fmt.Sprintf("%-11s %-14s %-12s %-12s %-*s %7s %7s %7s", "job ID", "user", "partition", "state", reasonWidth, "reason", "tasks", "CPUs", "GPUs")
}

func wideJobRowLine(job slurm.JobSummary, contentWidth int) string {
	reasonWidth := wideJobReasonWidth(contentWidth)
	state := strings.ToUpper(strings.TrimSpace(job.State))
	return fmt.Sprintf("%-11s %-14s %-12s %-12s %-*s %7d %7d %7d", truncateRunes(job.JobID, 11), truncateRunes(job.User, 14), truncateRunes(job.Partition, 12), truncateRunes(state, 12), reasonWidth, truncateRunes(job.Reason, reasonWidth), job.Tasks, job.CPU, job.GPU)
}

func wideJobReasonWidth(contentWidth int) int {
	return max(18, contentWidth-77)
}

func compactJobHeaderLine() string {
	return fmt.Sprintf("%-10s %-9s %-9s %-11s %5s %5s %4s", "job ID", "user", "partition", "state", "tasks", "CPUs", "GPUs")
}

func compactJobRowLine(job slurm.JobSummary) string {
	state := strings.ToUpper(strings.TrimSpace(job.State))
	return fmt.Sprintf("%-10s %-9s %-9s %-11s %5d %5d %4d", truncateRunes(job.JobID, 10), truncateRunes(job.User, 9), truncateRunes(job.Partition, 9), truncateRunes(state, 11), job.Tasks, job.CPU, job.GPU)
}

func compactJobSummaryLine(job slurm.JobSummary, contentWidth int) string {
	state := strings.ToUpper(strings.TrimSpace(job.State))
	identity := fmt.Sprintf("job %s %s %s %s", job.JobID, job.User, job.Partition, state)
	resources := fmt.Sprintf("%d tasks, %d CPUs, %d GPUs", job.Tasks, job.CPU, job.GPU)
	return joinWithPaddingKeepRight(identity, resources, contentWidth)
}

func pendingReasonHeaderLine(contentWidth int, wide bool) string {
	reasonWidth, taskWidth, resourceWidth := pendingReasonColumnWidths(contentWidth, wide)
	if wide {
		return fmt.Sprintf("%-*s %*s %*s %*s", reasonWidth, "reason", taskWidth, "affected tasks", resourceWidth, "requested CPUs", resourceWidth, "requested GPUs")
	}
	return fmt.Sprintf("%-*s %*s %*s %*s", reasonWidth, "reason", taskWidth, "tasks", resourceWidth, "CPUs", resourceWidth, "GPUs")
}

func pendingReasonRowLine(reason slurm.PendingReasonSummary, contentWidth int, wide bool) string {
	reasonWidth, taskWidth, resourceWidth := pendingReasonColumnWidths(contentWidth, wide)
	return fmt.Sprintf("%-*s %*d %*d %*d", reasonWidth, truncateRunes(reason.Reason, reasonWidth), taskWidth, reason.Tasks, resourceWidth, reason.CPU, resourceWidth, reason.GPU)
}

func compactPendingReasonSummaryLine(reason slurm.PendingReasonSummary, contentWidth int) string {
	resources := fmt.Sprintf("%d tasks, %d CPUs, %d GPUs", reason.Tasks, reason.CPU, reason.GPU)
	return joinWithPaddingKeepRight(reason.Reason, resources, contentWidth)
}

func pendingReasonColumnWidths(contentWidth int, wide bool) (int, int, int) {
	if wide {
		const taskWidth = 14
		const resourceWidth = 14
		return max(18, contentWidth-taskWidth-2*resourceWidth-3), taskWidth, resourceWidth
	}
	const taskWidth = 7
	const resourceWidth = 7
	return max(18, contentWidth-taskWidth-2*resourceWidth-3), taskWidth, resourceWidth
}

func compactUserHeaderLine() string {
	return fmt.Sprintf("%-10s %12s %7s %16s %11s", "user", "CPU-only run", "GPU run", "CPU-only pending", "GPU pending")
}

func compactUserRowLine(u slurm.UserSummary) string {
	return fmt.Sprintf(
		"%-10s %12d %7d %16d %11d",
		truncateRunes(u.User, 10),
		u.RunningCPUJobs,
		u.RunningGPUJobs,
		u.PendingCPUJobs,
		u.PendingGPUJobs,
	)
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

func schedulerSummaryHeaderLine() string {
	return fmt.Sprintf("%-7s %13s %9s %8s %6s", "status", "CPU-only jobs", "GPU jobs", "CPUs", "GPUs")
}

func schedulerSummaryRowLine(state string, cpuOnlyJobs, gpuJobs, cpu, gpu int) string {
	return fmt.Sprintf("%-7s %13d %9d %8d %6d", state, cpuOnlyJobs, gpuJobs, cpu, gpu)
}

func (m Model) sectionTitle(label string) string {
	icon := "•"
	switch {
	case strings.HasPrefix(label, "scheduler overview"):
		icon = "◍"
	case strings.HasPrefix(label, "users"):
		icon = "◒"
	case strings.HasPrefix(label, "why jobs are pending"):
		icon = "◇"
	}
	return m.styles.tableHdr.Render(icon + " " + label)
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
