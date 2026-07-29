//! Global application state and screen routing.

use crate::queue::EventQueue;
use crate::theme::Theme;
use parking_lot::Mutex;
use std::sync::Arc;

/// Shared mutable state for the REPL screen.
pub struct ReplState {
    /// (role, content) — role is one of "user", "agent", "meta", "error".
    pub history: Vec<(String, String)>,
    pub token_buffer: String,
    pub current_tool: String,
    pub thinking: bool,
    pub input_text: String,
    pub cursor_position: usize,
    pub scroll_offset: usize,
    pub pulse_frame: u64,
}

impl Default for ReplState {
    fn default() -> Self {
        Self {
            history: Vec::new(),
            token_buffer: String::new(),
            current_tool: String::new(),
            thinking: false,
            input_text: String::new(),
            cursor_position: 0,
            scroll_offset: 0,
            pulse_frame: 0,
        }
    }
}

/// Shared mutable state for the status screen.
#[derive(Default)]
pub struct StatusState {
    pub cpu_pct: f64,
    pub ram_mb: f64,
    pub ram_pct: f64,
    pub alive: bool,
    pub etime: String,
    /// Non-empty when the Go-side stats fetcher failed (process not found,
    /// /proc unreadable, ps unparseable, …). The dashboard surfaces this
    /// instead of showing misleading zeros.
    pub error: String,
    pub version: String,
    pub skill_summary: String,
    pub channels: Vec<String>,
    pub plano_enabled: bool,
    pub plano_endpoint: String,
    pub mcp_enabled: bool,
    pub mcp_transport: String,
}

/// Which screen is currently active.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Screen {
    Repl,
    Status,
    Onboard,
    Skills,
    Settings,
}

/// Global application state shared between the render thread and FFI callers.
pub struct AppState {
    pub theme: Theme,
    pub screen: Screen,
    pub running: bool,
    pub width: u16,
    pub height: u16,
    pub event_queue: EventQueue,
    pub repl: Mutex<ReplState>,
    pub status: Mutex<StatusState>,
    pub version: String,
    pub session_id: String,
}

impl AppState {
    pub fn new(version: String, session_id: String, event_queue: EventQueue) -> Self {
        Self {
            theme: Theme::default(),
            screen: Screen::Repl,
            running: true,
            width: 80,
            height: 24,
            event_queue,
            repl: Mutex::new(ReplState::default()),
            status: Mutex::new(StatusState::default()),
            version,
            session_id,
        }
    }
}

/// Global app state. Initialized when a screen starts.
static GLOBAL_APP: std::sync::RwLock<Option<Arc<Mutex<AppState>>>> = std::sync::RwLock::new(None);

pub fn init_app(version: String, session_id: String, queue: EventQueue) {
    let app = AppState::new(version, session_id, queue);
    let mut guard = GLOBAL_APP.write().unwrap();
    *guard = Some(Arc::new(Mutex::new(app)));
}

pub fn app() -> Option<Arc<Mutex<AppState>>> {
    GLOBAL_APP.read().unwrap().clone()
}

pub fn clear_app() {
    let mut guard = GLOBAL_APP.write().unwrap();
    *guard = None;
}
