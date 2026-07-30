//! Expandable dropdown widget.

pub struct Dropdown {
    pub options: Vec<String>,
    pub selected: usize,
    pub expanded: bool,
    pub hovered: bool,
}

impl Dropdown {
    pub fn new(options: Vec<String>) -> Self {
        Self {
            options,
            selected: 0,
            expanded: false,
            hovered: false,
        }
    }

    pub fn render(&self) -> String {
        let current = self.options.get(self.selected).map(|s| s.as_str()).unwrap_or("");
        let arrow = if self.expanded { "▲" } else { "▼" };
        if self.hovered {
            format!("> {} {}", current, arrow)
        } else {
            format!("  {} {}", current, arrow)
        }
    }

    pub fn render_expanded(&self) -> Vec<String> {
        self.options
            .iter()
            .enumerate()
            .map(|(i, opt)| {
                let marker = if i == self.selected { "► " } else { "  " };
                format!("{}{}", marker, opt)
            })
            .collect()
    }
}
