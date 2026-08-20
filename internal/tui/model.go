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

	left := m.styles.title.Render(" SLURM MONITOR ") + "  " +
		m.styles.label.Render("source: ") + m.styles.value.Render(m.source) + "  " +
		m.styles.chip.Render("clock: "+now.Format("15:04:05")) + " " +
		m.styles.chip.Render(ageText)
	right := statusChip.Render(statusText)
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
	expanded := !m.compact && contentWidth >= 84
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
		m.sectionTitle("scheduler summary"),
		m.queueStatusLine("running cpu jobs", q.RunningCPUJobs),
		m.queueStatusLine("running gpu jobs", q.RunningGPUJobs),
		m.queueStatusLine("pending cpu jobs", q.PendingCPUJobs),
		m.queueStatusLine("pending gpu jobs", q.PendingGPUJobs),
		m.queueStatusLine("other", q.Other),
		m.queueStatusLine("total", q.TotalJobs()),
		m.queueResourceLine("running resources", q.ResourceLoad.RunningCPU, q.ResourceLoad.RunningGPU),
		m.queueResourceLine("pending demand", q.ResourceLoad.PendingCPU, q.ResourceLoad.PendingGPU),
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
		lines = append(lines, m.renderJobLinesWithBudget(detailBudgets[3], contentWidth >= 88, contentWidth)...)
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
	lines := []string{m.sectionTitle(viewTitle("pending reasons", len(reasons), visibleRows))}
	if rowBudget == 1 {
		return fitLinesToWidth(lines, contentWidth)
	}
	if rowBudget == 2 {
		if visibleRows == 1 {
			lines = append(lines, compactPendingReasonRowLine(reasons[0]))
		}
		return fitLinesToWidth(lines, contentWidth)
	}
	if wide {
		lines = append(lines, widePendingReasonHeaderLine())
		for _, reason := range reasons[:visibleRows] {
			lines = append(lines, widePendingReasonRowLine(reason))
		}
	} else {
		lines = append(lines, compactPendingReasonHeaderLine())
		for _, reason := range reasons[:visibleRows] {
			lines = append(lines, compactPendingReasonRowLine(reason))
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
		return fmt.Sprintf("%s (+%d hidden)", name, hiddenRows)
	}
	return fmt.Sprintf("%s (top %d/%d, +%d hidden)", name, visibleRows, totalRows, hiddenRows)
}

func (m Model) renderPartitionLinesWithBudget(rowBudget int, wide bool, contentWidth int) []string {
	if m.snapshot == nil || rowBudget <= 0 {
		return nil
	}
	partitions := append([]slurm.PartitionSummary(nil), m.snapshot.Partitions...)
	slurm.SortPartitionsForDisplay(partitions)
	visibleRows := visibleRowsForBudget(len(partitions), rowBudget)
	lines := []string{m.sectionTitle(viewTitle("partition view", len(partitions), visibleRows))}
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
		lines = append(lines, widePartitionHeaderLine())
		for _, partition := range partitions[:visibleRows] {
			lines = append(lines, widePartitionRowLine(partition))
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
	lines := []string{m.sectionTitle(viewTitle("user view", totalUsers, visibleRows))}
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
		lines = append(lines, wideUserHeaderLine())
		for _, u := range visibleUsers {
			lines = append(lines, wideUserRowLine(u))
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
	lines := []string{m.sectionTitle(viewTitle("job view", len(jobs), visibleRows))}
	if rowBudget == 1 {
		return fitLinesToWidth(lines, contentWidth)
	}
	if rowBudget == 2 {
		if visibleRows == 1 {
			lines = append(lines, compactJobRowLine(jobs[0]))
		}
		return fitLinesToWidth(lines, contentWidth)
	}
	if wide {
		lines = append(lines, wideJobHeaderLine())
		for _, job := range jobs[:visibleRows] {
			lines = append(lines, wideJobRowLine(job))
		}
	} else {
		lines = append(lines, compactJobHeaderLine())
		for _, job := range jobs[:visibleRows] {
			lines = append(lines, compactJobRowLine(job))
		}
	}
	return fitLinesToWidth(clipLines(lines, rowBudget), contentWidth)
}

func widePartitionHeaderLine() string {
	return fmt.Sprintf("%-12s %7s %7s %8s %8s %7s %8s %7s %8s", "partition", "runCPUj", "runGPUj", "pendCPUj", "pendGPUj", "runCPU", "pendCPU", "runGPU", "pendGPU")
}

func widePartitionRowLine(partition slurm.PartitionSummary) string {
	q := partition.Queue
	return fmt.Sprintf("%-12s %7d %7d %8d %8d %7d %8d %7d %8d", truncateRunes(partition.Name, 12), q.RunningCPUJobs, q.RunningGPUJobs, q.PendingCPUJobs, q.PendingGPUJobs, q.ResourceLoad.RunningCPU, q.ResourceLoad.PendingCPU, q.ResourceLoad.RunningGPU, q.ResourceLoad.PendingGPU)
}

func compactPartitionHeaderLine() string {
	return fmt.Sprintf("%-10s %4s %4s %4s %4s", "partition", "rCJ", "rGJ", "pCJ", "pGJ")
}

func compactPartitionRowLine(partition slurm.PartitionSummary) string {
	q := partition.Queue
	return fmt.Sprintf("%-10s %4d %4d %4d %4d", truncateRunes(partition.Name, 10), q.RunningCPUJobs, q.RunningGPUJobs, q.PendingCPUJobs, q.PendingGPUJobs)
}

func wideJobHeaderLine() string {
	return fmt.Sprintf("%-12s %-13s %-10s %-7s %-18s %5s %7s %5s", "job", "user", "partition", "state", "reason", "tasks", "cpu", "gpu")
}

func wideJobRowLine(job slurm.JobSummary) string {
	return fmt.Sprintf("%-12s %-13s %-10s %-7s %-18s %5d %7d %5d", truncateRunes(job.JobID, 12), truncateRunes(job.User, 13), truncateRunes(job.Partition, 10), truncateRunes(job.State, 7), truncateRunes(job.Reason, 18), job.Tasks, job.CPU, job.GPU)
}

func compactJobHeaderLine() string {
	return fmt.Sprintf("%-10s %-10s %-8s %-4s %5s %5s %4s", "job", "user", "queue", "st", "tasks", "cpu", "gpu")
}

func compactJobRowLine(job slurm.JobSummary) string {
	return fmt.Sprintf("%-10s %-10s %-8s %-4s %5d %5d %4d", truncateRunes(job.JobID, 10), truncateRunes(job.User, 10), truncateRunes(job.Partition, 8), shortJobState(job.State), job.Tasks, job.CPU, job.GPU)
}

func shortJobState(state string) string {
	switch slurmState := strings.ToUpper(strings.TrimSpace(state)); {
	case strings.Contains(slurmState, "PENDING"):
		return "PEND"
	case strings.Contains(slurmState, "RUNNING"):
		return "RUN"
	case strings.Contains(slurmState, "COMPLETING"):
		return "COMP"
	case strings.Contains(slurmState, "CONFIGURING"):
		return "CONF"
	default:
		return truncateRunes(slurmState, 4)
	}
}

func wideUserHeaderLine() string {
	return fmt.Sprintf("%-12s %7s %7s %8s %8s %7s %8s %7s %8s", "user", "runCPUj", "runGPUj", "pendCPUj", "pendGPUj", "runCPU", "pendCPU", "runGPU", "pendGPU")
}

func wideUserRowLine(u slurm.UserSummary) string {
	return fmt.Sprintf(
		"%-12s %7d %7d %8d %8d %7d %8d %7d %8d",
		truncateRunes(u.User, 12),
		u.RunningCPUJobs,
		u.RunningGPUJobs,
		u.PendingCPUJobs,
		u.PendingGPUJobs,
		u.RunningCPU,
		u.PendingCPU,
		u.RunningGPU,
		u.PendingGPU,
	)
}

func widePendingReasonHeaderLine() string {
	return fmt.Sprintf("%-30s %7s %8s %8s", "reason", "jobs", "cpu", "gpu")
}

func widePendingReasonRowLine(reason slurm.PendingReasonSummary) string {
	return fmt.Sprintf("%-30s %7d %8d %8d", truncateRunes(reason.Reason, 30), reason.Jobs, reason.CPU, reason.GPU)
}

func compactPendingReasonHeaderLine() string {
	return fmt.Sprintf("%-18s %5s %6s %5s", "reason", "jobs", "cpu", "gpu")
}

func compactPendingReasonRowLine(reason slurm.PendingReasonSummary) string {
	return fmt.Sprintf("%-18s %5d %6d %5d", truncateRunes(reason.Reason, 18), reason.Jobs, reason.CPU, reason.GPU)
}

func compactUserHeaderLine() string {
	return fmt.Sprintf("%-10s %4s %4s %4s %4s", "user", "rCJ", "rGJ", "pCJ", "pGJ")
}

func compactUserRowLine(u slurm.UserSummary) string {
	return fmt.Sprintf(
		"%-10s %4d %4d %4d %4d",
		truncateRunes(u.User, 10),
		u.RunningCPUJobs,
		u.RunningGPUJobs,
		u.PendingCPUJobs,
		u.PendingGPUJobs,
	)
}

func (m Model) queueStatusLine(label string, value int) string {
	return m.styles.label.Render(fmt.Sprintf("%-16s", label)) + "  " + m.styles.value.Render(fmt.Sprintf("%5d", value))
}

func (m Model) queueResourceLine(label string, cpu, gpu int) string {
	resources := fmt.Sprintf("cpu=%d  gpu=%d", cpu, gpu)
	return m.styles.label.Render(fmt.Sprintf("%-16s", label)) + "  " + m.styles.value.Render(resources)
}

func (m Model) sectionTitle(label string) string {
	icon := "•"
	switch {
	case strings.HasPrefix(label, "scheduler summary"):
		icon = "◍"
	case strings.HasPrefix(label, "user view"):
		icon = "◒"
	case strings.HasPrefix(label, "pending reasons"):
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
