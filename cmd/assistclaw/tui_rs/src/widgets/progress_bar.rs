//! Animated block progress bar.

use crate::theme::Theme;
use ratatui::style::Style;

pub struct ProgressBar {
    pub percent: f64,
    pub width: usize,
}

impl ProgressBar {
    pub fn new(percent: f64, width: usize) -> Self {
        Self { percent, width }
    }

    pub fn render(&self, theme: &Theme) -> String {
        let filled = ((self.percent / 100.0) * self.width as f64) as usize;
        let filled = filled.min(self.width);
        let _filled_style = Style::default().fg(theme.palette.primary);
        let _empty_style = Style::default().fg(theme.palette.border);

        let mut result = String::with_capacity(self.width);
        for i in 0..self.width {
            if i < filled {
                result.push('█');
            } else {
                result.push('░');
            }
        }
        result
    }

    pub fn render_with_label(&self, theme: &Theme, label: &str) -> String {
        let bar = self.render(theme);
        format!("{}  {}", bar, label)
    }
}
