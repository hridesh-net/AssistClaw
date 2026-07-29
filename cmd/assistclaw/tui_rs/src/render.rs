//! One-shot ANSI rendering for splash banners, CLI headers, and version blocks.
//!
//! These render functions produce ANSI-coloured strings that the Go caller
//! prints directly. No terminal capture or ratatui machinery — pure string
//! composition. Mirrors the lipgloss output the Go TUI previously produced.

use strip_ansi_escapes::strip;
use unicode_width::UnicodeWidthStr;

// ── Palette (matches former Go tui/theme.go) ────────────────────────────────

pub const PRIMARY: (u8, u8, u8) = (0x7C, 0xC7, 0x2C);
pub const NEON: (u8, u8, u8) = (0xAA, 0xEB, 0x47);
pub const BORDER: (u8, u8, u8) = (0x2E, 0x40, 0x20);
pub const SURFACE: (u8, u8, u8) = (0x14, 0x18, 0x0F);
pub const MUTED: (u8, u8, u8) = (0x6B, 0x7B, 0x6B);
pub const CYAN: (u8, u8, u8) = (0x44, 0xCC, 0xCC);
pub const ACCENT: (u8, u8, u8) = (0xB3, 0x88, 0xFF);
pub const WHITE: (u8, u8, u8) = (0xE8, 0xF0, 0xE8);

const RESET: &str = "\x1b[0m";

// ── ANSI primitives ─────────────────────────────────────────────────────────

pub fn fg(rgb: (u8, u8, u8), text: &str) -> String {
    format!("\x1b[38;2;{};{};{}m{}{}", rgb.0, rgb.1, rgb.2, text, RESET)
}

pub fn fg_bold(rgb: (u8, u8, u8), text: &str) -> String {
    format!(
        "\x1b[1;38;2;{};{};{}m{}{}",
        rgb.0, rgb.1, rgb.2, text, RESET
    )
}

/// Visible cell width of a string, ignoring ANSI escape sequences.
fn visible_width(s: &str) -> usize {
    let stripped = strip(s.as_bytes());
    let plain = String::from_utf8_lossy(&stripped);
    UnicodeWidthStr::width(plain.as_ref())
}

fn pad_to_width(line: &str, target: usize) -> String {
    let w = visible_width(line);
    if w >= target {
        return line.to_string();
    }
    format!("{}{}", line, " ".repeat(target - w))
}

// ── Composition helpers ─────────────────────────────────────────────────────

/// Join two multi-line blocks side by side, top-aligned.
/// Shorter block is padded with empty lines to match the taller.
fn join_horizontal_top(left: &str, right: &str) -> String {
    let l: Vec<&str> = left.split('\n').collect();
    let r: Vec<&str> = right.split('\n').collect();
    let max_lines = l.len().max(r.len());

    // Compute each side's max line width so we can pad each row uniformly.
    let l_width = l.iter().map(|s| visible_width(s)).max().unwrap_or(0);
    let r_width = r.iter().map(|s| visible_width(s)).max().unwrap_or(0);

    let mut out = String::new();
    for i in 0..max_lines {
        let lhs = l.get(i).copied().unwrap_or("");
        let rhs = r.get(i).copied().unwrap_or("");
        out.push_str(&pad_to_width(lhs, l_width));
        out.push_str(&pad_to_width(rhs, r_width));
        if i + 1 < max_lines {
            out.push('\n');
        }
    }
    out
}

/// Wrap a content block in a rounded border with the given foreground colour
/// and horizontal padding (0 vertical padding to match lipgloss `Padding(0, 1)`).
fn box_rounded(content: &str, border_rgb: (u8, u8, u8), pad_h: usize) -> String {
    let lines: Vec<&str> = content.split('\n').collect();
    let inner_w = lines.iter().map(|s| visible_width(s)).max().unwrap_or(0);
    let bar_w = inner_w + pad_h * 2;
    let pad = " ".repeat(pad_h);

    let top = format!("╭{}╮", "─".repeat(bar_w));
    let bottom = format!("╰{}╯", "─".repeat(bar_w));
    let vbar = fg(border_rgb, "│");
    let top_styled = fg(border_rgb, &top);
    let bot_styled = fg(border_rgb, &bottom);

    let mut out = String::new();
    out.push_str(&top_styled);
    out.push('\n');
    for line in &lines {
        let padded_line = pad_to_width(line, inner_w);
        out.push_str(&vbar);
        out.push_str(&pad);
        out.push_str(&padded_line);
        out.push_str(&pad);
        out.push_str(&vbar);
        out.push('\n');
    }
    out.push_str(&bot_styled);
    out
}

// ── Branded text fragments ──────────────────────────────────────────────────

/// `ASSISTCLAW` rendered with a horizontal lime→cyan→neon→cyan→lime sweep.
pub fn gradient_word(s: &str) -> String {
    if s.is_empty() {
        return String::new();
    }
    let palette = [PRIMARY, CYAN, NEON, CYAN, PRIMARY];
    let mut out = String::new();
    for (i, ch) in s.chars().enumerate() {
        let c = palette[i % palette.len()];
        out.push_str(&fg_bold(c, &ch.to_string()));
    }
    out
}

/// `⟨INNER⟩` accent-coloured brackets with white bold middle.
pub fn cyber_bracket(inner: &str) -> String {
    format!(
        "{}{}{}",
        fg(ACCENT, "⟨"),
        fg_bold(WHITE, inner),
        fg(ACCENT, "⟩"),
    )
}

/// Dim CRT-style noise strip of `width` cells.
pub fn scanline_suffix(width: usize) -> String {
    let width = if width < 8 { 32 } else { width };
    let raw: String = "·░".chars().cycle().take(width).collect();
    fg(BORDER, &raw)
}

// ── Robot ASCII art ─────────────────────────────────────────────────────────

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

fn render_robot() -> String {
    let mut out = String::new();
    for (i, line) in ROBOT_LINES.iter().enumerate() {
        // Eye/mouth rows get the neon highlight; everything else lime green.
        let styled = if i == 2 || i == 3 {
            fg_bold(NEON, line)
        } else {
            fg_bold(PRIMARY, line)
        };
        out.push_str(&styled);
        if i + 1 < ROBOT_LINES.len() {
            out.push('\n');
        }
    }
    out
}

// ── Public render entry points ──────────────────────────────────────────────

fn short_id(id: &str) -> String {
    let chars: Vec<char> = id.chars().collect();
    if chars.len() > 8 {
        chars[..8].iter().collect()
    } else {
        id.to_string()
    }
}

pub fn render_banner(version: &str, session_id: &str, providers: i32, skills: i32) -> String {
    let robot = render_robot();

    let info_lines = vec![
        String::new(),
        format!("  {} {}", gradient_word("ASSISTCLAW"), fg(MUTED, version)),
        fg(PRIMARY, "  Edge Intelligence System"),
        String::new(),
        format!(
            "{}{}",
            fg(MUTED, "  session  "),
            fg_bold(PRIMARY, &short_id(session_id))
        ),
        format!(
            "{}{}",
            fg(MUTED, "  providers"),
            fg_bold(PRIMARY, &format!("  {providers}"))
        ),
        format!(
            "{}{}",
            fg(MUTED, "  skills   "),
            fg_bold(PRIMARY, &format!("  {skills}"))
        ),
        String::new(),
        fg(MUTED, "  Type your message, Enter to send"),
        fg(MUTED, "  ESC or Ctrl+C to quit"),
    ];
    let info = info_lines.join("\n");

    let combined = join_horizontal_top(&robot, &info);
    let banner = box_rounded(&combined, PRIMARY, 1);

    let tagline = format!(
        "{}{}\n  {}",
        fg(BORDER, "  "),
        cyber_bracket("AUTONOMOUS EDGE CORE"),
        scanline_suffix(56),
    );

    format!("\n{banner}\n{tagline}\n")
}

pub fn render_onboard_banner(version: &str) -> String {
    let robot = render_robot();

    let info_lines = vec![
        String::new(),
        format!("  {} {}", gradient_word("ASSISTCLAW"), fg(MUTED, version)),
        fg_bold(PRIMARY, "  Setup Wizard"),
        String::new(),
        fg(MUTED, "  Let's get you configured."),
        fg(MUTED, "  This takes about 2 minutes."),
        String::new(),
    ];
    let info = info_lines.join("\n");

    let combined = join_horizontal_top(&robot, &info);
    let banner = box_rounded(&combined, PRIMARY, 1);

    format!("\n{banner}\n")
}

pub fn render_cli_header(version: &str, term_width: i32) -> String {
    let line = gradient_word("ASSISTCLAW");
    let sub = fg(
        MUTED,
        &format!("  edge neural shell  ·  {}", version.trim()),
    );
    let bar_w = compute_bar_width(term_width);
    let bar = fg(BORDER, &format!("  {}▸", "─".repeat(bar_w)));
    format!("\n{line}\n{sub}\n{bar}\n")
}

fn compute_bar_width(term_width: i32) -> usize {
    if term_width <= 12 {
        return 48;
    }
    let w = (term_width as usize).saturating_sub(4);
    w.min(72)
}

pub fn render_version_block(version: &str) -> String {
    let line1 = format!(
        "{}  {}",
        gradient_word("ASSISTCLAW"),
        fg(MUTED, version)
    );
    let line2 = cyber_bracket("edge neural shell");
    let line3 = scanline_suffix(48);
    format!("{line1}\n{line2}\n{line3}\n")
}

// ── Tests ───────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn banner_contains_robot_and_version() {
        let out = render_banner("1.2.3", "abcdef1234", 4, 7);
        let stripped = String::from_utf8_lossy(&strip(out.as_bytes())).to_string();
        assert!(stripped.contains("ASSISTCLAW"));
        assert!(stripped.contains("1.2.3"));
        assert!(stripped.contains("abcdef12"));
        assert!(stripped.contains("│◉ ◉│"));
        assert!(stripped.contains("AUTONOMOUS EDGE CORE"));
    }

    #[test]
    fn onboard_banner_has_wizard_text() {
        let out = render_onboard_banner("0.0.1");
        let stripped = String::from_utf8_lossy(&strip(out.as_bytes())).to_string();
        assert!(stripped.contains("Setup Wizard"));
        assert!(stripped.contains("0.0.1"));
    }

    #[test]
    fn cli_header_clamps_bar_width() {
        let small = render_cli_header("v1", 0);
        let big = render_cli_header("v1", 9999);
        let small_stripped = String::from_utf8_lossy(&strip(small.as_bytes())).to_string();
        let big_stripped = String::from_utf8_lossy(&strip(big.as_bytes())).to_string();
        // Small fallback bar of 48, big should clamp to 72 cells of '─'.
        assert!(small_stripped.matches('─').count() >= 48);
        assert_eq!(big_stripped.matches('─').count(), 72);
    }

    #[test]
    fn version_block_renders_all_three_lines() {
        let out = render_version_block("dev");
        assert_eq!(out.matches('\n').count(), 3);
    }

    #[test]
    fn box_rounded_padding_matches_widest_line() {
        let content = "hello\nworld!!";
        let boxed = box_rounded(content, PRIMARY, 1);
        let stripped = String::from_utf8_lossy(&strip(boxed.as_bytes())).to_string();
        // widest = 7 ("world!!"), +2 padding = 9 dashes between corners
        assert!(stripped.contains(&format!("╭{}╮", "─".repeat(9))));
        assert!(stripped.contains(&format!("╰{}╯", "─".repeat(9))));
    }
}
