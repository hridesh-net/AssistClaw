//! Responsive layout breakpoints and adaptive sizing.

/// Terminal width breakpoints.
pub enum Breakpoint {
    Compact,  // < 50 cols
    Narrow,   // 50-79 cols
    Normal,   // 80-119 cols
    Wide,     // >= 120 cols
}

impl From<u16> for Breakpoint {
    fn from(width: u16) -> Self {
        match width {
            0..=49 => Breakpoint::Compact,
            50..=79 => Breakpoint::Narrow,
            80..=119 => Breakpoint::Normal,
            _ => Breakpoint::Wide,
        }
    }
}

impl Breakpoint {
    pub fn chat_header_height(&self) -> u16 {
        match self {
            Breakpoint::Compact => 2,
            _ => 3,
        }
    }

    pub fn input_height(&self) -> u16 {
        match self {
            Breakpoint::Compact => 3,
            _ => 4,
        }
    }

    pub fn show_side_panel(&self) -> bool {
        matches!(self, Breakpoint::Wide)
    }

    pub fn padding(&self) -> u16 {
        match self {
            Breakpoint::Compact => 0,
            Breakpoint::Narrow => 1,
            _ => 2,
        }
    }
}
