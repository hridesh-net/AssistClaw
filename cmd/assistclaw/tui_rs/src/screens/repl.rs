//! REPL Chat screen — the main interactive chat interface.

use crate::event::TuiEvent;
use crate::queue::EventQueue;
use crate::terminal::TerminalGuard;
use crate::theme::Theme;
use color_eyre::Result;
use crossterm::event::{self, Event, KeyCode, KeyEventKind, MouseEventKind};
use ratatui::{
    backend::CrosstermBackend,
    layout::{Constraint, Direction, Layout, Margin, Rect},
    style::{Modifier, Style},
    text::{Line, Span, Text},
    widgets::{Block, Borders, Paragraph, Scrollbar, ScrollbarOrientation, Wrap},
    Frame, Terminal,
};
use std::io;
use std::time::{Duration, Instant};

/// Run the REPL TUI event loop.
///
/// `enable_mouse` controls whether mouse capture is enabled. Pass `false`
/// from Go when the user is inside tmux/screen, or when `--no-mouse` is set.
pub fn run(queue: EventQueue, enable_mouse: bool) -> Result<()> {
    let _guard = TerminalGuard::enter(enable_mouse)?;

    let backend = CrosstermBackend::new(io::stdout());
    let mut terminal = Terminal::new(backend)?;

    let mut state = ReplUIState::new(queue.clone());

    // The FFI layer (tui_repl_send_token/done/error) writes into the shared
    // AppState.repl; this loop owns the render state. Capture the handle once
    // and sync shared → render every frame so agent output actually appears.
    let app_handle = crate::app::app();
    if let Some(a) = &app_handle {
        let app = a.lock();
        state.version = app.version.clone();
        state.session_id = app.session_id.clone();
    }

    let mut last_tick = Instant::now();
    let tick_rate = Duration::from_millis(60); // ~16 FPS

    let result = loop {
        if let Some(a) = &app_handle {
            let app = a.lock();
            let repl = app.repl.lock();
            if repl.history.len() != state.history.len() {
                state.history = repl.history.clone();
                state.scroll_to_bottom = true;
            }
            if state.token_buffer != repl.token_buffer {
                state.token_buffer = repl.token_buffer.clone();
                state.scroll_to_bottom = true;
            }
            state.current_tool = repl.current_tool.clone();
            state.thinking = repl.thinking;
        }

        terminal.draw(|f| draw(f, &mut state))?;

        let timeout = tick_rate.saturating_sub(last_tick.elapsed());
        if crossterm::event::poll(timeout)? {
            match event::read()? {
                Event::Key(key) => {
                    if key.kind == KeyEventKind::Press {
                        match key.code {
                            KeyCode::Char('c')
                                if key.modifiers.contains(event::KeyModifiers::CONTROL) =>
                            {
                                if state.thinking {
                                    // Stop the in-flight agent run; stay in the REPL.
                                    queue.push(TuiEvent::Interrupt);
                                    if let Some(a) = &app_handle {
                                        let app = a.lock();
                                        let mut repl = app.repl.lock();
                                        repl.thinking = false;
                                        repl.current_tool.clear();
                                    }
                                    state.thinking = false;
                                } else {
                                    queue.push(TuiEvent::Quit);
                                    break Ok(());
                                }
                            }
                            KeyCode::Esc => {
                                queue.push(TuiEvent::Quit);
                                break Ok(());
                            }
                            KeyCode::Enter => {
                                let msg = state.input_text.clone();
                                if !msg.trim().is_empty() && !state.thinking {
                                    // Write to the shared state — the frame
                                    // sync above mirrors it into render state.
                                    if let Some(a) = &app_handle {
                                        let app = a.lock();
                                        let mut repl = app.repl.lock();
                                        repl.history.push(("user".to_string(), msg.clone()));
                                        repl.thinking = true;
                                    } else {
                                        state.history.push(("user".to_string(), msg.clone()));
                                    }
                                    state.input_text.clear();
                                    state.cursor_position = 0;
                                    state.thinking = true;
                                    queue.push(TuiEvent::UserMessage(msg));
                                    state.scroll_to_bottom = true;
                                }
                            }
                            KeyCode::Char('j')
                                if key.modifiers.contains(event::KeyModifiers::CONTROL) =>
                            {
                                state.input_text.insert(state.cursor_position, '\n');
                                state.cursor_position += 1;
                            }
                            KeyCode::Char(c) => {
                                state.input_text.insert(state.cursor_position, c);
                                state.cursor_position += 1;
                            }
                            KeyCode::Backspace => {
                                if state.cursor_position > 0 {
                                    state.cursor_position -= 1;
                                    state.input_text.remove(state.cursor_position);
                                }
                            }
                            KeyCode::Delete => {
                                if state.cursor_position < state.input_text.len() {
                                    state.input_text.remove(state.cursor_position);
                                }
                            }
                            KeyCode::Left => {
                                if state.cursor_position > 0 {
                                    state.cursor_position -= 1;
                                }
                            }
                            KeyCode::Right => {
                                if state.cursor_position < state.input_text.len() {
                                    state.cursor_position += 1;
                                }
                            }
                            KeyCode::Up => {
                                if state.chat_scroll > 0 {
                                    state.chat_scroll -= 1;
                                }
                            }
                            KeyCode::Down => {
                                state.chat_scroll += 1;
                            }
                            KeyCode::Home => {
                                state.cursor_position = 0;
                            }
                            KeyCode::End => {
                                state.cursor_position = state.input_text.len();
                            }
                            _ => {}
                        }
                    }
                }
                Event::Mouse(mouse) => {
                    let col = mouse.column;
                    let row = mouse.row;

                    match mouse.kind {
                        MouseEventKind::ScrollDown => {
                            state.chat_scroll += 1;
                        }
                        MouseEventKind::ScrollUp => {
                            if state.chat_scroll > 0 {
                                state.chat_scroll -= 1;
                            }
                        }
                        MouseEventKind::Down(_button) => {
                            // Check if click is in input area (bottom 4 rows)
                            let input_top = state.height.saturating_sub(4);
                            if row >= input_top {
                                state.input_focused = true;
                                // Place cursor roughly at click position (approximate)
                                let rel_col = col.saturating_sub(3) as usize;
                                state.cursor_position = rel_col.min(state.input_text.len());
                            } else {
                                state.input_focused = false;
                            }
                        }
                        MouseEventKind::Moved => {
                            // Hover detection for tooltips could go here
                        }
                        _ => {}
                    }
                }
                Event::Resize(w, h) => {
                    state.width = w;
                    state.height = h;
                    queue.push(TuiEvent::Resize { width: w, height: h });
                }
                _ => {}
            }
        }

        if last_tick.elapsed() >= tick_rate {
            state.pulse_frame = state.pulse_frame.wrapping_add(1);
            last_tick = Instant::now();
        }
    };

    // Terminal restoration is handled by TerminalGuard's Drop impl.
    let _ = terminal.show_cursor();
    result
}

/// Mutable UI state for the REPL.
pub struct ReplUIState {
    pub history: Vec<(String, String)>, // (role, content)
    pub token_buffer: String,
    pub current_tool: String,
    pub thinking: bool,
    pub input_text: String,
    pub cursor_position: usize,
    pub chat_scroll: usize,
    pub pulse_frame: u64,
    pub width: u16,
    pub height: u16,
    pub input_focused: bool,
    pub scroll_to_bottom: bool,
    pub queue: EventQueue,
    pub version: String,
    pub session_id: String,
}

impl ReplUIState {
    pub fn new(queue: EventQueue) -> Self {
        Self {
            history: Vec::new(),
            token_buffer: String::new(),
            current_tool: String::new(),
            thinking: false,
            input_text: String::new(),
            cursor_position: 0,
            chat_scroll: 0,
            pulse_frame: 0,
            width: 80,
            height: 24,
            input_focused: true,
            scroll_to_bottom: false,
            queue,
            version: String::from("v3.x"),
            session_id: String::new(),
        }
    }
}

fn draw(f: &mut Frame, state: &mut ReplUIState) {
    let theme = Theme::default();
    let area = f.area();
    state.width = area.width;
    state.height = area.height;

    // Responsive layout sizing
    let header_height = if state.width >= 80 { 3 } else { 2 };
    let input_height = if state.width >= 50 { 4 } else { 3 };

    // Main vertical layout: header | chat | input
    let main_chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(header_height),
            Constraint::Min(5),
            Constraint::Length(input_height),
        ])
        .margin(1)
        .split(area);

    // ── Header ─────────────────────────────────────────────────────────────
    let header_area = main_chunks[0];
    let pulse_color = if state.pulse_frame % 2 == 0 {
        theme.palette.primary
    } else {
        theme.palette.neon
    };

    let header_spans = vec![
        Span::styled("▶ ", Style::default().fg(pulse_color).add_modifier(Modifier::BOLD)),
        Span::styled("ASSISTCLAW ", Style::default().fg(theme.palette.primary).add_modifier(Modifier::BOLD)),
        Span::styled("⟨REPL⟩", Style::default().fg(theme.palette.accent).add_modifier(Modifier::BOLD)),
        Span::styled(
            format!("    {}    ·    session {}", state.version, short_id(&state.session_id)),
            Style::default().fg(theme.palette.muted),
        ),
    ];
    let header = Paragraph::new(Line::from(header_spans));
    f.render_widget(header, header_area);

    // Scanline divider
    if state.width >= 50 {
        let scanline_width = (state.width as usize).saturating_sub(8).min(72).max(24);
        let scanline_text = "·░".repeat((scanline_width + 1) / 2);
        let scanline = Paragraph::new(&scanline_text[..scanline_width.min(scanline_text.len())])
            .style(Style::default().fg(theme.palette.border));
        let scanline_area = Rect {
            x: header_area.x + 1,
            y: header_area.y + header_height - 1,
            width: scanline_width as u16,
            height: 1,
        };
        f.render_widget(scanline, scanline_area);
    }

    // ── Chat Viewport ──────────────────────────────────────────────────────
    let chat_area = main_chunks[1];
    let border_color = if state.pulse_frame % 2 == 0 {
        theme.palette.primary
    } else {
        theme.palette.neon
    };

    let chat_block = Block::default()
        .borders(Borders::ALL)
        .border_type(ratatui::widgets::BorderType::Rounded)
        .border_style(Style::default().fg(border_color))
        .style(Style::default().bg(theme.palette.surface));

    let inner_chat = chat_block.inner(chat_area);
    f.render_widget(chat_block, chat_area);

    // Build chat content
    let mut lines: Vec<Line> = Vec::new();
    for (role, content) in &state.history {
        match role.as_str() {
            "meta" => {
                lines.push(Line::from(Span::styled(
                    format!("   {content}"),
                    Style::default().fg(theme.palette.muted),
                )));
            }
            "error" => {
                lines.push(Line::from(Span::styled(
                    format!("✗ {content}"),
                    Style::default().fg(theme.palette.error),
                )));
            }
            _ => {
                let prefix = if role == "user" {
                    Span::styled("You › ", theme.user_prefix())
                } else {
                    Span::styled("🤖 ", theme.agent_prefix())
                };
                // Multi-line messages render each line; the prefix marks the first.
                let mut first = true;
                for part in content.split('\n') {
                    let text = Span::styled(part.to_string(), Style::default().fg(theme.palette.white));
                    if first {
                        lines.push(Line::from(vec![prefix.clone(), text]));
                        first = false;
                    } else {
                        lines.push(Line::from(vec![Span::raw("      "), text]));
                    }
                }
            }
        }
        lines.push(Line::from(""));
    }

    // Current streaming token (may contain newlines mid-stream)
    if !state.token_buffer.is_empty() {
        let mut first = true;
        for part in state.token_buffer.split('\n') {
            let text = Span::styled(part.to_string(), Style::default().fg(theme.palette.white));
            if first {
                lines.push(Line::from(vec![Span::styled("🤖 ", theme.agent_prefix()), text]));
                first = false;
            } else {
                lines.push(Line::from(vec![Span::raw("      "), text]));
            }
        }
    }

    // Current tool
    if !state.current_tool.is_empty() {
        let tool_text = format!("  ⚡ {} ", state.current_tool);
        let spinner_char = match state.pulse_frame % 4 {
            0 => "◐",
            1 => "◓",
            2 => "◑",
            _ => "◒",
        };
        lines.push(Line::from(vec![
            Span::styled(tool_text, theme.tool_badge()),
            Span::styled(spinner_char, theme.cyan()),
        ]));
    }

    // Handle auto-scroll
    let content_height = lines.len() as u16;
    if state.scroll_to_bottom {
        if content_height > inner_chat.height {
            state.chat_scroll = (content_height - inner_chat.height) as usize;
        } else {
            state.chat_scroll = 0;
        }
        state.scroll_to_bottom = false;
    }

    let chat_paragraph = Paragraph::new(Text::from(lines))
        .wrap(Wrap { trim: true })
        .scroll((state.chat_scroll as u16, 0));
    f.render_widget(chat_paragraph, inner_chat);

    // Scrollbar
    if content_height > inner_chat.height {
        let mut scrollbar_state = ratatui::widgets::ScrollbarState::new(content_height as usize)
            .position(state.chat_scroll);
        f.render_stateful_widget(
            Scrollbar::new(ScrollbarOrientation::VerticalRight)
                .begin_symbol(Some("↑"))
                .end_symbol(Some("↓"))
                .track_symbol(Some("│"))
                .thumb_symbol("█"),
            inner_chat.inner(Margin { horizontal: 0, vertical: 0 }),
            &mut scrollbar_state,
        );
    }

    // ── Input Box ──────────────────────────────────────────────────────────
    let input_area = main_chunks[2];
    let input_border_style = if state.input_focused {
        Style::default().fg(theme.palette.primary)
    } else {
        Style::default().fg(theme.palette.border)
    };

    let input_block = Block::default()
        .borders(Borders::TOP)
        .border_style(input_border_style);

    let inner_input = input_block.inner(input_area);
    f.render_widget(input_block, input_area);

    // Input text with placeholder
    let input_display = if state.input_text.is_empty() && !state.input_focused {
        Span::styled("Message AssistClaw...", Style::default().fg(theme.palette.muted))
    } else {
        Span::styled(state.input_text.as_str(), Style::default().fg(theme.palette.white))
    };

    let input_line = Line::from(vec![Span::styled("› ", theme.primary()), input_display]);
    let input_paragraph = Paragraph::new(input_line);
    f.render_widget(input_paragraph, Rect {
        x: inner_input.x,
        y: inner_input.y,
        width: inner_input.width,
        height: 1,
    });

    // Footer hints
    let footer_text = if state.thinking {
        let spinner_char = match state.pulse_frame % 4 {
            0 => "◐",
            1 => "◓",
            2 => "◑",
            _ => "◒",
        };
        let mut text = format!("{} · thinking", spinner_char);
        if !state.current_tool.is_empty() {
            text.push_str(&format!("  ⚡ {}", state.current_tool));
        }
        text
    } else {
        "↵ send  ·  Ctrl+J newline  ·  ↑↓ scroll  ·  ESC quit".to_string()
    };
    let footer = Paragraph::new(footer_text)
        .style(Style::default().fg(theme.palette.muted));
    f.render_widget(footer, Rect {
        x: inner_input.x + 2,
        y: inner_input.y + inner_input.height.saturating_sub(1),
        width: inner_input.width.saturating_sub(2),
        height: 1,
    });
}

fn short_id(id: &str) -> &str {
    if id.len() > 8 {
        &id[..8]
    } else {
        id
    }
}
