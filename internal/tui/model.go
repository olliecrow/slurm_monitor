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
	if m.width <= 0 || m.height <= 0 {
		return "initializing..."
	}

	now := m.now
	if now.IsZero() {
		now = time.Now()
	}

	header := m.renderHeader(now)
	footer := m.styles.dim.Render("Ctrl+C to exit")
	headerLines := lineCount(header)
	footerLines := lineCount(footer)
	gapLines := 2
	if m.height <= 24 {
		gapLines = 1
	}
	bodyHeight := m.height - headerLines - footerLines - gapLines
	if bodyHeight < 3 {
		bodyHeight = 3
	}

	var body string
	if m.snapshot == nil {
		body = m.styles.panel.Width(max(20, m.width-6)).Render("waiting for first successful snapshot...")
		body = clipToHeight(body, bodyHeight)
	} else {
		body = m.renderMain(bodyHeight)
	}

	parts := []string{header}
	if m.height > 24 {
		parts = append(parts, "", body, "", footer)
	} else {
		parts = append(parts, body, "", footer)
	}
	joined := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return clipToViewport(joined, m.width, m.height)
}

func (m Model) renderHeader(now time.Time) string {
	statusText, _, statusChip := m.renderStatusText(now)
	pulse := pulseFrames[m.pulseIndex%len(pulseFrames)]
	statusText = pulse + " " + statusText
	ageText := "last update: never"
	if !m.lastSuccess.IsZero() {
		ageText = "last update: " + humanDuration(now.Sub(m.lastSuccess)) + " ago"
	}

	left := m.styles.title.Render(" SLURM MONITOR ") + "  " + m.styles.label.Render("source: ") + m.styles.value.Render(m.source)
	right := statusChip.Render(statusText)
	line1 := joinWithPadding(left, right, m.width)

	line2 := m.styles.chip.Render("clock: " + now.Format("15:04:05"))
	line2 += " " + m.styles.chip.Render(ageText)
	if m.refresh > 0 {
		line2 += " " + m.styles.chip.Render("refresh "+m.refresh.String())
	}
	if m.lastError != "" {
		line2 += "  " + m.styles.errorLabel.Render("error: "+m.lastError)
	}
	line2 = truncateRunes(line2, m.width)
	return line1 + "\n" + line2
}

func (m Model) renderStatusText(now time.Time) (string, lipgloss.Style, lipgloss.Style) {
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
	compactLayout := m.compact || m.width < 118 || maxHeight < 18

	queueTarget := max(9, maxHeight/2)
	nodeTarget := maxHeight - queueTarget - 1
	if nodeTarget < 6 {
		nodeTarget = 6
		queueTarget = maxHeight - nodeTarget - 1
	}
	if queueTarget < 6 {
		queueTarget = 6
		nodeTarget = max(3, maxHeight-queueTarget-1)
	}

	showDemandCols := m.width >= 70
	queueBodyHeight := max(1, queueTarget-2)
	userRows := max(1, queueBodyHeight-8)
	queueBody := m.renderQueuePanel(userRows, showDemandCols)
	queueBody = clipToHeight(queueBody, queueBodyHeight)
	queuePanel := m.styles.panel.Width(inner).Render(queueBody)

	nodeLimit := max(1, nodeTarget-5)
	if compactLayout && maxHeight <= 28 && nodeLimit > 6 {
		nodeLimit = 6
	}
	nodePanel := m.styles.panel.Width(inner).Render(m.renderNodeTable(nodeLimit))
	nodePanel = clipToHeight(nodePanel, nodeTarget)

	body := lipgloss.JoinVertical(lipgloss.Left, nodePanel, "", queuePanel)
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
	if len(users) > limit {
		users = users[:limit]
	}

	lines := []string{m.sectionTitle("user view")}
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
	return lines
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
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}
	t := m.snapshot.Totals()

	compact := m.compact || m.width < 122
	lines := []string{m.sectionTitle("node summary")}
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
	switch label {
	case "node summary":
		icon = "◌"
	case "queue summary":
		icon = "◍"
	case "user view":
		icon = "◒"
	}
	return m.styles.tableHdr.Render(icon + " " + label)
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

func joinWithPadding(left, right string, width int) string {
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	padding := width - leftWidth - rightWidth
	if padding < 1 {
		return truncateRunes(left+" "+right, width)
	}
	return left + strings.Repeat(" ", padding) + right
}

func clipToViewport(s string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
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

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
