//! Hover tooltip overlay widget.

pub struct Tooltip {
    pub text: String,
    pub visible: bool,
    pub x: u16,
    pub y: u16,
}

impl Tooltip {
    pub fn new(text: impl Into<String>) -> Self {
        Self {
            text: text.into(),
            visible: false,
            x: 0,
            y: 0,
        }
    }

    pub fn show(&mut self, x: u16, y: u16) {
        self.visible = true;
        self.x = x;
        self.y = y;
    }

    pub fn hide(&mut self) {
        self.visible = false;
    }

    pub fn lines(&self, max_width: usize) -> Vec<String> {
        textwrap::wrap(&self.text, max_width)
            .into_iter()
            .map(|s| s.to_string())
            .collect()
    }
}
