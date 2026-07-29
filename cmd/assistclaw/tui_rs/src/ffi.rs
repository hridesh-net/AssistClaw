//! C FFI exports — thin C ABI boundary for CGO.
//!
//! All functions here are `extern "C"` and use C-compatible types.
//! Complex data is passed as JSON strings.

use crate::app::{app, clear_app, init_app};
use crate::queue::{global_queue, init_global_queue};
use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_int};

// ── Helpers ───────────────────────────────────────────────────────────────

/// Convert a C string pointer to a Rust `&str`.
/// Returns an empty string if the pointer is null or invalid UTF-8.
unsafe fn c_str_to_str(ptr: *const c_char) -> &'static str {
    if ptr.is_null() {
        return "";
    }
    unsafe { CStr::from_ptr(ptr).to_str().unwrap_or("") }
}

/// Convert a Rust `String` to a C string pointer.
/// The caller **must** free this with `tui_free_string`.
fn rust_str_to_c(s: String) -> *mut c_char {
    match CString::new(s) {
        Ok(c) => c.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}

// ── Lifecycle ─────────────────────────────────────────────────────────────

/// Initialize the TUI runtime. Must be called once before any other function.
/// Returns 0 on success, non-zero on error.
#[unsafe(no_mangle)]
pub extern "C" fn tui_init() -> c_int {
    init_global_queue();
    0
}

/// Shutdown the TUI runtime and free global resources.
#[unsafe(no_mangle)]
pub extern "C" fn tui_shutdown() {
    clear_app();
}

// ── REPL Screen ───────────────────────────────────────────────────────────

/// Start the REPL TUI. Non-blocking; spawns a Rust thread.
/// `config_json` contains: version, session_id, provider_count, skill_count,
/// and optionally `enable_mouse` (defaults to auto: off inside tmux/screen).
/// Returns 0 on success.
#[unsafe(no_mangle)]
pub extern "C" fn tui_repl_start(config_json: *const c_char) -> c_int {
    let config = unsafe { c_str_to_str(config_json) };

    let parsed: serde_json::Value = match serde_json::from_str(config) {
        Ok(v) => v,
        Err(_) => return 1,
    };

    let version = parsed["version"]
        .as_str()
        .unwrap_or("unknown")
        .to_string();
    let session_id = parsed["session_id"]
        .as_str()
        .unwrap_or("")
        .to_string();

    // Mouse: explicit config value wins; otherwise auto-detect multiplexer.
    let enable_mouse = match parsed["enable_mouse"].as_bool() {
        Some(b) => b,
        None => !crate::terminal::detect_multiplexer(),
    };

    let queue = match global_queue() {
        Some(q) => q,
        None => return 2,
    };

    init_app(version, session_id, queue.clone());

    // Spawn the TUI event loop in a dedicated thread.
    std::thread::spawn(move || {
        if let Err(e) = crate::screens::repl::run(queue, enable_mouse) {
            eprintln!("[tui_rs] REPL error: {e}");
        }
    });

    0
}

/// Stop the REPL TUI. Signals the Rust thread to exit.
#[unsafe(no_mangle)]
pub extern "C" fn tui_repl_stop() {
    if let Some(a) = app() {
        a.lock().running = false;
    }
    clear_app();
}

/// Send a token to the REPL chat viewport.
/// An empty token marks the start of a new agent run: it clears any stale
/// streaming buffer and turns the thinking indicator on.
#[unsafe(no_mangle)]
pub extern "C" fn tui_repl_send_token(token: *const c_char) {
    let text = unsafe { c_str_to_str(token) };
    if let Some(a) = app() {
        let app = a.lock();
        let mut repl = app.repl.lock();
        if text.is_empty() {
            repl.token_buffer.clear();
        } else {
            repl.token_buffer.push_str(text);
        }
        repl.thinking = true;
    }
}

/// Send a tool-call indicator.
#[unsafe(no_mangle)]
pub extern "C" fn tui_repl_send_tool(name: *const c_char, _input: *const c_char) {
    let tool_name = unsafe { c_str_to_str(name) };
    if let Some(a) = app() {
        let app = a.lock();
        let mut repl = app.repl.lock();
        repl.current_tool = tool_name.to_string();
        repl.thinking = true;
    }
}

/// Signal agent finished successfully. Moves the streamed text into history
/// so the response survives the end of the run.
#[unsafe(no_mangle)]
pub extern "C" fn tui_repl_send_done(iterations: c_int, total_tokens: c_int) {
    if let Some(a) = app() {
        let app = a.lock();
        let mut repl = app.repl.lock();
        repl.thinking = false;
        if !repl.token_buffer.is_empty() {
            let text = std::mem::take(&mut repl.token_buffer);
            repl.history.push(("agent".to_string(), text));
        }
        repl.current_tool.clear();
        repl.history.push((
            "meta".to_string(),
            format!("▸ {} iter · {} tokens", iterations, fmt_num(total_tokens as usize)),
        ));
    }
}

/// Signal agent error. Preserves any partial streamed text, then shows the error.
#[unsafe(no_mangle)]
pub extern "C" fn tui_repl_send_error(message: *const c_char) {
    let msg = unsafe { c_str_to_str(message) };
    if let Some(a) = app() {
        let app = a.lock();
        let mut repl = app.repl.lock();
        repl.thinking = false;
        if !repl.token_buffer.is_empty() {
            let text = std::mem::take(&mut repl.token_buffer);
            repl.history.push(("agent".to_string(), text));
        }
        repl.current_tool.clear();
        repl.history.push(("error".to_string(), msg.to_string()));
    }
}

// ── Status Dashboard ──────────────────────────────────────────────────────

/// Start the status dashboard. Blocks until user quits.
/// `info_json` may include an optional `enable_mouse` boolean field; the
/// default auto-detects multiplexer environments.
#[unsafe(no_mangle)]
pub extern "C" fn tui_status_run(info_json: *const c_char) -> c_int {
    let info = unsafe { c_str_to_str(info_json) };
    let queue = match global_queue() {
        Some(q) => q,
        None => return 1,
    };
    let parsed = serde_json::from_str::<serde_json::Value>(info).ok();
    let enable_mouse = parsed
        .as_ref()
        .and_then(|v| v["enable_mouse"].as_bool())
        .unwrap_or_else(|| !crate::terminal::detect_multiplexer());
    let version = parsed
        .as_ref()
        .and_then(|v| v["version"].as_str())
        .unwrap_or("")
        .to_string();

    // The dashboard reads live metrics from the shared AppState, which
    // tui_status_update writes into — it must exist before the loop starts
    // (the REPL path initializes it in tui_repl_start; status runs standalone).
    init_app(version, String::new(), queue.clone());

    let code = match crate::screens::status::run(info, queue, enable_mouse) {
        Ok(()) => 0,
        Err(e) => {
            eprintln!("[tui_rs] Status error: {e}");
            2
        }
    };
    clear_app();
    code
}

/// Send a status update while the dashboard is running.
#[unsafe(no_mangle)]
pub extern "C" fn tui_status_update(status_json: *const c_char) {
    let status = unsafe { c_str_to_str(status_json) };
    if let Ok(parsed) = serde_json::from_str::<serde_json::Value>(status) {
        if let Some(a) = app() {
            let app = a.lock();
            let mut st = app.status.lock();
            st.cpu_pct = parsed["cpu_pct"].as_f64().unwrap_or(0.0);
            st.ram_mb = parsed["ram_mb"].as_f64().unwrap_or(0.0);
            st.ram_pct = parsed["ram_pct"].as_f64().unwrap_or(0.0);
            st.alive = parsed["alive"].as_bool().unwrap_or(false);
            if let Some(et) = parsed["etime"].as_str() {
                st.etime = et.to_string();
            }
            // `error` is present only when Go's stats fetcher failed. Clear
            // it on a healthy update so transient failures don't stick.
            st.error = parsed["error"].as_str().unwrap_or("").to_string();
        }
    }
}

// ── Onboarding Wizard ─────────────────────────────────────────────────────

/// Run the onboarding wizard. Blocks until completed or cancelled.
/// Returns a JSON string with the new config (caller must free with tui_free_string).
/// Returns NULL if cancelled.
#[unsafe(no_mangle)]
pub extern "C" fn tui_onboard_run(config_json: *const c_char) -> *mut c_char {
    let config = unsafe { c_str_to_str(config_json) };
    let queue = match global_queue() {
        Some(q) => q,
        None => return std::ptr::null_mut(),
    };

    match crate::screens::onboard::run(config, queue) {
        Some(result) => rust_str_to_c(result),
        None => std::ptr::null_mut(),
    }
}

// ── Skills Configuration ──────────────────────────────────────────────────

/// Run the skills configurator. Blocks until completed or cancelled.
/// Returns a JSON string with selected skills (caller must free with tui_free_string).
/// Returns NULL if cancelled.
#[unsafe(no_mangle)]
pub extern "C" fn tui_skills_run(
    catalog_json: *const c_char,
    current_skills_json: *const c_char,
) -> *mut c_char {
    let catalog = unsafe { c_str_to_str(catalog_json) };
    let current = unsafe { c_str_to_str(current_skills_json) };
    let queue = match global_queue() {
        Some(q) => q,
        None => return std::ptr::null_mut(),
    };

    match crate::screens::skills::run(catalog, current) {
        Some(result) => rust_str_to_c(serde_json::to_string(&result).unwrap_or_else(|_| "[]".to_string())),
        None => std::ptr::null_mut(),
    }
}

// ── Event Polling ─────────────────────────────────────────────────────────

/// Poll for the next TUI event. Returns NULL if no event pending.
/// Caller must free the returned string with `tui_free_string`.
#[unsafe(no_mangle)]
pub extern "C" fn tui_poll_event() -> *mut c_char {
    let queue = match global_queue() {
        Some(q) => q,
        None => return std::ptr::null_mut(),
    };

    match queue.pop() {
        Some(event) => rust_str_to_c(event.to_json()),
        None => std::ptr::null_mut(),
    }
}

// ── One-shot Renderers (banner, version, CLI header) ─────────────────────

/// Render the full splash banner (robot + info pane + tagline).
/// Caller must free the returned string with `tui_free_string`.
#[unsafe(no_mangle)]
pub extern "C" fn tui_render_banner(
    version: *const c_char,
    session_id: *const c_char,
    providers: c_int,
    skills: c_int,
) -> *mut c_char {
    let v = unsafe { c_str_to_str(version) };
    let s = unsafe { c_str_to_str(session_id) };
    rust_str_to_c(crate::render::render_banner(v, s, providers, skills))
}

/// Render the shorter onboarding banner.
#[unsafe(no_mangle)]
pub extern "C" fn tui_render_onboard_banner(version: *const c_char) -> *mut c_char {
    let v = unsafe { c_str_to_str(version) };
    rust_str_to_c(crate::render::render_onboard_banner(v))
}

/// Render the compact CLI header for stderr at process start.
/// `term_width` is the current terminal width in columns; pass 0 for default.
#[unsafe(no_mangle)]
pub extern "C" fn tui_render_cli_header(
    version: *const c_char,
    term_width: c_int,
) -> *mut c_char {
    let v = unsafe { c_str_to_str(version) };
    rust_str_to_c(crate::render::render_cli_header(v, term_width))
}

/// Render the styled `version` command output.
#[unsafe(no_mangle)]
pub extern "C" fn tui_render_version_block(version: *const c_char) -> *mut c_char {
    let v = unsafe { c_str_to_str(version) };
    rust_str_to_c(crate::render::render_version_block(v))
}

/// Free a string returned by any Rust function.
#[unsafe(no_mangle)]
pub extern "C" fn tui_free_string(s: *mut c_char) {
    if !s.is_null() {
        unsafe {
            let _ = CString::from_raw(s);
        }
    }
}

// ── Helpers ───────────────────────────────────────────────────────────────

fn fmt_num(n: usize) -> String {
    let s = n.to_string();
    if s.len() <= 3 {
        return s;
    }
    let mut result = String::new();
    for (i, c) in s.chars().enumerate() {
        if i > 0 && (s.len() - i) % 3 == 0 {
            result.push(',');
        }
        result.push(c);
    }
    result
}
