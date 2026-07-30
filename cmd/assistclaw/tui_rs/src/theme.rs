//! Design system — color palette and styles ported from the Go lipgloss theme.

use ratatui::style::{Color, Modifier, Style};

/// Full color palette derived from the lime-green robot logo.
pub struct Palette {
    pub primary: Color,  // #7CC72C — logo lime-green
    pub neon: Color,     // #AAEB47 — bright neon pop
    pub border: Color,   // #2E4020 — dim green border
    pub surface: Color,  // #14180F — dark card bg
    pub bg: Color,       // #0D0F0D — near-black
    pub muted: Color,    // #6B7B6B — dimmed text
    pub cyan: Color,     // #44CCCC — tool indicator
    pub error: Color,    // #E05A4E — error red
    pub white: Color,    // #E8F0E8 — soft white
    pub accent: Color,   // #B388FF — magenta cyber accent
}

pub static PALETTE: Palette = Palette {
    primary: Color::from_u32(0x7CC72C),
    neon: Color::from_u32(0xAAEB47),
    border: Color::from_u32(0x2E4020),
    surface: Color::from_u32(0x14180F),
    bg: Color::from_u32(0x0D0F0D),
    muted: Color::from_u32(0x6B7B6B),
    cyan: Color::from_u32(0x44CCCC),
    error: Color::from_u32(0xE05A4E),
    white: Color::from_u32(0xE8F0E8),
    accent: Color::from_u32(0xB388FF),
};

/// Pre-built styles for common use cases.
pub struct Theme {
    pub palette: &'static Palette,
}

impl Default for Theme {
    fn default() -> Self {
        Self {
            palette: &PALETTE,
        }
    }
}

impl Theme {
    pub fn primary(&self) -> Style {
        Style::default()
            .fg(self.palette.primary)
            .add_modifier(Modifier::BOLD)
    }

    pub fn neon(&self) -> Style {
        Style::default()
            .fg(self.palette.neon)
            .add_modifier(Modifier::BOLD)
    }

    pub fn muted(&self) -> Style {
        Style::default().fg(self.palette.muted)
    }

    pub fn white(&self) -> Style {
        Style::default()
            .fg(self.palette.white)
            .add_modifier(Modifier::BOLD)
    }

    pub fn error(&self) -> Style {
        Style::default()
            .fg(self.palette.error)
            .add_modifier(Modifier::BOLD)
    }

    pub fn cyan(&self) -> Style {
        Style::default()
            .fg(self.palette.cyan)
            .add_modifier(Modifier::BOLD)
    }

    pub fn accent(&self) -> Style {
        Style::default()
            .fg(self.palette.accent)
            .add_modifier(Modifier::BOLD)
    }

    pub fn surface_bg(&self) -> Style {
        Style::default().bg(self.palette.surface)
    }

    pub fn main_border(&self) -> Style {
        Style::default()
            .fg(self.palette.primary)
            .bg(self.palette.surface)
    }

    pub fn input_border(&self) -> Style {
        Style::default().fg(self.palette.border)
    }

    pub fn user_prefix(&self) -> Style {
        Style::default()
            .fg(self.palette.neon)
            .add_modifier(Modifier::BOLD)
    }

    pub fn agent_prefix(&self) -> Style {
        Style::default()
            .fg(self.palette.primary)
            .add_modifier(Modifier::BOLD)
    }

    pub fn tool_badge(&self) -> Style {
        Style::default()
            .fg(self.palette.cyan)
            .add_modifier(Modifier::BOLD)
    }

    pub fn status_ok(&self) -> Style {
        Style::default()
            .fg(self.palette.primary)
            .add_modifier(Modifier::BOLD)
    }

    pub fn status_err(&self) -> Style {
        Style::default()
            .fg(self.palette.error)
            .add_modifier(Modifier::BOLD)
    }

    /// Pulsing border color that alternates between primary and neon.
    pub fn pulse_border(&self, frame: u64) -> Style {
        let color = if frame % 2 == 0 {
            self.palette.primary
        } else {
            self.palette.neon
        };
        Style::default().fg(color)
    }

    /// Render a short string with a horizontal green→cyan→neon sweep.
    pub fn gradient_word(&self, s: &str) -> String {
        if s.is_empty() {
            return String::new();
        }
        let palette = [
            self.palette.primary,
            self.palette.cyan,
            self.palette.neon,
            self.palette.cyan,
            self.palette.primary,
        ];
        s.chars()
            .enumerate()
            .map(|(i, c)| {
                let color = palette[i % palette.len()];
                let _style = Style::default()
                    .fg(color)
                    .add_modifier(Modifier::BOLD);
                // We return a string here; callers use ratatui::text::Span
                format!("\x1b[38;2;{};{};{}m{c}\x1b[0m",
                    color.r(), color.g(), color.b())
            })
            .collect()
    }
}

/// Hex helpers for Color.
pub trait ColorExt {
    fn from_u32(hex: u32) -> Self;
    fn r(&self) -> u8;
    fn g(&self) -> u8;
    fn b(&self) -> u8;
}

impl ColorExt for Color {
    fn from_u32(hex: u32) -> Self {
        Color::Rgb(
            ((hex >> 16) & 0xFF) as u8,
            ((hex >> 8) & 0xFF) as u8,
            (hex & 0xFF) as u8,
        )
    }

    fn r(&self) -> u8 {
        match self {
            Color::Rgb(r, _, _) => *r,
            _ => 0,
        }
    }

    fn g(&self) -> u8 {
        match self {
            Color::Rgb(_, g, _) => *g,
            _ => 0,
        }
    }

    fn b(&self) -> u8 {
        match self {
            Color::Rgb(_, _, b) => *b,
            _ => 0,
        }
    }
}
