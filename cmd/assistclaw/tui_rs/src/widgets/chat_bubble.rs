//! Message bubble widget for chat display.

use ratatui::{
    style::Style,
    text::{Line, Span},
};

pub struct ChatBubble<'a> {
    pub content: &'a str,
    pub is_user: bool,
}

impl<'a> ChatBubble<'a> {
    pub fn lines(&self, user_style: Style, agent_style: Style, width: usize) -> Vec<Line> {
        let prefix = if self.is_user {
            Span::styled("You › ", user_style)
        } else {
            Span::styled("🤖 ", agent_style)
        };

        let wrapped_lines: Vec<String> = textwrap::wrap(self.content, width.saturating_sub(6))
            .into_iter()
            .map(|s| s.to_string())
            .collect();
        
        wrapped_lines
            .into_iter()
            .enumerate()
            .map(|(i, line)| {
                if i == 0 {
                    Line::from(vec![prefix.clone(), Span::raw(line)])
                } else {
                    Line::from(vec![Span::raw("      "), Span::raw(line)])
                }
            })
            .collect()
    }
}
