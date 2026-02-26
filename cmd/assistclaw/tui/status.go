package tui

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Public config passed in from statusCmd
// ─────────────────────────────────────────────

// StatusInfo holds the static config that doesn't change between refreshes.
type StatusInfo struct {
	PID           int
	Version       string
	SkillSummary  string
	Channels      []string
	PlanoEnabled  bool
	PlanoEndpoint string
	MCPEnabled    bool
	MCPTransport  string
}

// ─────────────────────────────────────────────
// Tea messages
// ─────────────────────────────────────────────

type tickMsg time.Time

type psResult struct {
	cpu   string
	rssKB int64
	vsz   string
	etime string
	alive bool
}

// ─────────────────────────────────────────────
// Model
// ─────────────────────────────────────────────

type StatusModel struct {
	info  StatusInfo
	ps    psResult
	width int
	err   string
}

func NewStatusModel(info StatusInfo) StatusModel {
	return StatusModel{info: info}
}

func (m StatusModel) Init() tea.Cmd {
	return tea.Batch(
		fetchPS(m.info.PID), // immediate first fetch
		tickEvery(),
	)
}

func (m StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "Q", "ctrl+c", "esc":
			return m, tea.Quit
		}

	case tickMsg:
		return m, tea.Batch(fetchPS(m.info.PID), tickEvery())

	case psResult:
		m.ps = msg
	}
	return m, nil
}

func (m StatusModel) View() string {
	bold := lipgloss.NewStyle().Bold(true).Foreground(ColorNeon)
	dim := lipgloss.NewStyle().Foreground(ColorMuted)
	prim := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	// ── Status indicator ──
	var statusLine string
	if m.ps.alive {
		statusLine = StatusOK + " " + prim.Render("RUNNING") +
			dim.Render(fmt.Sprintf("   PID %d   %s", m.info.PID, m.ps.etime))
	} else {
		statusLine = StatusErr + " " + lipgloss.NewStyle().Foreground(ColorError).Bold(true).Render("STOPPED")
	}

	// ── CPU & RAM bars ──
	cpuPct := 0.0
	fmt.Sscanf(m.ps.cpu, "%f", &cpuPct)
	ramMB := float64(m.ps.rssKB) / 1024.0
	ramPct := (ramMB / 1024.0) * 100.0
	if ramPct > 100 {
		ramPct = 100
	}

	cpuLine := dim.Render("CPU ") + ProgressBar(cpuPct, 14) + dim.Render(fmt.Sprintf("  %.1f%%", cpuPct))
	ramLine := dim.Render("RAM ") + ProgressBar(ramPct, 14) + dim.Render(fmt.Sprintf("  %.1f MB", ramMB))

	// ── Channels ──
	channelStr := dim.Render("none")
	if len(m.info.Channels) > 0 {
		colored := make([]string, len(m.info.Channels))
		for i, ch := range m.info.Channels {
			colored[i] = prim.Render(ch)
		}
		channelStr = strings.Join(colored, dim.Render(" · "))
	}

	// ── Plano ──
	planoStr := dim.Render("disabled")
	if m.info.PlanoEnabled {
		planoStr = prim.Render("✓ ") + dim.Render(m.info.PlanoEndpoint)
	}

	// ── MCP ──
	mcpStr := dim.Render("disabled")
	if m.info.MCPEnabled {
		t := m.info.MCPTransport
		if t == "" {
			t = "stdio"
		}
		mcpStr = prim.Render("✓ ") + dim.Render(t)
	}

	// ── Assemble body ──
	rows := []string{
		statusLine,
		"",
		dim.Render("  version     ") + prim.Render(m.info.Version),
		dim.Render("  skills      ") + prim.Render(m.info.SkillSummary),
		"",
		"  " + cpuLine,
		"  " + ramLine,
		"",
		dim.Render("  channels    ") + channelStr,
		dim.Render("  plano       ") + planoStr,
		dim.Render("  mcp         ") + mcpStr,
		"",
		dim.Render("  press q to quit"),
	}

	body := strings.Join(rows, "\n")

	return "\n" + lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Background(ColorSurface).
		Padding(1, 3).
		Render(bold.Render("  AssistClaw Status")+"\n\n"+body) + "\n"
}

// ─────────────────────────────────────────────
// Commands
// ─────────────────────────────────────────────

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchPS(pid int) tea.Cmd {
	return func() tea.Msg {
		r := psResult{cpu: "0.0", rssKB: 0, vsz: "0", etime: "--:--", alive: false}

		out, err := exec.Command("ps", "-p", fmt.Sprint(pid), "-o", "%cpu,rss,vsz,etime", "--no-headers").Output()
		if err != nil {
			// macOS retry without --no-headers
			out, _ = exec.Command("ps", "-p", fmt.Sprint(pid), "-o", "%cpu,rss,vsz,etime").Output()
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			// Skip header line that ps sometimes outputs on macOS
			if len(fields) >= 4 && fields[0] != "%CPU" {
				r.cpu = fields[0]
				if kb, e := strconv.ParseInt(fields[1], 10, 64); e == nil {
					r.rssKB = kb
				}
				r.vsz = fields[2]
				r.etime = fields[3]
				r.alive = true
				break
			}
		}
		return r
	}
}

// ─────────────────────────────────────────────
// Entry point
// ─────────────────────────────────────────────

// RunStatus launches the live-updating status dashboard.
// Exits when the user presses q, Esc, or Ctrl+C.
func RunStatus(info StatusInfo) error {
	p := tea.NewProgram(
		NewStatusModel(info),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}
