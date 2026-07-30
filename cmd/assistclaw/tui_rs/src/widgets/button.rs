//! Clickable button widget.

use ratatui::style::{Color, Modifier, Style};

pub struct Button<'a> {
    pub label: &'a str,
    pub active: bool,
    pub hovered: bool,
}

impl<'a> Button<'a> {
    pub fn new(label: &'a str) -> Self {
        Self {
            label,
            active: false,
            hovered: false,
        }
    }

    pub fn style(&self, base_fg: Color, base_bg: Color) -> Style {
        let mut style = Style::default();
        if self.active {
            style = style
                .fg(base_bg)
                .bg(base_fg)
                .add_modifier(Modifier::BOLD);
        } else if self.hovered {
            style = style
                .fg(base_fg)
                .bg(base_bg)
                .add_modifier(Modifier::BOLD);
        } else {
            style = style.fg(base_fg);
        }
        style
    }

    pub fn render(&self) -> String {
        if self.active || self.hovered {
            format!("[ {} ]", self.label)
        } else {
            format!("  {}  ", self.label)
        }
    }
}
