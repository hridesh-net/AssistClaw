//! Skills Configuration screen.

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
use serde::Deserialize;
use std::io;
use std::time::{Duration, Instant};

#[derive(Debug, Clone, Deserialize)]
pub struct MarketplaceEntry {
    pub name: String,
    pub description: String,
    #[serde(default)]
    pub emoji: String,
    #[serde(default)]
    pub bundled: bool,
    #[serde(default)]
    pub tags: Vec<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct MarketplaceIndex {
    #[serde(default)]
    pub version: String,
    #[serde(default)]
    pub updated: String,
    pub skills: Vec<MarketplaceEntry>,
}

#[derive(Debug, Clone)]
pub struct SkillItem {
    pub name: String,
    pub description: String,
    pub emoji: String,
    pub installed: bool,
    pub enabled: bool,
}

pub struct SkillsState {
    pub items: Vec<SkillItem>,
    pub filtered: Vec<usize>,
    pub selected: usize,
    pub search: String,
    pub width: u16,
    pub height: u16,
    pub completed: bool,
    pub cancelled: bool,
}

impl SkillsState {
    pub fn new(items: Vec<SkillItem>) -> Self {
        let filtered: Vec<usize> = (0..items.len()).collect();
        Self {
            items,
            filtered,
            selected: 0,
            search: String::new(),
            width: 80,
            height: 24,
            completed: false,
            cancelled: false,
        }
    }

    pub fn apply_filter(&mut self) {
        let query = self.search.to_lowercase();
        self.filtered = self
            .items
            .iter()
            .enumerate()
            .filter(|(_, item)| {
                query.is_empty()
                    || item.name.to_lowercase().contains(&query)
                    || item.description.to_lowercase().contains(&query)
            })
            .map(|(i, _)| i)
            .collect();
        if self.selected >= self.filtered.len() {
            self.selected = self.filtered.len().saturating_sub(1);
        }
    }

    pub fn current_item(&self) -> Option<&SkillItem> {
        self.filtered.get(self.selected).and_then(|&i| self.items.get(i))
    }

    pub fn toggle_current(&mut self) {
        if let Some(&idx) = self.filtered.get(self.selected) {
            if let Some(item) = self.items.get_mut(idx) {
                item.enabled = !item.enabled;
            }
        }
    }

    pub fn selected_names(&self) -> Vec<String> {
        self.items
            .iter()
            .filter(|i| i.enabled)
            .map(|i| i.name.clone())
            .collect()
    }
}

pub fn run(catalog_json: &str, _current_skills_json: &str) -> Option<Vec<String>> {
    let items = parse_catalog(catalog_json);
    if items.is_empty() {
        // Fallback demo skills if parsing fails
        return Some(Vec::new());
    }

    if let Err(e) = run_ui(items) {
        eprintln!("[tui_rs] Skills error: {e}");
        return None;
    }

    // run_ui doesn't return state directly. For now return None
    // to indicate the Go side should use the event queue or fallback.
    None
}

fn parse_catalog(json: &str) -> Vec<SkillItem> {
    let index: MarketplaceIndex = match serde_json::from_str(json) {
        Ok(i) => i,
        Err(e) => {
            eprintln!("[tui_rs] Failed to parse catalog: {e}");
            return Vec::new();
        }
    };

    index
        .skills
        .into_iter()
        .map(|e| SkillItem {
            name: e.name.clone(),
            description: e.description,
            emoji: if e.emoji.is_empty() {
                "📦".to_string()
            } else {
                e.emoji
            },
            installed: false,
            enabled: false,
        })
        .collect()
}

fn run_ui(items: Vec<SkillItem>) -> Result<()> {
    let enable_mouse = !crate::terminal::detect_multiplexer();
    let _guard = TerminalGuard::enter(enable_mouse)?;
    let backend = CrosstermBackend::new(io::stdout());
    let mut terminal = Terminal::new(backend)?;

    let mut state = SkillsState::new(items);
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
                                state.completed = true;
                                break Ok(());
                            }
                            KeyCode::Char(' ') => {
                                state.toggle_current();
                            }
                            KeyCode::Char(c) => {
                                state.search.push(c);
                                state.apply_filter();
                            }
                            KeyCode::Backspace => {
                                state.search.pop();
                                state.apply_filter();
                            }
                            KeyCode::Up => {
                                if state.selected > 0 {
                                    state.selected -= 1;
                                }
                            }
                            KeyCode::Down => {
                                if state.selected + 1 < state.filtered.len() {
                                    state.selected += 1;
                                }
                            }
                            _ => {}
                        }
                    }
                }
                Event::Mouse(mouse) => {
                    match mouse.kind {
                        MouseEventKind::ScrollDown => {
                            if state.selected + 1 < state.filtered.len() {
                                state.selected += 1;
                            }
                        }
                        MouseEventKind::ScrollUp => {
                            if state.selected > 0 {
                                state.selected -= 1;
                            }
                        }
                        MouseEventKind::Down(_) => {
                            // Click to select could be implemented with layout tracking
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

fn draw(f: &mut Frame, state: &mut SkillsState) {
    let theme = Theme::default();
    let area = f.area();
    state.width = area.width;
    state.height = area.height;

    // Layout: search bar | list (60%) | detail (40%)
    let main_chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(3), Constraint::Min(5)])
        .margin(1)
        .split(area);

    // Search bar
    let search_area = main_chunks[0];
    let search_block = Block::default()
        .title(" Skills ")
        .borders(Borders::ALL)
        .border_type(ratatui::widgets::BorderType::Rounded)
        .border_style(Style::default().fg(theme.palette.primary));
    let search_inner = search_block.inner(search_area);
    f.render_widget(search_block, search_area);

    let search_text = if state.search.is_empty() {
        Span::styled("Type to search...", Style::default().fg(theme.palette.muted))
    } else {
        Span::styled(format!("/ {}", state.search), Style::default().fg(theme.palette.white))
    };
    let search_para = Paragraph::new(Line::from(vec![
        Span::styled("🔍 ", Style::default().fg(theme.palette.primary)),
        search_text,
    ]));
    f.render_widget(search_para, search_inner);

    // Content area split
    let content_area = main_chunks[1];
    let list_width = (content_area.width as f32 * 0.6) as u16;
    let detail_width = content_area.width.saturating_sub(list_width).saturating_sub(1);

    let list_area = Rect {
        x: content_area.x,
        y: content_area.y,
        width: list_width,
        height: content_area.height,
    };
    let detail_area = Rect {
        x: content_area.x + list_width + 1,
        y: content_area.y,
        width: detail_width,
        height: content_area.height,
    };

    // Skill list
    let list_block = Block::default()
        .borders(Borders::ALL)
        .border_type(ratatui::widgets::BorderType::Rounded)
        .border_style(Style::default().fg(theme.palette.border));
    let list_inner = list_block.inner(list_area);
    f.render_widget(list_block, list_area);

    let mut list_lines: Vec<Line> = Vec::new();
    for (fi, &idx) in state.filtered.iter().enumerate() {
        if let Some(item) = state.items.get(idx) {
            let focused = fi == state.selected;
            let prefix = if focused { "> " } else { "  " };
            let check = if item.enabled { "☑" } else { "☐" };
            let name_style = if focused {
                Style::default().fg(theme.palette.neon).add_modifier(Modifier::BOLD)
            } else if item.enabled {
                Style::default().fg(theme.palette.primary)
            } else {
                Style::default().fg(theme.palette.muted)
            };

            let status = if item.installed {
                if item.enabled {
                    Span::styled(" ✔", Style::default().fg(theme.palette.primary))
                } else {
                    Span::styled(" (installed, disabled)", Style::default().fg(theme.palette.muted))
                }
            } else {
                Span::styled("", Style::default())
            };

            list_lines.push(Line::from(vec![
                Span::styled(prefix, name_style),
                Span::styled(format!("{} ", check), name_style),
                Span::styled(format!("{} {}{}", item.emoji, item.name, if focused { "" } else { "" }), name_style),
                status,
            ]));

            // Description line (indented)
            let max_desc = list_inner.width.saturating_sub(8) as usize;
            let desc = if item.description.len() > max_desc {
                format!("{}…", &item.description[..max_desc.saturating_sub(1)])
            } else {
                item.description.clone()
            };
            list_lines.push(Line::from(vec![
                Span::styled("     ", Style::default()),
                Span::styled(desc, Style::default().fg(theme.palette.muted)),
            ]));
        }
    }

    if list_lines.is_empty() {
        list_lines.push(Line::from(Span::styled(
            "No skills match your search.",
            Style::default().fg(theme.palette.muted),
        )));
    }

    let list_para = Paragraph::new(list_lines);
    f.render_widget(list_para, list_inner);

    // Detail panel
    let detail_block = Block::default()
        .title(" Detail ")
        .title_alignment(Alignment::Center)
        .borders(Borders::ALL)
        .border_type(ratatui::widgets::BorderType::Rounded)
        .border_style(Style::default().fg(theme.palette.border));
    let detail_inner = detail_block.inner(detail_area);
    f.render_widget(detail_block, detail_area);

    let mut detail_lines: Vec<Line> = Vec::new();
    if let Some(item) = state.current_item() {
        detail_lines.push(Line::from(vec![
            Span::styled(&item.emoji, Style::default().fg(theme.palette.primary).add_modifier(Modifier::BOLD)),
            Span::styled(" ", Style::default()),
            Span::styled(&item.name, Style::default().fg(theme.palette.white).add_modifier(Modifier::BOLD)),
        ]));
        detail_lines.push(Line::from(""));
        detail_lines.push(Line::from(Span::styled(&item.description, Style::default().fg(theme.palette.muted))));
        detail_lines.push(Line::from(""));
        detail_lines.push(Line::from(vec![
            Span::styled("Status: ", Style::default().fg(theme.palette.muted)),
            if item.installed {
                Span::styled("Installed", Style::default().fg(theme.palette.primary))
            } else {
                Span::styled("Not installed", Style::default().fg(theme.palette.error))
            },
        ]));
        detail_lines.push(Line::from(vec![
            Span::styled("Enabled: ", Style::default().fg(theme.palette.muted)),
            if item.enabled {
                Span::styled("Yes", Style::default().fg(theme.palette.primary))
            } else {
                Span::styled("No", Style::default().fg(theme.palette.muted))
            },
        ]));
    } else {
        detail_lines.push(Line::from(Span::styled(
            "No skills match your search.",
            Style::default().fg(theme.palette.muted),
        )));
    }

    let detail_para = Paragraph::new(detail_lines);
    f.render_widget(detail_para, detail_inner);
}
