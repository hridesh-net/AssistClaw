//! ASCII robot banner with gradient title.

use crate::theme::Theme;
use ratatui::style::{Modifier, Style};

const ROBOT_LINES: &[&str] = &[
    "     ╷     ",
    "   ┌─┴─┐   ",
    "   │◉ ◉│   ",
    "   │ ▬ │   ",
    "   └──┬┘   ",
    "  ╔═══╧═╗  ",
    "  ║CLAW ║  ",
    "  ╚══╤══╝  ",
    "   ┌─┴─┐   ",
    "   └───┘   ",
];

pub fn render_banner(theme: &Theme, _ver: &str, _session_id: &str, _providers: usize, _skills: usize) -> String {
    let robot_style = Style::default()
        .fg(theme.palette.primary)
        .add_modifier(Modifier::BOLD);
    let glow_style = Style::default()
        .fg(theme.palette.neon)
        .add_modifier(Modifier::BOLD);

    let mut result = String::new();
    for (i, line) in ROBOT_LINES.iter().enumerate() {
        let _styled = if i == 2 || i == 3 {
            glow_style
        } else {
            robot_style
        };
        // We can't easily render styled strings to plain String without a buffer,
        // so for now just return plain text. Full implementation will use ratatui Frame.
        result.push_str(line);
        result.push('\n');
    }
    result
}
