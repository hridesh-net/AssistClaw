//! Status Dashboard screen.

use crate::queue::EventQueue;
use crate::theme::Theme;
use crate::widgets::progress_bar::ProgressBar;
use color_eyre::Result;
use crate::terminal::TerminalGuard;
use crossterm::event::{self, Event, KeyCode, KeyEventKind, MouseEventKind};
use ratatui::{
    backend::CrosstermBackend,
    layout::{Alignment, Rect},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame, Terminal,
};
use std::io;
use std::time::{Duration, Instant};

/// Parsed status info from Go.
#[derive(Debug, Clone, Default)]
pub struct StatusInfo {
    pub pid: i32,
    pub version: String,
    pub skill_summary: String,
    pub channels: Vec<String>,
    pub plano_enabled: bool,
    pub plano_endpoint: String,
    pub mcp_enabled: bool,
    pub mcp_transport: String,
}

/// Live metrics updated by Go.
#[derive(Debug, Clone, Default)]
pub struct LiveMetrics {
    pub cpu_pct: f64,
    pub ram_mb: f64,
    pub ram_pct: f64,
    pub alive: bool,
    pub etime: String,
    /// Non-empty when Go could not read process stats. The dashboard
    /// shows this string in the status line instead of zeroed metrics.
    pub error: String,
}

/// Mutable UI state for the status dashboard.
pub struct StatusUIState {
    pub info: StatusInfo,
    pub metrics: LiveMetrics,
    pub width: u16,
    pub height: u16,
    pub pulse_frame: u64,
    pub hover_cpu: bool,
    pub hover_ram: bool,
}

impl StatusUIState {
    pub fn new(info: StatusInfo) -> Self {
        Self {
            info,
            metrics: LiveMetrics::default(),
            width: 80,
            height: 24,
            pulse_frame: 0,
            hover_cpu: false,
            hover_ram: false,
        }
    }
}

pub fn run(info_json: &str, _queue: EventQueue, enable_mouse: bool) -> Result<()> {
    let info: StatusInfo = parse_info(info_json);

    let _guard = TerminalGuard::enter(enable_mouse)?;
    let backend = CrosstermBackend::new(io::stdout());
    let mut terminal = Terminal::new(backend)?;

    let mut state = StatusUIState::new(info);
    let mut last_tick = Instant::now();
    let tick_rate = Duration::from_millis(500);

    let result = loop {
        // Pull latest metrics from app state (updated by Go via tui_status_update)
        if let Some(app) = crate::app::app() {
            let app_lock = app.lock();
            let st = app_lock.status.lock();
            state.metrics.cpu_pct = st.cpu_pct;
            state.metrics.ram_mb = st.ram_mb;
            state.metrics.ram_pct = st.ram_pct;
            state.metrics.alive = st.alive;
            state.metrics.etime.clone_from(&st.etime);
            state.metrics.error.clone_from(&st.error);
        }

        terminal.draw(|f| draw(f, &mut state))?;

        let timeout = tick_rate.saturating_sub(last_tick.elapsed());
        if crossterm::event::poll(timeout)? {
            match event::read()? {
                Event::Key(key) => {
                    if key.kind == KeyEventKind::Press {
                        match key.code {
                            KeyCode::Char('q') | KeyCode::Char('Q') | KeyCode::Esc => break Ok(()),
                            _ => {}
                        }
                    }
                }
                Event::Mouse(mouse) => {
                    match mouse.kind {
                        MouseEventKind::Moved => {
                            // Simple hover detection for progress bars
                            // This is approximate; full hit-testing would need layout rects
                            state.hover_cpu = false;
                            state.hover_ram = false;
                        }
                        _ => {}
                    }
                }
                Event::Resize(w, h) => {
                    state.width = w;
                    state.height = h;
                }
                _ => {}
            }
        }

        if last_tick.elapsed() >= tick_rate {
            state.pulse_frame = state.pulse_frame.wrapping_add(1);
            last_tick = Instant::now();
        }
    };

    // TerminalGuard restores raw mode + alt screen + mouse on drop.
    let _ = terminal.show_cursor();
    result
}

fn parse_info(json: &str) -> StatusInfo {
    let mut info = StatusInfo::default();
    if let Ok(val) = serde_json::from_str::<serde_json::Value>(json) {
        info.pid = val["pid"].as_i64().unwrap_or(0) as i32;
        info.version = val["version"].as_str().unwrap_or("").to_string();
        info.skill_summary = val["skill_summary"].as_str().unwrap_or("").to_string();
        if let Some(arr) = val["channels"].as_array() {
            info.channels = arr
                .iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect();
        }
        info.plano_enabled = val["plano_enabled"].as_bool().unwrap_or(false);
        info.plano_endpoint = val["plano_endpoint"].as_str().unwrap_or("").to_string();
        info.mcp_enabled = val["mcp_enabled"].as_bool().unwrap_or(false);
        info.mcp_transport = val["mcp_transport"].as_str().unwrap_or("").to_string();
    }
    info
}

fn draw(f: &mut Frame, state: &mut StatusUIState) {
    let theme = Theme::default();
    let area = f.area();
    state.width = area.width;
    state.height = area.height;

    let pulse_color = if state.pulse_frame % 2 == 0 {
        theme.palette.primary
    } else {
        theme.palette.neon
    };

    // Centered status panel
    let panel_width = 60_u16.min(state.width.saturating_sub(4)).max(40);
    let panel_height = 18_u16.min(state.height.saturating_sub(4)).max(12);
    let panel_x = (state.width.saturating_sub(panel_width)) / 2;
    let panel_y = (state.height.saturating_sub(panel_height)) / 2;

    let panel_rect = Rect {
        x: panel_x,
        y: panel_y,
        width: panel_width,
        height: panel_height,
    };

    let block = Block::default()
        .title(" AssistClaw Status ")
        .title_alignment(Alignment::Center)
        .borders(Borders::ALL)
        .border_type(ratatui::widgets::BorderType::Rounded)
        .border_style(Style::default().fg(pulse_color))
        .style(Style::default().bg(theme.palette.surface));

    let inner = block.inner(panel_rect);
    f.render_widget(block, panel_rect);

    // Build content lines
    let mut lines: Vec<Line> = Vec::new();

    // Status indicator — if Go reported an error, surface it instead of
    // a misleading "0% CPU / 0 MB RAM" row.
    let status_line = if !state.metrics.error.is_empty() {
        Line::from(vec![
            Span::styled("● ", theme.status_err()),
            Span::styled(
                "STATS UNAVAILABLE",
                Style::default()
                    .fg(theme.palette.error)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::styled(
                format!("   {}", state.metrics.error),
                Style::default().fg(theme.palette.muted),
            ),
        ])
    } else if state.metrics.alive {
        Line::from(vec![
            Span::styled("● ", theme.status_ok()),
            Span::styled(
                "RUNNING",
                Style::default()
                    .fg(theme.palette.primary)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::styled(
                format!("   PID {}   {}", state.info.pid, state.metrics.etime),
                Style::default().fg(theme.palette.muted),
            ),
        ])
    } else {
        Line::from(vec![
            Span::styled("● ", theme.status_err()),
            Span::styled(
                "STOPPED",
                Style::default()
                    .fg(theme.palette.error)
                    .add_modifier(Modifier::BOLD),
            ),
        ])
    };
    lines.push(status_line);
    lines.push(Line::from(""));

    // Version and skills
    lines.push(Line::from(vec![
        Span::styled("  version     ", Style::default().fg(theme.palette.muted)),
        Span::styled(&state.info.version, Style::default().fg(theme.palette.primary).add_modifier(Modifier::BOLD)),
    ]));
    lines.push(Line::from(vec![
        Span::styled("  skills      ", Style::default().fg(theme.palette.muted)),
        Span::styled(&state.info.skill_summary, Style::default().fg(theme.palette.primary).add_modifier(Modifier::BOLD)),
    ]));
    lines.push(Line::from(""));

    // CPU bar
    let _cpu_label = if state.hover_cpu {
        format!("  CPU  {:.1}%", state.metrics.cpu_pct)
    } else {
        format!("  CPU  {:.1}%", state.metrics.cpu_pct)
    };
    let cpu_bar = ProgressBar::new(state.metrics.cpu_pct, 14).render(&theme);
    lines.push(Line::from(vec![
        Span::styled("  CPU ", Style::default().fg(theme.palette.muted)),
        Span::styled(cpu_bar, Style::default().fg(theme.palette.primary)),
        Span::styled(format!("  {:.1}%", state.metrics.cpu_pct), Style::default().fg(theme.palette.muted)),
    ]));

    // RAM bar
    let ram_bar = ProgressBar::new(state.metrics.ram_pct, 14).render(&theme);
    lines.push(Line::from(vec![
        Span::styled("  RAM ", Style::default().fg(theme.palette.muted)),
        Span::styled(ram_bar, Style::default().fg(theme.palette.primary)),
        Span::styled(format!("  {:.1} MB", state.metrics.ram_mb), Style::default().fg(theme.palette.muted)),
    ]));
    lines.push(Line::from(""));

    // Channels
    let channel_str = if state.info.channels.is_empty() {
        Span::styled("none", Style::default().fg(theme.palette.muted))
    } else {
        let mut spans = Vec::new();
        for (i, ch) in state.info.channels.iter().enumerate() {
            if i > 0 {
                spans.push(Span::styled(" · ", Style::default().fg(theme.palette.muted)));
            }
            spans.push(Span::styled(ch.as_str(), Style::default().fg(theme.palette.primary).add_modifier(Modifier::BOLD)));
        }
        // Return as a combined text since Line::from doesn't take nested spans well for this case
        // Instead we'll just render a single text line
        Span::styled(state.info.channels.join(" · "), Style::default().fg(theme.palette.primary).add_modifier(Modifier::BOLD))
    };
    lines.push(Line::from(vec![
        Span::styled("  channels    ", Style::default().fg(theme.palette.muted)),
        channel_str,
    ]));

    // Plano
    let plano_str = if state.info.plano_enabled {
        Line::from(vec![
            Span::styled("  plano       ", Style::default().fg(theme.palette.muted)),
            Span::styled("✓ ", Style::default().fg(theme.palette.primary).add_modifier(Modifier::BOLD)),
            Span::styled(&state.info.plano_endpoint, Style::default().fg(theme.palette.muted)),
        ])
    } else {
        Line::from(vec![
            Span::styled("  plano       ", Style::default().fg(theme.palette.muted)),
            Span::styled("disabled", Style::default().fg(theme.palette.muted)),
        ])
    };
    lines.push(plano_str);

    // MCP
    let mcp_str = if state.info.mcp_enabled {
        let transport = if state.info.mcp_transport.is_empty() {
            "stdio"
        } else {
            &state.info.mcp_transport
        };
        Line::from(vec![
            Span::styled("  mcp         ", Style::default().fg(theme.palette.muted)),
            Span::styled("✓ ", Style::default().fg(theme.palette.primary).add_modifier(Modifier::BOLD)),
            Span::styled(transport, Style::default().fg(theme.palette.muted)),
        ])
    } else {
        Line::from(vec![
            Span::styled("  mcp         ", Style::default().fg(theme.palette.muted)),
            Span::styled("disabled", Style::default().fg(theme.palette.muted)),
        ])
    };
    lines.push(mcp_str);
    lines.push(Line::from(""));

    // Quit hint
    lines.push(Line::from(
        Span::styled("  press q to quit", Style::default().fg(theme.palette.muted)),
    ));

    let paragraph = Paragraph::new(lines);
    f.render_widget(paragraph, inner);
}
