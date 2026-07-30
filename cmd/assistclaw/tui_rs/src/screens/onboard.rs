//! Onboarding Wizard screen.

use crate::event::TuiEvent;
use crate::queue::EventQueue;
use crate::terminal::TerminalGuard;
use crate::theme::Theme;
use color_eyre::Result;
use crossterm::event::{self, Event, KeyCode, KeyEventKind, MouseEventKind};
use ratatui::{
    backend::CrosstermBackend,
    layout::{Alignment, Constraint, Direction, Layout, Rect},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame, Terminal,
};
use std::io;
use std::time::{Duration, Instant};

#[derive(Debug, Clone)]
pub struct OnboardStep {
    pub title: String,
    pub description: String,
    pub fields: Vec<OnboardField>,
}

#[derive(Debug, Clone)]
pub enum OnboardField {
    Select { label: String, options: Vec<String>, selected: usize },
    Input { label: String, value: String },
    Toggle { label: String, enabled: bool },
}

pub struct OnboardState {
    pub steps: Vec<OnboardStep>,
    pub current_step: usize,
    pub width: u16,
    pub height: u16,
    pub focused_field: usize,
    pub completed: bool,
    pub cancelled: bool,
}

impl OnboardState {
    pub fn new() -> Self {
        Self {
            steps: vec![
                OnboardStep {
                    title: "Primary Provider".to_string(),
                    description: "Choose your main LLM provider".to_string(),
                    fields: vec![
                        OnboardField::Select {
                            label: "Provider".to_string(),
                            options: vec![
                                "OpenAI".to_string(),
                                "Anthropic".to_string(),
                                "Bedrock".to_string(),
                                "Ollama".to_string(),
                                "Groq".to_string(),
                                "Vertex".to_string(),
                            ],
                            selected: 0,
                        },
                    ],
                },
                OnboardStep {
                    title: "Model".to_string(),
                    description: "Select the model to use".to_string(),
                    fields: vec![
                        OnboardField::Select {
                            label: "Model".to_string(),
                            options: vec![
                                "gpt-4o".to_string(),
                                "claude-3-5-sonnet".to_string(),
                                "llama3.1".to_string(),
                            ],
                            selected: 0,
                        },
                    ],
                },
                OnboardStep {
                    title: "Gateway".to_string(),
                    description: "Configure the web UI and API gateway".to_string(),
                    fields: vec![
                        OnboardField::Toggle {
                            label: "Enable Gateway".to_string(),
                            enabled: true,
                        },
                        OnboardField::Input {
                            label: "Port".to_string(),
                            value: "18790".to_string(),
                        },
                    ],
                },
                OnboardStep {
                    title: "Channels".to_string(),
                    description: "Enable messaging channels".to_string(),
                    fields: vec![
                        OnboardField::Toggle {
                            label: "Telegram".to_string(),
                            enabled: false,
                        },
                        OnboardField::Toggle {
                            label: "Discord".to_string(),
                            enabled: false,
                        },
                        OnboardField::Toggle {
                            label: "Slack".to_string(),
                            enabled: false,
                        },
                    ],
                },
                OnboardStep {
                    title: "Skills".to_string(),
                    description: "Select skills to enable".to_string(),
                    fields: vec![
                        OnboardField::Toggle {
                            label: "1Password".to_string(),
                            enabled: false,
                        },
                        OnboardField::Toggle {
                            label: "Apple Notes".to_string(),
                            enabled: false,
                        },
                        OnboardField::Toggle {
                            label: "Web Fetch".to_string(),
                            enabled: true,
                        },
                    ],
                },
            ],
            current_step: 0,
            width: 80,
            height: 24,
            focused_field: 0,
            completed: false,
            cancelled: false,
        }
    }
}

pub fn run(_config_json: &str, _queue: EventQueue) -> Option<String> {
    if let Err(e) = run_ui() {
        eprintln!("[tui_rs] Onboard error: {e}");
        return None;
    }
    // Return empty config for now — full implementation would serialize wizard state
    Some("{}".to_string())
}

fn run_ui() -> Result<()> {
    let enable_mouse = !crate::terminal::detect_multiplexer();
    let _guard = TerminalGuard::enter(enable_mouse)?;
    let backend = CrosstermBackend::new(io::stdout());
    let mut terminal = Terminal::new(backend)?;

    let mut state = OnboardState::new();
    let mut last_tick = Instant::now();
    let tick_rate = Duration::from_millis(60);

    let result = loop {
        terminal.draw(|f| draw(f, &mut state))?;

        let timeout = tick_rate.saturating_sub(last_tick.elapsed());
        if crossterm::event::poll(timeout)? {
            match event::read()? {
                Event::Key(key) => {
                    if key.kind == KeyEventKind::Press {
                        match key.code {
                            KeyCode::Esc => {
                                state.cancelled = true;
                                break Ok(());
                            }
                            KeyCode::Enter => {
                                if state.current_step < state.steps.len() - 1 {
                                    state.current_step += 1;
                                    state.focused_field = 0;
                                } else {
                                    state.completed = true;
                                    break Ok(());
                                }
                            }
                            KeyCode::BackTab | KeyCode::Left => {
                                if state.current_step > 0 {
                                    state.current_step -= 1;
                                    state.focused_field = 0;
                                }
                            }
                            KeyCode::Tab | KeyCode::Right => {
                                if state.current_step < state.steps.len() - 1 {
                                    state.current_step += 1;
                                    state.focused_field = 0;
                                }
                            }
                            KeyCode::Up => {
                                if state.focused_field > 0 {
                                    state.focused_field -= 1;
                                }
                            }
                            KeyCode::Down => {
                                let step = &state.steps[state.current_step];
                                if state.focused_field < step.fields.len().saturating_sub(1) {
                                    state.focused_field += 1;
                                }
                            }
                            KeyCode::Char(' ') => {
                                toggle_current_field(&mut state);
                            }
                            _ => {}
                        }
                    }
                }
                Event::Mouse(mouse) => {
                    match mouse.kind {
                        MouseEventKind::ScrollDown => {
                            let step = &state.steps[state.current_step];
                            if state.focused_field < step.fields.len().saturating_sub(1) {
                                state.focused_field += 1;
                            }
                        }
                        MouseEventKind::ScrollUp => {
                            if state.focused_field > 0 {
                                state.focused_field -= 1;
                            }
                        }
                        MouseEventKind::Down(_) => {
                            // Simple click-to-focus logic could go here
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
            last_tick = Instant::now();
        }
    };

    let _ = terminal.show_cursor();
    result
}

fn toggle_current_field(state: &mut OnboardState) {
    let step = &mut state.steps[state.current_step];
    if let Some(field) = step.fields.get_mut(state.focused_field) {
        match field {
            OnboardField::Toggle { enabled, .. } => {
                *enabled = !*enabled;
            }
            OnboardField::Select { selected, options, .. } => {
                *selected = (*selected + 1) % options.len();
            }
            _ => {}
        }
    }
}

fn draw(f: &mut Frame, state: &mut OnboardState) {
    let theme = Theme::default();
    let area = f.area();
    state.width = area.width;
    state.height = area.height;

    // Breadcrumb at top
    let breadcrumb_height = 2_u16;
    let main_chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(breadcrumb_height), Constraint::Min(10)])
        .margin(1)
        .split(area);

    // Render breadcrumb
    let breadcrumb_spans: Vec<Span> = state
        .steps
        .iter()
        .enumerate()
        .flat_map(|(i, step)| {
            let mut spans = Vec::new();
            let style = if i == state.current_step {
                Style::default()
                    .fg(theme.palette.bg)
                    .bg(theme.palette.primary)
                    .add_modifier(Modifier::BOLD)
            } else if i < state.current_step {
                Style::default().fg(theme.palette.primary)
            } else {
                Style::default().fg(theme.palette.muted)
            };
            spans.push(Span::styled(format!(" {} ", step.title), style));
            if i < state.steps.len() - 1 {
                spans.push(Span::styled(" > ", Style::default().fg(theme.palette.muted)));
            }
            spans
        })
        .collect();

    let breadcrumb = Paragraph::new(Line::from(breadcrumb_spans));
    f.render_widget(breadcrumb, main_chunks[0]);

    // Main content area
    let content_area = main_chunks[1];
    let step = &state.steps[state.current_step];

    let block = Block::default()
        .title(format!(" {} ", step.title))
        .title_alignment(Alignment::Center)
        .borders(Borders::ALL)
        .border_type(ratatui::widgets::BorderType::Rounded)
        .border_style(Style::default().fg(theme.palette.primary))
        .style(Style::default().bg(theme.palette.surface));

    let inner = block.inner(content_area);
    f.render_widget(block, content_area);

    let mut lines: Vec<Line> = Vec::new();
    lines.push(Line::from(Span::styled(
        &step.description,
        Style::default().fg(theme.palette.muted),
    )));
    lines.push(Line::from(""));

    for (fi, field) in step.fields.iter().enumerate() {
        let focused = fi == state.focused_field;
        let prefix = if focused { "> " } else { "  " };
        let focus_style = if focused {
            Style::default().fg(theme.palette.neon).add_modifier(Modifier::BOLD)
        } else {
            Style::default().fg(theme.palette.white)
        };

        match field {
            OnboardField::Select { label, options, selected } => {
                let selected_opt = options.get(*selected).map(|s| s.as_str()).unwrap_or("");
                lines.push(Line::from(vec![
                    Span::styled(prefix, focus_style),
                    Span::styled(format!("{}: [ {} ] ▼", label, selected_opt), focus_style),
                ]));
            }
            OnboardField::Input { label, value } => {
                lines.push(Line::from(vec![
                    Span::styled(prefix, focus_style),
                    Span::styled(format!("{}: {}", label, value), focus_style),
                ]));
            }
            OnboardField::Toggle { label, enabled } => {
                let symbol = if *enabled { "☑" } else { "☐" };
                lines.push(Line::from(vec![
                    Span::styled(prefix, focus_style),
                    Span::styled(format!("{} {}", symbol, label), focus_style),
                ]));
            }
        }
    }

    lines.push(Line::from(""));
    lines.push(Line::from(Span::styled(
        "Enter = next  ·  Shift+Tab = back  ·  Space = toggle  ·  Esc = cancel",
        Style::default().fg(theme.palette.muted),
    )));

    let paragraph = Paragraph::new(lines);
    f.render_widget(paragraph, inner);
}
