//! Navigation sidebar widget.

use crate::theme::Theme;
use ratatui::style::{Modifier, Style};

pub struct SidebarItem {
    pub label: String,
    pub icon: char,
    pub active: bool,
    pub hovered: bool,
}

impl SidebarItem {
    pub fn render(&self, theme: &Theme) -> String {
        let _style = if self.active {
            Style::default()
                .fg(theme.palette.bg)
                .bg(theme.palette.primary)
                .add_modifier(Modifier::BOLD)
        } else if self.hovered {
            Style::default()
                .fg(theme.palette.primary)
                .add_modifier(Modifier::BOLD)
        } else {
            Style::default().fg(theme.palette.muted)
        };

        if self.active {
            format!(" ▶ {} {} ", self.icon, self.label)
        } else {
            format!("   {} {} ", self.icon, self.label)
        }
    }
}

pub struct Sidebar {
    pub items: Vec<SidebarItem>,
}

impl Sidebar {
    pub fn new() -> Self {
        Self { items: Vec::new() }
    }

    pub fn add(&mut self, label: impl Into<String>, icon: char) {
        self.items.push(SidebarItem {
            label: label.into(),
            icon,
            active: false,
            hovered: false,
        });
    }

    pub fn render(&self, theme: &Theme) -> Vec<String> {
        self.items.iter().map(|item| item.render(theme)).collect()
    }
}
