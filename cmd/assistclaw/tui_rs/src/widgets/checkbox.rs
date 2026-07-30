//! Clickable checkbox widget.

pub struct Checkbox {
    pub checked: bool,
    pub label: String,
    pub hovered: bool,
}

impl Checkbox {
    pub fn new(label: impl Into<String>) -> Self {
        Self {
            checked: false,
            label: label.into(),
            hovered: false,
        }
    }

    pub fn render(&self) -> String {
        let symbol = if self.checked { "☑" } else { "☐" };
        if self.hovered {
            format!("> {} {}", symbol, self.label)
        } else {
            format!("  {} {}", symbol, self.label)
        }
    }
}
