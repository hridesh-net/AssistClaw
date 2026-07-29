//! AssistClaw TUI — Rust terminal UI library with C FFI exports.
//!
//! This library is designed to be linked into the Go binary via CGO.
//! All terminal rendering, input handling, and mouse interaction is
//! handled in Rust. Go provides business logic through a thin C boundary.

pub mod app;
pub mod event;
pub mod ffi;
pub mod queue;
pub mod render;
pub mod terminal;
pub mod theme;

pub mod screens {
    pub mod onboard;
    pub mod repl;
    pub mod skills;
    pub mod status;
}

pub mod widgets {
    pub mod banner;
    pub mod button;
    pub mod chat_bubble;
    pub mod checkbox;
    pub mod dropdown;
    pub mod input_box;
    pub mod progress_bar;
    pub mod sidebar;
    pub mod tooltip;
}

pub mod layout {
    pub mod responsive;
}
