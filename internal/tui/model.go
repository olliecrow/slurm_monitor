package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"slurm_monitor/internal/monitor"
	"slurm_monitor/internal/slurm"
	"slurm_monitor/internal/uifmt"
)

type Options struct {
	Source      string
	Compact     bool
	NoColor     bool
	Refresh     time.Duration
	MaxDuration time.Duration
	Updates     <-chan monitor.Update
}

type Model struct {
	source      string
	compact     bool
	noColor     bool
	refresh     time.Duration
	maxDuration time.Duration
	updates     <-chan monitor.Update

	width  int
	height int

	started time.Time
	now     time.Time

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
		source:      opts.Source,
		compact:     opts.Compact,
		noColor:     opts.NoColor,
		refresh:     opts.Refresh,
		maxDuration: opts.MaxDuration,
		updates:     opts.Updates,
		started:     time.Now(),
		now:         time.Now(),
		state:       monitor.StateReconnecting,
		styles:      defaultStyles(opts.NoColor),
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
		if m.maxDuration > 0 && m.now.Sub(m.started) >= m.maxDuration {
			return m, tea.Quit
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
	compactLayout := m.compact || m.width < 118 || maxHeight < 18

	queueTarget := max(9, maxHeight/2)
	nodeTarget := maxHeight - queueTarget
	if nodeTarget < 6 {
		nodeTarget = 6
		queueTarget = maxHeight - nodeTarget
	}
	if queueTarget < 6 {
		queueTarget = 6
		nodeTarget = max(3, maxHeight-queueTarget)
	}

	showDemandCols := contentWidth >= 62
	queueBodyHeight := panelContentHeight(queueTarget)
	queueBody := m.renderQueuePanelWithBudget(queueBodyHeight, maxHeight, compactLayout, showDemandCols, contentWidth)
	queuePanel := m.styles.panel.Width(inner).Render(queueBody)

	nodeBodyHeight := panelContentHeight(nodeTarget)
	nodeBody := m.renderNodeTableWithBudget(nodeBodyHeight, maxHeight, compactLayout, contentWidth)
	nodePanel := m.styles.panel.Width(inner).Render(nodeBody)

	body := lipgloss.JoinVertical(lipgloss.Left, nodePanel, queuePanel)
	return clipToHeight(body, maxHeight)
}

func (m Model) renderQueuePanel(userLimit int, showDemand bool) string {
	if m.snapshot == nil {
		return "queue summary\n(no data)"
	}
	q := m.snapshot.Queue
	total := q.Running + q.Pending + q.Other

	lines := []string{
		m.sectionTitle("queue summary"),
		m.queueStatusLine("running", q.Running),
		m.queueStatusLine("pending", q.Pending),
		m.queueStatusLine("other", q.Other),
		m.queueStatusLine("total", total),
	}

	lines = append(lines, "")
	lines = append(lines, m.renderUserLines(userLimit, showDemand)...)
	lines = fitLinesToWidth(lines, panelContentWidth(max(20, m.width-6)))
	return strings.Join(lines, "\n")
}

func (m Model) renderQueuePanelWithBudget(contentHeight, maxHeight int, compactLayout, showDemand bool, contentWidth int) string {
	if m.snapshot == nil {
		return "queue summary\n(no data)"
	}
	if contentHeight <= 0 {
		return ""
	}

	q := m.snapshot.Queue
	total := q.Running + q.Pending + q.Other
	lines := []string{
		m.sectionTitle("queue summary"),
		m.queueStatusLine("running", q.Running),
		m.queueStatusLine("pending", q.Pending),
		m.queueStatusLine("other", q.Other),
		m.queueStatusLine("total", total),
	}

	available := contentHeight - len(lines)
	if available > 1 {
		lines = append(lines, "")
		available--
	}

	if available > 0 {
		userRowBudget := available
		userRows := 0
		if userRowBudget > 2 {
			userRows = userRowBudget - 2
		}
		lines = append(lines, m.renderUserLinesWithBudget(userRows, userRowBudget, showDemand, contentWidth)...)
	}

	lines = clipLines(lines, contentHeight)
	lines = fitLinesToWidth(lines, contentWidth)
	return strings.Join(lines, "\n")
}

func (m Model) renderUserLines(limit int, showDemand bool) []string {
	if m.snapshot == nil {
		return []string{"users", "(no data)"}
	}
	users := append([]slurm.UserSummary(nil), m.snapshot.Users...)
	slurm.SortUsersByPendingDemand(users)

	if limit <= 0 {
		limit = 10
		if m.height > 40 {
			limit = 14
		} else if m.height < 22 {
			limit = 6
		}
	}
	totalUsers := len(users)
	if len(users) > limit {
		users = users[:limit]
	}
	hiddenUsers := totalUsers - len(users)
	title := "user view"
	if hiddenUsers > 0 {
		if len(users) == 0 {
			title = fmt.Sprintf("user view (+%d hidden)", hiddenUsers)
		} else {
			title = fmt.Sprintf("user view (top %d/%d, +%d hidden)", len(users), totalUsers, hiddenUsers)
		}
	}

	lines := []string{m.sectionTitle(title)}
	if showDemand {
		lines = append(lines, fmt.Sprintf("%-12s %7s %7s %14s %14s", "user", "running", "pending", "pendingCPUJobs", "pendingGPUJobs"))
		for _, u := range users {
			lines = append(lines, fmt.Sprintf(
				"%-12s %7d %7d %14d %14d",
				truncateRunes(u.User, 12),
				u.Running,
				u.Pending,
				u.PendingCPUJobs,
				u.PendingGPUJobs,
			))
		}
		return lines
	}

	lines = append(lines, fmt.Sprintf("%-18s %8s %8s", "user", "running", "pending"))
	for _, u := range users {
		lines = append(lines, fmt.Sprintf("%-18s %8d %8d", truncateRunes(u.User, 18), u.Running, u.Pending))
	}
	return fitLinesToWidth(lines, panelContentWidth(max(20, m.width-6)))
}

func (m Model) renderUserLinesWithBudget(maxRows, rowBudget int, showDemand bool, contentWidth int) []string {
	if m.snapshot == nil || rowBudget <= 0 {
		return nil
	}
	users := append([]slurm.UserSummary(nil), m.snapshot.Users...)
	slurm.SortUsersByPendingDemand(users)

	totalUsers := len(users)
	if maxRows < 0 {
		maxRows = 0
	}
	visibleRows := 0
	switch {
	case rowBudget <= 1:
		visibleRows = 0
	case rowBudget == 2:
		if totalUsers > 0 {
			visibleRows = min(maxRows, 1)
		}
	default:
		visibleRows = min(maxRows, rowBudget-2)
	}
	if visibleRows > totalUsers {
		visibleRows = totalUsers
	}
	visibleUsers := users[:visibleRows]
	hiddenUsers := totalUsers - len(visibleUsers)

	title := "user view"
	if hiddenUsers > 0 {
		if len(visibleUsers) == 0 {
			title = fmt.Sprintf("user view (+%d hidden)", hiddenUsers)
		} else {
			title = fmt.Sprintf("user view (top %d/%d, +%d hidden)", len(visibleUsers), totalUsers, hiddenUsers)
		}
	}
	lines := []string{m.sectionTitle(title)}
	if rowBudget == 1 {
		return fitLinesToWidth(lines, contentWidth)
	}
	if rowBudget == 2 {
		if len(visibleUsers) == 1 {
			u := visibleUsers[0]
			lines = append(lines, fmt.Sprintf("%-18s %8d %8d", truncateRunes(u.User, 18), u.Running, u.Pending))
		}
		return fitLinesToWidth(lines, contentWidth)
	}

	if showDemand {
		lines = append(lines, fmt.Sprintf("%-12s %7s %7s %14s %14s", "user", "running", "pending", "pendingCPUJobs", "pendingGPUJobs"))
		for _, u := range visibleUsers {
			lines = append(lines, fmt.Sprintf(
				"%-12s %7d %7d %14d %14d",
				truncateRunes(u.User, 12),
				u.Running,
				u.Pending,
				u.PendingCPUJobs,
				u.PendingGPUJobs,
			))
		}
		lines = clipLines(lines, rowBudget)
		return fitLinesToWidth(lines, contentWidth)
	}

	lines = append(lines, fmt.Sprintf("%-18s %8s %8s", "user", "running", "pending"))
	for _, u := range visibleUsers {
		lines = append(lines, fmt.Sprintf("%-18s %8d %8d", truncateRunes(u.User, 18), u.Running, u.Pending))
	}
	lines = clipLines(lines, rowBudget)
	return fitLinesToWidth(lines, contentWidth)
}

func (m Model) renderNodeTable(limit int) string {
	if m.snapshot == nil {
		return "node summary\n(no data)"
	}
	const (
		compactRowFmt = "%-14s %-9s %-10s %-9s %-13s %-13s"
		wideRowFmt    = "%-12s %-14s %-14s %-10s %-6s %-13s %-6s %-10s %-6s"
	)

	nodes := m.snapshot.Nodes
	totalNodes := len(nodes)
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}
	hiddenNodes := totalNodes - len(nodes)
	t := m.snapshot.Totals()

	compact := m.compact || m.width < 122
	title := "node summary"
	if hiddenNodes > 0 {
		title = fmt.Sprintf("node summary (top %d/%d, +%d hidden)", len(nodes), totalNodes, hiddenNodes)
	}
	contentWidth := panelContentWidth(max(20, m.width-6))
	lines := []string{m.sectionTitle(title)}
	if alert, ok := nodeStateAlert(m.snapshot); ok {
		lines = append(lines, m.styles.bad.Render(alert))
	}
	if compact {
		lines = append(lines, fmt.Sprintf(compactRowFmt, "node", "part", "state", "cpu", "mem", "gpu"))
		for _, n := range nodes {
			lines = append(lines, fmt.Sprintf(
				compactRowFmt,
				truncateRunes(n.Name, 14),
				truncateRunes(n.Partition, 9),
				truncateRunes(n.State, 10),
				uifmt.Ratio(n.CPUAlloc, n.CPUTotal),
				uifmt.MemPair(n.MemAllocMB, n.MemTotalMB),
				uifmt.Ratio(n.GPUAlloc, n.GPUTotal),
			))
		}
		totalLine := fmt.Sprintf(
			compactRowFmt,
			"TOTAL",
			"",
			"",
			uifmt.Ratio(t.CPUAlloc, t.CPUTotal),
			uifmt.MemPair(t.MemAllocMB, t.MemTotalMB),
			uifmt.Ratio(t.GPUAlloc, t.GPUTotal),
		)
		lines = append(lines, m.styles.accent.Render(totalLine))
		lines = fitLinesToWidth(lines, contentWidth)
		return strings.Join(lines, "\n")
	}

	lines = append(lines, fmt.Sprintf(
		wideRowFmt,
		"node", "partition", "state", "cpu", "cpu%", "mem", "mem%", "gpu", "gpu%",
	))
	for _, n := range nodes {
		lines = append(lines, fmt.Sprintf(
			wideRowFmt,
			truncateRunes(n.Name, 12),
			truncateRunes(n.Partition, 14),
			truncateRunes(n.State, 14),
			uifmt.Ratio(n.CPUAlloc, n.CPUTotal),
			uifmt.Percent(n.CPUUtil, n.HasCPU),
			uifmt.MemPair(n.MemAllocMB, n.MemTotalMB),
			uifmt.Percent(n.MemUtil, n.HasMem),
			uifmt.Ratio(n.GPUAlloc, n.GPUTotal),
			uifmt.Percent(n.GPUUtil, n.HasGPU),
		))
	}

	var cpuPct, memPct, gpuPct string
	if t.CPUTotal > 0 {
		cpuPct = fmt.Sprintf("%.1f%%", float64(t.CPUAlloc)/float64(t.CPUTotal)*100.0)
	} else {
		cpuPct = "n/a"
	}
	if t.MemTotalMB > 0 {
		memPct = fmt.Sprintf("%.1f%%", float64(t.MemAllocMB)/float64(t.MemTotalMB)*100.0)
	} else {
		memPct = "n/a"
	}
	if t.GPUTotal > 0 {
		gpuPct = fmt.Sprintf("%.1f%%", float64(t.GPUAlloc)/float64(t.GPUTotal)*100.0)
	} else {
		gpuPct = "n/a"
	}

	totalLine := fmt.Sprintf(
		wideRowFmt,
		"TOTAL",
		"",
		"",
		uifmt.Ratio(t.CPUAlloc, t.CPUTotal),
		cpuPct,
		uifmt.MemPair(t.MemAllocMB, t.MemTotalMB),
		memPct,
		uifmt.Ratio(t.GPUAlloc, t.GPUTotal),
		gpuPct,
	)
	lines = append(lines, m.styles.accent.Render(totalLine))
	lines = fitLinesToWidth(lines, contentWidth)
	return strings.Join(lines, "\n")
}

func (m Model) renderNodeTableWithBudget(contentHeight, maxHeight int, compactLayout bool, contentWidth int) string {
	if m.snapshot == nil || contentHeight <= 0 {
		return ""
	}
	const (
		compactRowFmt = "%-14s %-9s %-10s %-9s %-13s %-13s"
		wideRowFmt    = "%-12s %-14s %-14s %-10s %-6s %-13s %-6s %-10s %-6s"
	)

	compact := m.compact || m.width < 122
	if compactLayout && m.width < 132 {
		compact = true
	}
	nodes := m.snapshot.Nodes
	totalNodes := len(nodes)

	alert, hasAlert := nodeStateAlert(m.snapshot)
	mandatoryLines := 2 // title + total
	if hasAlert {
		mandatoryLines++
	}
	remainingAfterMandatory := contentHeight - mandatoryLines
	showHeader := remainingAfterMandatory > 0
	visibleRows := 0
	if showHeader {
		visibleRows = min(totalNodes, remainingAfterMandatory-1)
	}
	hiddenNodes := totalNodes - visibleRows

	title := "node summary"
	if hiddenNodes > 0 {
		title = fmt.Sprintf("node summary (top %d/%d, +%d hidden)", visibleRows, totalNodes, hiddenNodes)
	}

	t := m.snapshot.Totals()
	lines := []string{m.sectionTitle(title)}
	if hasAlert {
		lines = append(lines, m.styles.bad.Render(alert))
	}

	if compact {
		if showHeader {
			lines = append(lines, fmt.Sprintf(compactRowFmt, "node", "part", "state", "cpu", "mem", "gpu"))
		}
		for i := 0; i < visibleRows; i++ {
			n := nodes[i]
			lines = append(lines, fmt.Sprintf(
				compactRowFmt,
				truncateRunes(n.Name, 14),
				truncateRunes(n.Partition, 9),
				truncateRunes(n.State, 10),
				uifmt.Ratio(n.CPUAlloc, n.CPUTotal),
				uifmt.MemPair(n.MemAllocMB, n.MemTotalMB),
				uifmt.Ratio(n.GPUAlloc, n.GPUTotal),
			))
		}
		totalLine := fmt.Sprintf(
			compactRowFmt,
			"TOTAL",
			"",
			"",
			uifmt.Ratio(t.CPUAlloc, t.CPUTotal),
			uifmt.MemPair(t.MemAllocMB, t.MemTotalMB),
			uifmt.Ratio(t.GPUAlloc, t.GPUTotal),
		)
		lines = append(lines, m.styles.accent.Render(totalLine))
		lines = clipLines(lines, contentHeight)
		lines = fitLinesToWidth(lines, contentWidth)
		return strings.Join(lines, "\n")
	}

	if showHeader {
		lines = append(lines, fmt.Sprintf(
			wideRowFmt,
			"node", "partition", "state", "cpu", "cpu%", "mem", "mem%", "gpu", "gpu%",
		))
	}
	for i := 0; i < visibleRows; i++ {
		n := nodes[i]
		lines = append(lines, fmt.Sprintf(
			wideRowFmt,
			truncateRunes(n.Name, 12),
			truncateRunes(n.Partition, 14),
			truncateRunes(n.State, 14),
			uifmt.Ratio(n.CPUAlloc, n.CPUTotal),
			uifmt.Percent(n.CPUUtil, n.HasCPU),
			uifmt.MemPair(n.MemAllocMB, n.MemTotalMB),
			uifmt.Percent(n.MemUtil, n.HasMem),
			uifmt.Ratio(n.GPUAlloc, n.GPUTotal),
			uifmt.Percent(n.GPUUtil, n.HasGPU),
		))
	}

	var cpuPct, memPct, gpuPct string
	if t.CPUTotal > 0 {
		cpuPct = fmt.Sprintf("%.1f%%", float64(t.CPUAlloc)/float64(t.CPUTotal)*100.0)
	} else {
		cpuPct = "n/a"
	}
	if t.MemTotalMB > 0 {
		memPct = fmt.Sprintf("%.1f%%", float64(t.MemAllocMB)/float64(t.MemTotalMB)*100.0)
	} else {
		memPct = "n/a"
	}
	if t.GPUTotal > 0 {
		gpuPct = fmt.Sprintf("%.1f%%", float64(t.GPUAlloc)/float64(t.GPUTotal)*100.0)
	} else {
		gpuPct = "n/a"
	}

	totalLine := fmt.Sprintf(
		wideRowFmt,
		"TOTAL",
		"",
		"",
		uifmt.Ratio(t.CPUAlloc, t.CPUTotal),
		cpuPct,
		uifmt.MemPair(t.MemAllocMB, t.MemTotalMB),
		memPct,
		uifmt.Ratio(t.GPUAlloc, t.GPUTotal),
		gpuPct,
	)
	lines = append(lines, m.styles.accent.Render(totalLine))
	lines = clipLines(lines, contentHeight)
	lines = fitLinesToWidth(lines, contentWidth)
	return strings.Join(lines, "\n")
}

func nodeStateAlert(snap *slurm.Snapshot) (string, bool) {
	if snap == nil || len(snap.Nodes) == 0 {
		return "", false
	}
	var down, drain int
	for _, n := range snap.Nodes {
		state := strings.ToUpper(n.State)
		if strings.Contains(state, "DOWN") {
			down++
		}
		if strings.Contains(state, "DRAIN") {
			drain++
		}
	}
	switch {
	case down > 0 && drain > 0:
		return fmt.Sprintf("node alert: down=%d drain=%d", down, drain), true
	case down > 0:
		return fmt.Sprintf("node alert: down=%d", down), true
	case drain > 0:
		return fmt.Sprintf("node alert: drain=%d", drain), true
	default:
		return "", false
	}
}

func (m Model) queueStatusLine(label string, value int) string {
	return m.styles.label.Render(fmt.Sprintf("%-8s", label)) + "  " + m.styles.value.Render(fmt.Sprintf("%5d", value))
}

func (m Model) sectionTitle(label string) string {
	icon := "•"
	switch {
	case strings.HasPrefix(label, "node summary"):
		icon = "◌"
	case strings.HasPrefix(label, "queue summary"):
		icon = "◍"
	case strings.HasPrefix(label, "user view"):
		icon = "◒"
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
