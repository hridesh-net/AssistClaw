/* ═══════════════════════════════════════════════════════════
   AssistClaw Web UI — Client Application
   ═══════════════════════════════════════════════════════════ */

'use strict';

// ── State ────────────────────────────────────────────────────
const state = {
    token: '',
    sessionId: '',
    streaming: false,
    messages: [],   // {role, text, id}
};

// ── Init ─────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
    state.token = localStorage.getItem('ac_token') || '';
    state.sessionId = localStorage.getItem('ac_session') || generateId();
    saveSession();

    if (state.token) {
        hideAuthGate();
        fetchStatus();
    }

    // Enter key on token input
    document.getElementById('token-input').addEventListener('keydown', e => {
        if (e.key === 'Enter') submitAuth();
    });
});

// ── Auth ─────────────────────────────────────────────────────
function submitAuth() {
    const input = document.getElementById('token-input');
    const tok = input.value.trim();
    if (!tok) {
        showAuthError('Token is required.');
        return;
    }
    state.token = tok;
    localStorage.setItem('ac_token', tok);
    showAuthError('');
    hideAuthGate();
    fetchStatus();
}

function logout() {
    localStorage.removeItem('ac_token');
    state.token = '';
    document.getElementById('token-input').value = '';
    document.getElementById('auth-gate').classList.remove('hidden');
}

function showAuthError(msg) {
    document.getElementById('auth-error').textContent = msg;
}

function hideAuthGate() {
    document.getElementById('auth-gate').classList.add('hidden');
}

// ── Status ───────────────────────────────────────────────────
async function fetchStatus() {
    try {
        const res = await apiFetch('/api/status');
        if (!res.ok) { setOffline(); return; }
        const data = await res.json();
        setOnline(data);
    } catch {
        setOffline();
    }
}

function setOnline(data) {
    const dot = document.getElementById('status-dot');
    const text = document.getElementById('status-text');
    dot.className = 'dot online';
    text.textContent = 'Running (PID ' + (data.pid || '?') + ')';
    if (data.version) document.getElementById('version-badge').textContent = data.version;
    if (data.model) document.getElementById('model-chip').textContent = data.model;
}

function setOffline() {
    const dot = document.getElementById('status-dot');
    const text = document.getElementById('status-text');
    dot.className = 'dot offline';
    text.textContent = 'Unreachable';
}

// ── Send Message ─────────────────────────────────────────────
async function sendMessage() {
    if (state.streaming) return;
    const input = document.getElementById('msg-input');
    const text = input.value.trim();
    if (!text) return;

    input.value = '';
    autoResize(input);

    appendMessage('user', text);
    hideEmptyState();

    const typingId = appendTyping();
    state.streaming = true;
    setSendDisabled(true);

    const agentMsgId = 'msg-' + generateId();
    let agentText = '';
    let bubbleEl = null;
    let hasStartedBubble = false;

    try {
        const res = await apiFetch('/api/chat', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ message: text, session_id: state.sessionId }),
        });

        if (!res.ok) {
            removeTyping(typingId);
            const errText = await res.text();
            showToast('Error: ' + (errText || res.status), 'error');
            state.streaming = false;
            setSendDisabled(false);
            return;
        }

        // SSE streaming
        const reader = res.body.getReader();
        const dec = new TextDecoder();
        let buf = '';

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            buf += dec.decode(value, { stream: true });

            const lines = buf.split('\n');
            buf = lines.pop(); // keep incomplete line

            for (const line of lines) {
                if (!line.startsWith('data: ')) continue;
                const raw = line.slice(6);
                if (raw === '[DONE]') break;

                let evt;
                try { evt = JSON.parse(raw); } catch { continue; }

                if (evt.type === 'token') {
                    if (!hasStartedBubble) {
                        removeTyping(typingId);
                        bubbleEl = appendAgentBubble(agentMsgId);
                        hasStartedBubble = true;
                    }
                    agentText += evt.content;
                    renderBubble(bubbleEl, agentText);
                } else if (evt.type === 'tool_start') {
                    if (!hasStartedBubble) {
                        removeTyping(typingId);
                        bubbleEl = appendAgentBubble(agentMsgId);
                        hasStartedBubble = true;
                    }
                    appendToolCall(bubbleEl.parentElement.parentElement, evt.name, false);
                } else if (evt.type === 'tool_end') {
                    markToolCallDone(agentMsgId, evt.name);
                } else if (evt.type === 'error') {
                    showToast('Agent error: ' + evt.content, 'error');
                }
            }
        }
    } catch (err) {
        showToast('Connection error: ' + err.message, 'error');
    } finally {
        removeTyping(typingId);
        if (!hasStartedBubble && agentText) {
            bubbleEl = appendAgentBubble(agentMsgId);
            renderBubble(bubbleEl, agentText);
        }
        state.streaming = false;
        setSendDisabled(false);
        scrollToBottom();
    }
}

// ── Keyboard ─────────────────────────────────────────────────
function handleKey(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
    }
}

function autoResize(el) {
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 180) + 'px';
}

// ── DOM Helpers ───────────────────────────────────────────────
function hideEmptyState() {
    const es = document.getElementById('empty-state');
    if (es) es.remove();
}

function appendMessage(role, text) {
    const id = 'msg-' + generateId();
    const wrap = buildMsgEl(role, id);
    const bubble = wrap.querySelector('.msg-bubble');
    if (role === 'user') {
        bubble.textContent = text;
    } else {
        renderBubble(bubble, text);
    }
    document.getElementById('messages').appendChild(wrap);
    scrollToBottom();
    return id;
}

function appendAgentBubble(id) {
    const msgs = document.getElementById('messages');
    // check if already created
    let existing = document.getElementById(id);
    if (existing) return existing.querySelector('.msg-bubble');

    const wrap = buildMsgEl('agent', id);
    msgs.appendChild(wrap);
    scrollToBottom();
    return wrap.querySelector('.msg-bubble');
}

function buildMsgEl(role, id) {
    const avatar = role === 'user' ? '🙂' : '🦅';
    const wrap = document.createElement('div');
    wrap.className = 'msg ' + role;
    wrap.id = id;
    wrap.innerHTML = `
    <div class="msg-avatar">${avatar}</div>
    <div class="msg-body"><div class="msg-bubble"></div></div>
  `;
    return wrap;
}

function renderBubble(el, text) {
    // Basic markdown rendering (no external lib dependency)
    let html = escHtml(text)
        // code blocks
        .replace(/```([\s\S]*?)```/g, '<pre><code>$1</code></pre>')
        // inline code
        .replace(/`([^`]+)`/g, '<code>$1</code>')
        // bold
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        // italic
        .replace(/\*(.+?)\*/g, '<em>$1</em>')
        // lines → paragraphs (split on double newline)
        .split('\n\n')
        .map(p => '<p>' + p.replace(/\n/g, '<br>') + '</p>')
        .join('');
    el.innerHTML = html;
    scrollToBottom();
}

function appendTyping(id) {
    const mid = 'typing-' + generateId();
    const wrap = document.createElement('div');
    wrap.className = 'msg agent';
    wrap.id = mid;
    wrap.innerHTML = `
    <div class="msg-avatar">🦅</div>
    <div class="msg-body"><div class="typing-dots"><span></span><span></span><span></span></div></div>
  `;
    document.getElementById('messages').appendChild(wrap);
    scrollToBottom();
    return mid;
}

function removeTyping(id) {
    const el = document.getElementById(id);
    if (el) el.remove();
}

function appendToolCall(msgWrap, name, done) {
    const tc = document.createElement('div');
    tc.className = 'tool-call' + (done ? ' done' : '');
    tc.id = 'tc-' + name.replace(/\W/g, '_');
    tc.innerHTML = `⚙ <span>${escHtml(name)}</span>`;
    const body = msgWrap.querySelector('.msg-body');
    if (body) body.insertBefore(tc, body.firstChild);
}

function markToolCallDone(msgId, name) {
    const tc = document.getElementById('tc-' + name.replace(/\W/g, '_'));
    if (tc) tc.classList.add('done');
}

function setSendDisabled(v) {
    document.getElementById('send-btn').disabled = v;
}

function scrollToBottom() {
    const msgs = document.getElementById('messages');
    msgs.scrollTop = msgs.scrollHeight;
}

// ── Session Actions ───────────────────────────────────────────
function clearChat() {
    const msgs = document.getElementById('messages');
    msgs.innerHTML = '';
    const es = document.createElement('div');
    es.className = 'empty-state';
    es.id = 'empty-state';
    es.innerHTML = '<div class="big-icon">🦅</div><h2>AssistClaw</h2><p>Your autonomous AI agent is ready. Start chatting below.</p>';
    msgs.appendChild(es);
}

function newSession() {
    state.sessionId = generateId();
    saveSession();
    clearChat();
    showToast('New session started.');
}

function saveSession() {
    localStorage.setItem('ac_session', state.sessionId);
    document.getElementById('session-hint').textContent = 'Session: ' + state.sessionId.slice(0, 8) + '…';
}

function copySession() {
    navigator.clipboard.writeText(state.sessionId).then(() => showToast('Session ID copied!'));
}

// ── Toast ─────────────────────────────────────────────────────
function showToast(msg, type) {
    const area = document.getElementById('toast-area');
    const t = document.createElement('div');
    t.className = 'toast' + (type === 'error' ? ' error' : '');
    t.textContent = msg;
    area.appendChild(t);
    setTimeout(() => t.remove(), 4000);
}

// ── Utilities ─────────────────────────────────────────────────
function generateId() {
    return Math.random().toString(36).slice(2, 10) + Date.now().toString(36);
}

function escHtml(str) {
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

async function apiFetch(path, opts = {}) {
    const headers = { ...(opts.headers || {}) };
    if (state.token) headers['Authorization'] = 'Bearer ' + state.token;
    return fetch(path, { ...opts, headers });
}
