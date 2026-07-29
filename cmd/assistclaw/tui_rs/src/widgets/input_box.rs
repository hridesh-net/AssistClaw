//! Multi-line input box widget.

pub struct InputBox {
    pub text: String,
    pub cursor: usize,
    pub placeholder: String,
    pub focused: bool,
    pub max_height: usize,
}

impl InputBox {
    pub fn new(placeholder: impl Into<String>) -> Self {
        Self {
            text: String::new(),
            cursor: 0,
            placeholder: placeholder.into(),
            focused: true,
            max_height: 3,
        }
    }

    pub fn insert(&mut self, c: char) {
        self.text.insert(self.cursor, c);
        self.cursor += 1;
    }

    pub fn backspace(&mut self) {
        if self.cursor > 0 {
            self.cursor -= 1;
            self.text.remove(self.cursor);
        }
    }

    pub fn delete(&mut self) {
        if self.cursor < self.text.len() {
            self.text.remove(self.cursor);
        }
    }

    pub fn move_left(&mut self) {
        if self.cursor > 0 {
            self.cursor -= 1;
        }
    }

    pub fn move_right(&mut self) {
        if self.cursor < self.text.len() {
            self.cursor += 1;
        }
    }

    pub fn display_text(&self) -> &str {
        if self.text.is_empty() && !self.focused {
            &self.placeholder
        } else {
            &self.text
        }
    }

    pub fn lines(&self, width: usize) -> Vec<String> {
        let text = self.display_text();
        if text.is_empty() {
            return vec![String::new()];
        }
        textwrap::wrap(text, width)
            .into_iter()
            .map(|s| s.to_string())
            .collect()
    }
}
