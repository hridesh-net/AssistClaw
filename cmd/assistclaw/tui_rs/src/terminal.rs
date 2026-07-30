//! Terminal lifecycle safety.
//!
//! Owns the raw-mode + alternate-screen + mouse-capture state for every TUI
//! screen and guarantees the terminal is restored even on:
//!   • normal exit (Drop)
//!   • Rust panic (panic hook calls restore)
//!   • SIGTERM / SIGINT / SIGHUP (signal-handler thread calls restore)
//!
//! Without this, an abnormal exit leaves the user's shell in raw mode —
//! typing is invisible, the cursor is hidden, and mouse events get dumped
//! as garbage. This module is the only place that should enter or leave
//! raw mode from the Rust TUI.

use crossterm::{
    cursor::Show,
    event::{DisableMouseCapture, EnableMouseCapture},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use parking_lot::Mutex;
use signal_hook::consts::{SIGHUP, SIGINT, SIGTERM};
use signal_hook::iterator::Signals;
use std::io::{self, IsTerminal, Write};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::sync::OnceLock;

static HOOKS_INSTALLED: OnceLock<()> = OnceLock::new();

/// Tracks whether a guard is currently holding raw mode. Multiple guards
/// across screens are serialised — only one can be active. The signal
/// handler / panic hook consult this flag to decide whether to clean up.
static ACTIVE: AtomicBool = AtomicBool::new(false);

/// Whether the active guard enabled mouse capture (so we know whether to
/// emit the matching disable on cleanup).
static MOUSE_ON: AtomicBool = AtomicBool::new(false);

/// Errors logged by the signal-handler thread, surfaced for diagnostics.
static SIGNAL_LOG: OnceLock<Mutex<Vec<String>>> = OnceLock::new();

fn signal_log() -> &'static Mutex<Vec<String>> {
    SIGNAL_LOG.get_or_init(|| Mutex::new(Vec::new()))
}

/// One-time installation of the panic hook and signal-handler thread.
/// Safe to call multiple times; only runs once.
fn install_hooks() {
    HOOKS_INSTALLED.get_or_init(|| {
        // Panic hook: restore terminal, then defer to the previous hook so
        // the panic message is still printed.
        let previous = std::panic::take_hook();
        std::panic::set_hook(Box::new(move |info| {
            force_restore();
            previous(info);
        }));

        // Signal-handler thread: blocks on SIGTERM/SIGINT/SIGHUP, restores
        // the terminal, then re-raises the signal with the default action
        // so the process exits with the conventional status.
        if let Ok(mut signals) = Signals::new([SIGTERM, SIGINT, SIGHUP]) {
            std::thread::Builder::new()
                .name("assistclaw-tui-signal".into())
                .spawn(move || {
                    if let Some(sig) = signals.forever().next() {
                        force_restore();
                        signal_log()
                            .lock()
                            .push(format!("signal {sig} received — terminal restored"));
                        // Replace the handler with the default and re-raise.
                        // SAFETY: signal() is the standard exit path; the
                        // restored terminal is now safe to discard.
                        let _ = signal_hook::low_level::emulate_default_handler(sig);
                    }
                })
                .ok();
        }
    });
}

/// Drain any messages the signal-handler thread left behind, for the host
/// process to log. Returns an empty vec if nothing was logged.
pub fn take_signal_log() -> Vec<String> {
    let mut guard = signal_log().lock();
    std::mem::take(&mut *guard)
}

/// Restore the terminal unconditionally. Idempotent. Called from Drop,
/// the panic hook, and the signal-handler thread, so it must not panic.
fn force_restore() {
    if !ACTIVE.swap(false, Ordering::SeqCst) {
        return;
    }
    let mut stdout = io::stdout();
    if MOUSE_ON.swap(false, Ordering::SeqCst) {
        let _ = execute!(stdout, DisableMouseCapture);
    }
    let _ = execute!(stdout, LeaveAlternateScreen, Show);
    let _ = disable_raw_mode();
    let _ = stdout.flush();
}

/// RAII guard for the alt-screen / raw-mode / mouse-capture state.
///
/// Construct one with [`TerminalGuard::enter`] at the top of every screen
/// `run` function. Its [`Drop`] guarantees the terminal is restored when
/// the function returns — even via `?`, panic, or signal.
pub struct TerminalGuard {
    cleaned: bool,
}

impl TerminalGuard {
    /// Enter raw mode + alternate screen, optionally enabling mouse capture.
    ///
    /// If the terminal was already in raw mode (a previous abnormal exit),
    /// reset it first so we start from a known state.
    pub fn enter(enable_mouse: bool) -> io::Result<Self> {
        install_hooks();

        // Defensive: if a stale guard somehow left raw mode active, clean
        // it up before claiming ownership.
        if ACTIVE.load(Ordering::SeqCst) {
            force_restore();
        }
        if crossterm::terminal::is_raw_mode_enabled().unwrap_or(false) {
            // The terminal is in raw mode but no guard owns it. Reset.
            let _ = disable_raw_mode();
        }

        enable_raw_mode()?;
        let mut stdout = io::stdout();
        execute!(stdout, EnterAlternateScreen)?;
        if enable_mouse {
            execute!(stdout, EnableMouseCapture)?;
            MOUSE_ON.store(true, Ordering::SeqCst);
        }
        ACTIVE.store(true, Ordering::SeqCst);

        Ok(Self { cleaned: false })
    }

    /// Restore the terminal explicitly. After this returns, `Drop` is a no-op.
    pub fn restore(&mut self) {
        if self.cleaned {
            return;
        }
        self.cleaned = true;
        force_restore();
    }
}

impl Drop for TerminalGuard {
    fn drop(&mut self) {
        if !self.cleaned {
            force_restore();
        }
    }
}

/// True if stdout is attached to a real terminal. Helper for callers that
/// want to refuse to enter raw mode when piped.
pub fn stdout_is_tty() -> bool {
    io::stdout().is_terminal()
}

/// True if the user's environment suggests a multiplexer where mouse
/// capture is usually broken (tmux, screen, GNU screen, nested ssh).
/// Callers can pass this into `TerminalGuard::enter(!detect_multiplexer())`
/// or honour a config flag.
pub fn detect_multiplexer() -> bool {
    std::env::var("TMUX").is_ok()
        || std::env::var("STY").is_ok() // GNU screen
        || matches!(std::env::var("TERM").ok().as_deref(), Some(t) if t.starts_with("screen") || t.starts_with("tmux"))
}

/// Shared wrapper so non-Rust callers (the Go bridge) can request a force-
/// restore explicitly — e.g. if Go detects its own SIGTERM before Rust does.
pub fn shared_force_restore() -> Arc<dyn Fn() + Send + Sync> {
    Arc::new(force_restore)
}
