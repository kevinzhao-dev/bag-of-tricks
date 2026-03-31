package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kvzhao/netwatch/internal/collector"
	"github.com/kvzhao/netwatch/internal/storage"
	"github.com/kvzhao/netwatch/internal/tracker"
)

const version = "0.1.0"

// tickMsg triggers a data collection cycle.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Styles
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236"))

	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	orangeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	dlStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	ulStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	sepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// Model is the bubbletea model for the dashboard.
type Model struct {
	collector        *collector.Collector
	tracker          *tracker.Tracker
	store            *storage.Store
	iface            string
	width            int
	lastDelta        *collector.Delta
	lastAbsRx        uint64 // last seen absolute OS counter (for checkpoint)
	lastAbsTx        uint64
	consecutiveDays  int
	consecutiveDates []string
	tickCount        int
	err              error
	quitting         bool
}

// NewModel creates the dashboard model.
func NewModel(iface string, thresholdGB float64) Model {
	store, _ := storage.New()
	coll := collector.New(iface)
	trk := tracker.New(thresholdGB)

	// Load consecutive days info
	var days int
	var dates []string
	if store != nil {
		limitBytes := uint64(thresholdGB * float64(tracker.GB))
		days, dates, _ = store.ConsecutiveDaysOver(limitBytes)
	}

	m := Model{
		collector:        coll,
		tracker:          trk,
		store:            store,
		iface:            iface,
		width:            80,
		consecutiveDays:  days,
		consecutiveDates: dates,
	}

	// Restore from checkpoint: recover traffic that happened while program was off
	m.restoreFromCheckpoint()

	return m
}

// restoreFromCheckpoint loads the saved checkpoint and reconciles with current
// OS counters to recover traffic accumulated while the program wasn't running.
func (m *Model) restoreFromCheckpoint() {
	if m.store == nil {
		return
	}
	cp, err := m.store.LoadCheckpoint()
	if err != nil || cp == nil {
		return
	}

	today := time.Now().Format("2006-01-02")

	// Take a snapshot of current absolute counters
	_, stats, err := m.collector.Collect()
	if err != nil || stats == nil {
		return
	}

	if cp.Date != today {
		// Different day: save the checkpoint data as yesterday's final record,
		// then start fresh. We can't recover cross-day traffic accurately.
		m.store.Save(storage.DayRecord{
			Date:    cp.Date,
			RxBytes: cp.RxAccum,
			TxBytes: cp.TxAccum,
		})
		return
	}

	// Same day: restore accumulated total + add traffic from downtime
	rxAccum := cp.RxAccum
	txAccum := cp.TxAccum

	// If OS counters advanced (no reboot), add the diff
	if stats.RxBytes >= cp.RxAbs {
		rxAccum += stats.RxBytes - cp.RxAbs
	}
	if stats.TxBytes >= cp.TxAbs {
		txAccum += stats.TxBytes - cp.TxAbs
	}
	// If counter reset (reboot), we just use the saved accumulated value
	// — we can't know how much traffic happened between reboot and now

	m.tracker.Seed(rxAccum, txAccum, time.Now())
}

func (m Model) Init() tea.Cmd {
	return tickCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			m.saveToday()
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tickMsg:
		now := time.Time(msg)
		delta, stats, err := m.collector.Collect()
		if err != nil {
			m.err = err
			return m, tickCmd()
		}
		m.err = nil

		// Track absolute counters for checkpoint
		if stats != nil {
			m.lastAbsRx = stats.RxBytes
			m.lastAbsTx = stats.TxBytes
		}

		if delta != nil {
			dayRolled := m.tracker.Add(delta.RxBytes, delta.TxBytes, now)
			if dayRolled {
				m.saveToday()
			}
			m.lastDelta = delta
		}

		// Save checkpoint every 30 seconds
		m.tickCount++
		if m.tickCount%30 == 0 {
			m.saveCheckpoint()
		}

		return m, tickCmd()
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return "Bye!\n"
	}

	w := max(m.width, 40)

	var b strings.Builder

	// ── Header bar (full width) ──
	title := fmt.Sprintf(" netwatch v%s", version)
	dateStr := m.tracker.Date.Format("2006-01-02")
	right := fmt.Sprintf("%s  iface: %s ", dateStr, m.iface)
	pad := max(w-len(title)-len(right), 1)
	b.WriteString(headerStyle.Render(title + strings.Repeat(" ", pad) + right))
	b.WriteString("\n")

	// ── Error ──
	if m.err != nil {
		b.WriteString(redStyle.Render(fmt.Sprintf(" Error: %v", m.err)) + "\n")
	}

	// ── Separator ──
	b.WriteString(m.separator("Speed"))

	// ── Speed section ──
	var dlSpeed, ulSpeed string
	if m.lastDelta != nil {
		dlSpeed = formatSpeed(m.lastDelta.RxRate())
		ulSpeed = formatSpeed(m.lastDelta.TxRate())
	} else {
		dlSpeed = "---"
		ulSpeed = "---"
	}
	b.WriteString(m.meterLine("Download", dlSpeed, dlStyle))
	b.WriteString(m.meterLine("Upload  ", ulSpeed, ulStyle))

	// ── Separator ──
	b.WriteString(m.separator("Daily Usage"))

	// ── Today's accumulation ──
	rxBar := m.usageBar("RX", m.tracker.RxGB(), m.tracker.LimitGB(), dlStyle)
	txBar := m.usageBar("TX", m.tracker.TxGB(), m.tracker.LimitGB(), ulStyle)
	b.WriteString(rxBar)
	b.WriteString(txBar)
	b.WriteString("\n")

	// Total line
	totalStr := fmt.Sprintf("%.2f GB", m.tracker.TotalGB())
	limitStr := fmt.Sprintf("%.0f GB", m.tracker.LimitGB())
	b.WriteString(fmt.Sprintf(" Total: %s / %s\n",
		levelStyle(m.tracker.CheckLevel()).Render(totalStr),
		dimStyle.Render(limitStr),
	))

	// ── Threshold progress bar (wide) ──
	b.WriteString(m.separator("Threshold"))
	b.WriteString(m.thresholdBar())

	// ── Warnings ──
	if m.consecutiveDays > 0 {
		b.WriteString(m.separator("Warnings"))
		dateList := ""
		if len(m.consecutiveDates) > 0 {
			dateList = " (" + strings.Join(m.consecutiveDates, ", ") + ")"
		}
		b.WriteString(orangeStyle.Render(
			fmt.Sprintf(" Consecutive days over limit: %d%s",
				m.consecutiveDays, dateList)) + "\n")
		remaining := 3 - m.consecutiveDays
		if remaining > 0 {
			b.WriteString(yellowStyle.Render(
				fmt.Sprintf(" Throttle risk: %d more day(s) to trigger", remaining)) + "\n")
		} else {
			b.WriteString(redStyle.Render(" Throttle triggered!") + "\n")
		}
	}

	// ── Footer ──
	b.WriteString("\n")
	footerLeft := " q: Quit"
	footerRight := "checkpoint: every 30s "
	fpad := max(w-len(footerLeft)-len(footerRight), 1)
	b.WriteString(footerStyle.Render(footerLeft + strings.Repeat(" ", fpad) + footerRight))
	b.WriteString("\n")

	return b.String()
}

// separator renders a section divider like ── Title ────────
func (m Model) separator(title string) string {
	w := m.width
	label := fmt.Sprintf("── %s ", title)
	rest := max(w-len(label)-1, 2)
	return "\n" + sepStyle.Render(label+strings.Repeat("─", rest)) + "\n"
}

// meterLine renders a labeled speed value:  " Download    12.3 MB/s"
func (m Model) meterLine(label, value string, style lipgloss.Style) string {
	return fmt.Sprintf(" %-10s %s\n", dimStyle.Render(label), style.Render(value))
}

// usageBar renders a horizontal bar like:  " RX  [████████░░░░░░░░░░░░]  12.34 GB"
func (m Model) usageBar(label string, usedGB, limitGB float64, style lipgloss.Style) string {
	barW := max(m.width-28, 10) // room for label + value text
	frac := 0.0
	if limitGB > 0 {
		frac = usedGB / limitGB
	}
	frac = min(frac, 1.0)

	filled := int(frac * float64(barW))
	empty := barW - filled

	bar := style.Render(strings.Repeat("│", filled)) + dimStyle.Render(strings.Repeat("░", empty))
	return fmt.Sprintf(" %-4s [%s] %8.2f GB\n", dimStyle.Render(label), bar, usedGB)
}

// thresholdBar renders the main wide progress bar with threshold markers.
func (m Model) thresholdBar() string {
	barW := max(m.width-22, 20) // room for percentage text

	progress := min(m.tracker.Progress(), 1.0)
	filled := int(progress * float64(barW))
	empty := barW - filled

	level := m.tracker.CheckLevel()
	bar := levelStyle(level).Render(strings.Repeat("█", filled)) +
		dimStyle.Render(strings.Repeat("░", empty))

	pct := int(m.tracker.Progress() * 100)

	var status string
	switch level {
	case tracker.LevelCritical:
		status = redStyle.Render("OVER LIMIT")
	case tracker.LevelDanger:
		status = orangeStyle.Render("DANGER")
	case tracker.LevelWarning:
		status = yellowStyle.Render("WARNING")
	default:
		status = greenStyle.Render("OK")
	}

	return fmt.Sprintf(" [%s] %3d%%  %s\n", bar, pct, status)
}

func (m Model) saveToday() {
	if m.store == nil {
		return
	}
	rec := storage.DayRecord{
		Date:    m.tracker.Date.Format("2006-01-02"),
		RxBytes: m.tracker.RxTotal,
		TxBytes: m.tracker.TxTotal,
	}
	m.store.Save(rec)
	m.saveCheckpoint()
}

func (m Model) saveCheckpoint() {
	if m.store == nil {
		return
	}
	cp := storage.Checkpoint{
		Date:    m.tracker.Date.Format("2006-01-02"),
		RxAbs:   m.lastAbsRx,
		TxAbs:   m.lastAbsTx,
		RxAccum: m.tracker.RxTotal,
		TxAccum: m.tracker.TxTotal,
	}
	m.store.SaveCheckpoint(cp)
}

func formatSpeed(bytesPerSec float64) string {
	mb := bytesPerSec / (1024 * 1024)
	if mb >= 1.0 {
		return fmt.Sprintf("%.1f MB/s", mb)
	}
	kb := bytesPerSec / 1024
	return fmt.Sprintf("%.0f KB/s", kb)
}

func levelStyle(level tracker.ThresholdLevel) lipgloss.Style {
	switch level {
	case tracker.LevelWarning:
		return yellowStyle
	case tracker.LevelDanger:
		return orangeStyle
	case tracker.LevelCritical:
		return redStyle
	default:
		return greenStyle
	}
}
