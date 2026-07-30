//! Lock-free event queue for Go ↔ Rust communication.
//!
//! Go polls `tui_poll_event()` which reads from this queue.
//! Rust writes events here from the TUI event loop thread.

use crate::event::TuiEvent;
use parking_lot::Mutex;
use std::collections::VecDeque;
use std::sync::Arc;

/// Thread-safe event queue shared between Rust TUI thread and Go poller.
#[derive(Clone)]
pub struct EventQueue {
    inner: Arc<Mutex<VecDeque<TuiEvent>>>,
}

impl Default for EventQueue {
    fn default() -> Self {
        Self {
            inner: Arc::new(Mutex::new(VecDeque::new())),
        }
    }
}

impl EventQueue {
    /// Push an event into the queue.
    pub fn push(&self, event: TuiEvent) {
        self.inner.lock().push_back(event);
    }

    /// Pop the oldest event from the queue.
    pub fn pop(&self) -> Option<TuiEvent> {
        self.inner.lock().pop_front()
    }

    /// Check if the queue is empty.
    pub fn is_empty(&self) -> bool {
        self.inner.lock().is_empty()
    }

    /// Clear all pending events.
    pub fn clear(&self) {
        self.inner.lock().clear();
    }
}

/// Global event queue instance.
static GLOBAL_QUEUE: std::sync::OnceLock<EventQueue> = std::sync::OnceLock::new();

/// Initialize the global event queue. Safe to call multiple times.
pub fn init_global_queue() {
    let _ = GLOBAL_QUEUE.set(EventQueue::default());
}

/// Get a clone of the global event queue.
pub fn global_queue() -> Option<EventQueue> {
    GLOBAL_QUEUE.get().cloned()
}
