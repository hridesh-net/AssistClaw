package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Tea Messages
// ─────────────────────────────────────────────

type agentTokenMsg string
type agentToolMsg string
type agentDoneMsg struct{ iterations, tokens int }
type agentErrMsg error

// pulseMsg drives a slow border / header pulse independent of the spinner.
type pulseMsg struct{}

// ─────────────────────────────────────────────
// Agent interface (matches agent.StreamHandler + agent.Runner)
// ─────────────────────────────────────────────

// RunResult mirrors agent.RunResult for independence from agent package.
type RunResult struct {
	Iterations int
	Usage      struct{ TotalTokens int }
}

// AgentStreamHandler is the interface the REPL bridge must satisfy.
// It matches agent.StreamHandler exactly.
type AgentStreamHandler interface {
	OnToken(token string)
	OnToolCall(name string, input json.RawMessage)
	OnDone(result *RunResult)
}

// AgentRunner is the minimal surface area of agent.Runner needed by the REPL.
type AgentRunner interface {
	SessionID() string
	Run(ctx context.Context, userMessage string) (*RunResult, error)
}

// ─────────────────────────────────────────────
// REPL Model
// ─────────────────────────────────────────────

// REPLModel is the bubbletea model for the interactive REPL.
type REPLModel struct {
	ctx    context.Context
	cancel context.CancelFunc
	runner AgentRunner

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	history     []string
	tokenBuf    string
	currentTool string
	thinking    bool
	width       int
	height      int
	ready       bool
	pulseFrame  int
	sessionID   string
	version     string

	// sendMsg is set by RunREPL so the Update loop can call it.
	sendMsg func(line string)
}

// NewREPLModel creates a configured REPL model. sendMsg is called when the
// user presses Enter; it must dispatch the message to the agent asynchronously.
func NewREPLModel(ctx context.Context, runner AgentRunner, sessionID, ver string, sendMsg func(string)) *REPLModel {
	ctx, cancel := context.WithCancel(ctx)

	ta := textarea.New()
	ta.Placeholder = "Message AssistClaw..."
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(2)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("ctrl+j") // Ctrl+J = newline; Enter = send
	ta.CharLimit = 4096

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)

	return &REPLModel{
		ctx:       ctx,
		cancel:    cancel,
		runner:    runner,
		textarea:  ta,
		spinner:   sp,
		sendMsg:   sendMsg,
		sessionID: sessionID,
		version:   ver,
	}
}

func pulseCmd() tea.Cmd {
	return tea.Tick(480*time.Millisecond, func(time.Time) tea.Msg {
		return pulseMsg{}
	})
}

func (m *REPLModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick, pulseCmd())
}

func (m *REPLModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpH := m.height - 10 // header row + chat + input chrome
		if vpH < 4 {
			vpH = 4
		}
		if !m.ready {
			m.viewport = viewport.New(m.width-4, vpH)
			m.ready = true
		} else {
			m.viewport.Width = m.width - 4
			m.viewport.Height = vpH
		}
		m.textarea.SetWidth(m.width - 8)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancel()
			return m, tea.Quit

		case tea.KeyEnter:
			if !m.thinking {
				line := strings.TrimSpace(m.textarea.Value())
				if line != "" {
					m.appendHistory(UserPrefix.Render("You ›") + " " + line)
					m.textarea.Reset()
					m.thinking = true
					m.tokenBuf = ""
					m.currentTool = ""
					m.refreshViewport()
					if m.sendMsg != nil {
						m.sendMsg(line)
					}
				}
			}
		}

	case agentTokenMsg:
		m.tokenBuf += string(msg)
		m.refreshViewport()

	case agentToolMsg:
		if m.tokenBuf != "" {
			m.flushToken()
		}
		m.currentTool = string(msg)
		m.refreshViewport()

	case agentDoneMsg:
		m.flushToken()
		m.currentTool = ""
		m.thinking = false
		m.appendHistory(Muted.Render(fmt.Sprintf(
			"   ▸ %d iter · %s tokens", msg.iterations, fmtNum(msg.tokens),
		)))
		m.appendHistory("")
		m.refreshViewport()

	case agentErrMsg:
		m.flushToken()
		m.currentTool = ""
		m.thinking = false
		m.appendHistory(ErrorStyle.Render("✗ ") + Muted.Render(msg.Error()))
		m.refreshViewport()

	case pulseMsg:
		m.pulseFrame++
		cmds = append(cmds, pulseCmd())

	case spinner.TickMsg:
		var sc tea.Cmd
		m.spinner, sc = m.spinner.Update(msg)
		cmds = append(cmds, sc)
	}

	if !m.thinking {
		var tc tea.Cmd
		m.textarea, tc = m.textarea.Update(msg)
		cmds = append(cmds, tc)
	}

	var vc tea.Cmd
	m.viewport, vc = m.viewport.Update(msg)
	cmds = append(cmds, vc)

	return m, tea.Batch(cmds...)
}

func (m *REPLModel) View() string {
	if !m.ready {
		return "\n  " + Primary.Render("Starting AssistClaw...") + "\n"
	}

	// Viewport (chat history)
	m.viewport.SetContent(m.buildContent())
	borderCol := PulseBorder(m.pulseFrame)
	chatBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderCol).
		Width(m.width - 2).
		Render(m.viewport.View())

	pulseMark := lipgloss.NewStyle().Foreground(borderCol).Bold(true).Render("▶")
	headerRow := lipgloss.JoinHorizontal(
		lipgloss.Left,
		pulseMark,
		lipgloss.NewStyle().Render(" "),
		GradientWord("ASSISTCLAW"),
		lipgloss.NewStyle().Render(" "),
		CyberBracket("REPL"),
		Muted.Render("  "+strings.TrimSpace(m.version)+"  ·  session "+shortID(m.sessionID)),
	)
	headerBar := lipgloss.NewStyle().
		Width(m.width-2).
		Padding(0, 0, 0, 0).
		Render(headerRow)
	under := lipgloss.NewStyle().Foreground(ColorBorder).Width(m.width - 2).Render(ScanlineSuffix(minReplScanlineWidth(m.width)))

	// Status footer
	var footer string
	if m.thinking {
		tool := ""
		if m.currentTool != "" {
			tool = "  " + ToolBadge.Render("⚡ "+m.currentTool)
		}
		footer = m.spinner.View() + Neon.Render(" · ") + Muted.Render("thinking") + tool
	} else {
		footer = Muted.Render("↵ send  ·  Ctrl+J newline  ·  ↑↓ scroll  ·  ESC quit")
	}

	// Input box
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(ColorBorder).
		Width(m.width-2).
		Padding(0, 1).
		Render(Primary.Render("›") + " " + m.textarea.View() + "\n  " + footer)

	return lipgloss.JoinVertical(lipgloss.Left, headerBar, under, chatBox, inputBox)
}

func minReplScanlineWidth(termW int) int {
	w := termW - 6
	if w < 24 {
		w = 24
	}
	if w > 72 {
		w = 72
	}
	return w
}

func (m *REPLModel) buildContent() string {
	var sb strings.Builder
	for _, l := range m.history {
		sb.WriteString(l + "\n")
	}
	if m.tokenBuf != "" {
		sb.WriteString(AgentPrefix.Render("🤖") + " " + m.tokenBuf)
	}
	if m.currentTool != "" {
		sb.WriteString("\n" + ToolBadge.Render("  ⚡ "+m.currentTool) + " " + m.spinner.View())
	}
	return sb.String()
}

func (m *REPLModel) appendHistory(line string) { m.history = append(m.history, line) }
func (m *REPLModel) flushToken() {
	if m.tokenBuf != "" {
		m.appendHistory(AgentPrefix.Render("🤖") + " " + m.tokenBuf)
		m.tokenBuf = ""
	}
}
func (m *REPLModel) refreshViewport() {
	m.viewport.SetContent(m.buildContent())
	m.viewport.GotoBottom()
}

// fmtNum inserts commas into an integer string.
func fmtNum(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// ─────────────────────────────────────────────
// RunREPL — entry point called from main.go
// ─────────────────────────────────────────────

// RunREPL starts the futuristic bubbletea REPL.
// It bridges agent streaming events into the TUI via p.Send().
func RunREPL(ctx context.Context, runner AgentRunner, ver string, providerCount, skillCount int) error {
	// Print banner before entering alt-screen
	fmt.Print(RenderBanner(ver, runner.SessionID(), providerCount, skillCount))
	fmt.Println()

	var p *tea.Program

	sendMsg := func(line string) {
		go func() {
			bridge := &tuiStreamBridge{prog: p}
			result, err := runner.Run(ctx, line)
			if err != nil {
				p.Send(agentErrMsg(err))
				return
			}
			if result != nil {
				p.Send(agentDoneMsg{
					iterations: result.Iterations,
					tokens:     result.Usage.TotalTokens,
				})
			}
			_ = bridge // satisfies compiler; bridge not used in non-streaming fallback
		}()
	}

	model := NewREPLModel(ctx, runner, runner.SessionID(), ver, sendMsg)
	p = tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithContext(ctx),
	)
	_, err := p.Run()
	return err
}

// tuiStreamBridge forwards agent events to the tea.Program via p.Send().
// Kept for future streaming integration — currently Run() is used fallback.
type tuiStreamBridge struct {
	prog *tea.Program
}

func (b *tuiStreamBridge) OnToken(token string)                      { b.prog.Send(agentTokenMsg(token)) }
func (b *tuiStreamBridge) OnToolCall(name string, _ json.RawMessage) { b.prog.Send(agentToolMsg(name)) }
func (b *tuiStreamBridge) OnDone(r *RunResult) {
	b.prog.Send(agentDoneMsg{iterations: r.Iterations, tokens: r.Usage.TotalTokens})
}
