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
    if (!text) return;

    // ── Pattern 1: Info message from formatMailInfo ───────────────
    // Format: ## 🦅 AssistClaw — Email Review\n**Ready to send** ...\n**📬 EMAIL DETAILS**\n```\nFrom    │ ...\nTo      │ ...\nSubject │ ...\n```\n**✨ AI SUMMARY**\n> ...
    if (text.includes('AssistClaw — Email Review')) {
        const fromM = text.match(/From\s*│\s*(.+)/);
        const toM = text.match(/To\s*│\s*(.+)/);
        const subjectM = text.match(/Subject\s*│\s*(.+)/);
        const summaryM = text.match(/\*\*✨ AI SUMMARY\*\*\s*\n>\s*([\s\S]+?)(?:\n\n|$)/);
        const statusM = text.match(/\*\*([^*]+)\*\*\s*`\|`\s*\*\*([^*]+)\*\*/);

        const from = fromM ? fromM[1].trim() : '—';
        const to = toM ? toM[1].trim() : '—';
        const subject = subjectM ? subjectM[1].trim() : '—';
        const summary = summaryM ? summaryM[1].trim() : '—';
        const status = statusM ? statusM[1].trim() : 'Ready to send';
        const prio = statusM ? statusM[2].trim() : 'Medium priority';
        const now = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

        el.innerHTML = `
        <div class="ac-info-card">
          <div class="ac-card-header">
            <div class="ac-card-header-left">
              <div class="ac-bot-avatar">🦅</div>
              <div>
                <div class="ac-card-title">AssistClaw <span class="ac-card-sub">Email Review Agent</span></div>
                <div class="ac-card-time">${now}</div>
              </div>
            </div>
            <div class="ac-card-badges">
              <span class="ac-badge ac-badge-green">${escHtml(status)}</span>
              <span class="ac-badge">${escHtml(prio)}</span>
            </div>
          </div>

          <div class="ac-card-section">
            <div class="ac-section-label">Email Details</div>
            <div class="ac-detail-grid">
              <div class="ac-detail-item">
                <span class="ac-detail-key">From</span>
                <span class="ac-detail-val">${escHtml(from)}</span>
              </div>
              <div class="ac-detail-item">
                <span class="ac-detail-key">To</span>
                <span class="ac-detail-val">${escHtml(to)}</span>
              </div>
              <div class="ac-detail-item ac-detail-full">
                <span class="ac-detail-key">Subject</span>
                <span class="ac-detail-val">${escHtml(subject)}</span>
              </div>
            </div>
          </div>

          <div class="ac-card-section">
            <div class="ac-section-label">AI Summary</div>
            <div class="ac-summary-box">${escHtml(summary)}</div>
          </div>

          <div class="ac-card-footer-note">📋 Draft reply and action buttons will appear in the next message below.</div>
        </div>
        `;
        scrollToBottom();
        return;
    }

    // ── Pattern 2: Draft message from formatMailDraft ─────────────
    // Format: **✍️ DRAFT REPLY** ...\n```\n<body>\n```\n\n**⚡ ACTIONS** *(Token: `<tok>`)*
    if (text.includes('DRAFT REPLY') && text.includes('⚡ ACTIONS')) {
        const draftM = text.match(/```\n([\s\S]*?)```/);
        const tokenM = text.match(/Token:\s*`([a-zA-Z0-9]+)`/);
        const toneM = text.match(/\*\*DRAFT REPLY\*\*\s*\*\(([^)]+)\)\*/);

        const draft = draftM ? draftM[1].trim() : '';
        const token = tokenM ? tokenM[1] : '';
        const tone = toneM ? toneM[1] : 'Professional · English';
        const now = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

        el.innerHTML = `
        <div class="ac-draft-card" id="ew-${token}">
          <div class="ac-card-header">
            <div class="ac-card-header-left">
              <div class="ac-bot-avatar" style="background:linear-gradient(135deg,#66b2ff,#3a70cc)">✍️</div>
              <div>
                <div class="ac-card-title">Draft Reply <span class="ac-card-sub">${escHtml(tone)}</span></div>
                <div class="ac-card-time">${now} · Token: <code class="ac-token-code">${escHtml(token)}</code></div>
              </div>
            </div>
            <span class="ac-badge ac-badge-blue">Pending Review</span>
          </div>

          <div class="ac-card-section">
            <div class="ac-draft-toolbar">
              <div class="ac-section-label" style="margin:0">Draft Content</div>
              <div class="ac-toolbar-right">
                <select class="ac-select" onchange="triggerRegenerateTone('${token}', this.value)" title="Change tone">
                  <option value="" disabled selected>Tone</option>
                  <option value="Make it Highly Professional">Professional</option>
                  <option value="Make it Friendly and Warm">Friendly</option>
                  <option value="Make it Formal and respectful">Formal</option>
                  <option value="Make it extremely concise">Concise</option>
                  <option value="Make it more detailed and thorough">Detailed</option>
                </select>
                <select class="ac-select" onchange="triggerTranslate('${token}', this.value)" title="Translate">
                  <option value="" disabled selected>Language</option>
                  <option value="Spanish">Spanish</option>
                  <option value="French">French</option>
                  <option value="German">German</option>
                  <option value="Japanese">Japanese</option>
                  <option value="Hindi">Hindi</option>
                </select>
                <button class="ac-toolbar-btn" onclick="copyToClipboard('ac-draft-${token}')" title="Copy draft">📋 Copy</button>
              </div>
            </div>
            <textarea class="ac-draft-textarea" id="ac-draft-${token}">${escHtml(draft)}</textarea>
          </div>

          <div class="ac-card-section">
            <div class="ac-section-label">⚡ Quick Intent Tags</div>
            <div class="ac-tag-row">
              <button class="ac-tag" onclick="injectTone('${token}', 'Make it more concise and professional')">✂️ Shorten</button>
              <button class="ac-tag" onclick="injectTone('${token}', 'Add a polite follow-up reminder')">📌 Follow-up</button>
              <button class="ac-tag" onclick="injectTone('${token}', 'Express gratitude and appreciation')">🙏 Gratitude</button>
              <button class="ac-tag" onclick="injectTone('${token}', 'Add urgency and deadline awareness')">⏰ Urgency</button>
              <button class="ac-tag" onclick="injectTone('${token}', 'Make it more friendly and warm')">😊 Friendly</button>
            </div>
          </div>

          <div class="ac-actions-bar">
            <button class="ac-action-btn ac-btn-approve" onclick="submitEwAction('approve', '${token}')">
              <span>✅</span> Approve &amp; Send
            </button>
            <button class="ac-action-btn ac-btn-edit" onclick="submitEwAction('edit', '${token}')">
              <span>✏️</span> Save Edit
            </button>
            <button class="ac-action-btn ac-btn-regen" onclick="openRegenerateModal('${token}')">
              <span>🔄</span> Regenerate
            </button>
            <button class="ac-action-btn ac-btn-reject" onclick="submitEwAction('reject', '${token}')">
              <span>❌</span> Reject
            </button>
          </div>

          <div class="ac-card-footer-note">🔒 AssistClaw logs every action. Approvals are reversible within 5 minutes.</div>
        </div>
        `;
        scrollToBottom();
        return;
    }

    // ── Pattern 3: Updated draft (formatMailPost) ─────────────────
    // Format: **🔄 Updated Draft** — *<subject>* (token: `<tok>`)
    if (text.includes('🔄 Updated Draft')) {
        const tokenM = text.match(/token:\s*`([a-zA-Z0-9]+)`/);
        const subjectM = text.match(/Updated Draft\*\*\s*—\s*\*([^*]+)\*/);
        const summaryM = text.match(/\*\*✨ Summary\*\*\s*\n>\s*(.+?)(?:\n|$)/);
        const draftM = text.match(/```\n([\s\S]*?)```/);

        const token = tokenM ? tokenM[1] : '';
        const subject = subjectM ? subjectM[1].trim() : 'Updated Draft';
        const summary = summaryM ? summaryM[1].trim() : '';
        const draft = draftM ? draftM[1].trim() : '';
        const now = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

        el.innerHTML = `
        <div class="ac-draft-card ac-draft-updated" id="ew-${token}">
          <div class="ac-card-header">
            <div class="ac-card-header-left">
              <div class="ac-bot-avatar" style="background:linear-gradient(135deg,#ffd166,#c4953a)">🔄</div>
              <div>
                <div class="ac-card-title">Updated Draft <span class="ac-card-sub">${escHtml(subject)}</span></div>
                <div class="ac-card-time">${now} · Token: <code class="ac-token-code">${escHtml(token)}</code></div>
              </div>
            </div>
            <span class="ac-badge ac-badge-yellow">Updated</span>
          </div>

          ${summary ? `
          <div class="ac-card-section">
            <div class="ac-section-label">✨ Summary</div>
            <div class="ac-summary-box">${escHtml(summary)}</div>
          </div>` : ''}

          <div class="ac-card-section">
            <div class="ac-draft-toolbar">
              <div class="ac-section-label" style="margin:0">✍️ Draft Content</div>
              <div class="ac-toolbar-right">
                <select class="ac-select" onchange="triggerRegenerateTone('${token}', this.value)">
                  <option value="" disabled selected>Tone</option>
                  <option value="Make it Highly Professional">Professional</option>
                  <option value="Make it Friendly and Warm">Friendly</option>
                  <option value="Make it Formal and respectful">Formal</option>
                  <option value="Make it extremely concise">Concise</option>
                </select>
                <button class="ac-toolbar-btn" onclick="copyToClipboard('ac-draft-${token}')">📋 Copy</button>
              </div>
            </div>
            <textarea class="ac-draft-textarea" id="ac-draft-${token}">${escHtml(draft)}</textarea>
          </div>

          <div class="ac-actions-bar">
            <button class="ac-action-btn ac-btn-approve" onclick="submitEwAction('approve', '${token}')">
              <span>✅</span> Approve &amp; Send
            </button>
            <button class="ac-action-btn ac-btn-edit" onclick="submitEwAction('edit', '${token}')">
              <span>✏️</span> Save Edit
            </button>
            <button class="ac-action-btn ac-btn-regen" onclick="openRegenerateModal('${token}')">
              <span>🔄</span> Regenerate
            </button>
            <button class="ac-action-btn ac-btn-reject" onclick="submitEwAction('reject', '${token}')">
              <span>❌</span> Reject
            </button>
          </div>
          <div class="ac-card-footer-note">🔒 AssistClaw logs every action. Approvals are reversible within 5 minutes.</div>
        </div>
        `;
        scrollToBottom();
        return;
    }

    // ── Legacy pattern (old format kept for backwards compat) ────
    if (text.includes("📧 *Email Draft Review*")) {
        const match = text.match(/\*Account:\*\s*(.*?)\n\*From:\*\s*(.*?)\n\*Subject:\*\s*(.*?)\n\n\*Summary:\*\n([\s\S]*?)\n\n\*Draft Reply:\*\n```\n([\s\S]*?)```\n\n_Please.*edit\s+([a-zA-Z0-9]+)\._/i);
        if (match) {
            const [_, account, from, subject, summary, draft, token] = match;

            // Fake metadata derived from regex
            const attachments = ['proposal_v3.pdf', 'timeline.xlsx'];

            const htmlContent = `
                <div class="email-app-widget" id="ew-${token}">
                    
                    <!-- Top Status Bar -->
                    <div class="ew-panel" style="padding: 12px 16px; display: flex; justify-content: space-between; align-items: center;">
                        <div class="ew-h-left">
                            <div class="ew-avatar">AC</div>
                            <div class="ew-h-title">
                                <h3>AssistClaw</h3>
                                <p>Email Review Agent · Just now</p>
                            </div>
                        </div>
                        <div class="ew-h-right">
                            <button class="ew-pill green">Ready to send</button>
                            <button class="ew-pill">Medium priority</button>
                        </div>
                    </div>

                    <!-- Email Details -->
                    <div class="ew-panel">
                        <div class="ew-section-title">EMAIL DETAILS</div>
                        <div class="ew-details-grid">
                            <div class="ew-details-row">
                                <div class="ew-col-left">
                                    <div class="ew-info-label">From</div>
                                    <div class="ew-info-value bold">${escHtml(from.split('<')[0] || from)}</div>
                                    <div class="ew-info-value sub">${escHtml(from.includes('<') ? '<' + from.split('<')[1] : from)}</div>
                                </div>
                                <div class="ew-col-right">
                                    <div class="ew-info-label">To</div>
                                    <div class="ew-info-value bold">You</div>
                                    <div class="ew-info-value sub">${escHtml(account)}</div>
                                </div>
                            </div>
                            
                            <div style="height: 1px; background: #3A3A3A; margin: 4px 0;"></div>
                            
                            <div class="ew-flex-row">
                                <span class="ew-label">Subject</span>
                                <span class="ew-value">${escHtml(subject)}</span>
                            </div>
                            <div class="ew-flex-row">
                                <span class="ew-label">Received</span>
                                <span class="ew-value">Today at 6:41 PM</span>
                            </div>
                            <div class="ew-flex-row">
                                <span class="ew-label">Thread length</span>
                                <span class="ew-value">4 messages</span>
                            </div>
                            <div class="ew-flex-row">
                                <span class="ew-label">Attachments</span>
                                <div class="ew-attachments">
                                    <div class="ew-attachment-pill">proposal_v3.pdf</div>
                                    <div class="ew-attachment-pill">timeline.xlsx</div>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- AI Summary -->
                    <div class="ew-panel p-0">
                        <div class="ew-collapsible-header" onclick="toggleEwSection(this)">
                            <h4>AI summary</h4>
                            <svg class="ew-chevron" viewBox="0 0 24 24"><path d="M7 10l5 5 5-5z"/></svg>
                        </div>
                        <div class="ew-collapsible-body">
                            <div class="ew-summary-box">${escHtml(summary)}</div>
                            
                            <div class="ew-section-title">DETECTED INTENT TAGS</div>
                            <div class="ew-intent-tags">
                                <div class="ew-intent-tag highlight" onclick="injectTone('${token}', 'Action required')">Action required</div>
                                <div class="ew-intent-tag highlight" onclick="injectTone('${token}', 'Deadline: Friday')">Deadline: Friday</div>
                                <div class="ew-intent-tag" onclick="injectTone('${token}', 'Follow-up needed')">Follow-up needed</div>
                                <div class="ew-intent-tag highlight" onclick="injectTone('${token}', 'Proposal review')">Proposal review</div>
                                <div class="ew-intent-tag" onclick="injectTone('${token}', 'Client meeting')">Client meeting</div>
                                <div class="ew-intent-tag" onclick="injectTone('${token}', 'Pricing discussion')">Pricing discussion</div>
                            </div>

                            <div class="ew-stats">
                                <div class="ew-stat">
                                    <div class="ew-stat-val blue">87%</div>
                                    <div class="ew-stat-lbl">Sentiment score</div>
                                </div>
                                <div class="ew-stat">
                                    <div class="ew-stat-val green">High</div>
                                    <div class="ew-stat-lbl">Urgency level</div>
                                </div>
                                <div class="ew-stat">
                                    <div class="ew-stat-val purple">4m</div>
                                    <div class="ew-stat-lbl">Est. read time</div>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- Draft Reply -->
                    <div class="ew-panel p-0">
                        <div class="ew-collapsible-header" onclick="toggleEwSection(this)">
                            <h4>Draft reply</h4>
                            <svg class="ew-chevron" viewBox="0 0 24 24"><path d="M7 10l5 5 5-5z"/></svg>
                        </div>
                        <div class="ew-collapsible-body">
                            <div class="ew-draft-toolbar">
                                <select class="ew-dropdown" onchange="triggerRegenerateTone('${token}', this.value)">
                                    <option value="" disabled selected>Professional</option>
                                    <option value="Make it Highly Professional">Professional</option>
                                    <option value="Make it Friendly and Warm">Friendly</option>
                                    <option value="Make it Formal and respectful">Formal</option>
                                    <option value="Make it extremely concise">Concise</option>
                                </select>
                                <select class="ew-dropdown" onchange="triggerTranslate('${token}', this.value)">
                                    <option value="" disabled selected>English</option>
                                    <option value="Spanish">Spanish</option>
                                    <option value="French">French</option>
                                    <option value="Japanese">Japanese</option>
                                </select>
                                <button class="btn-copy" onclick="copyToClipboard('draft-tx-${token}')">Copy</button>
                            </div>
                            
                            <textarea class="ew-textarea" id="draft-tx-${token}">${escHtml(draft.trim())}</textarea>

                            <div class="ew-section-title">SIGNATURE</div>
                            <select class="ew-dropdown" style="width: 100%;" onchange="appendSignature('${token}', this.value)">
                                <option value="" disabled selected>Default signature</option>
                                <option value="\n\nBest Regards,\nAssistClaw">Standard</option>
                                <option value="\n\nThanks!">Short</option>
                            </select>
                        </div>
                    </div>

                    <!-- Options Toggles -->
                    <div class="ew-panel p-0 collapsed">
                        <div class="ew-collapsible-header" onclick="toggleEwSection(this)">
                            <h4>Send options & scheduling</h4>
                            <svg class="ew-chevron" viewBox="0 0 24 24"><path d="M7 10l5 5 5-5z"/></svg>
                        </div>
                        <div class="ew-collapsible-body"></div>
                    </div>
                    
                    <div class="ew-panel p-0 collapsed">
                        <div class="ew-collapsible-header" onclick="toggleEwSection(this)">
                            <h4>Thread history</h4>
                            <svg class="ew-chevron" viewBox="0 0 24 24"><path d="M7 10l5 5 5-5z"/></svg>
                        </div>
                        <div class="ew-collapsible-body"></div>
                    </div>

                    <!-- Actions Panel -->
                    <div class="ew-panel">
                        <div class="ew-section-title">ACTIONS</div>
                        <div class="ew-actions-grid">
                            <button class="ew-action-btn" onclick="submitEwAction('approve', '${token}')">Approve & send ↗</button>
                            <button class="ew-action-btn" onclick="submitEwAction('edit', '${token}')">Edit draft ↗</button>
                            <button class="ew-action-btn" onclick="openRegenerateModal('${token}')">Regenerate ↗</button>
                            <button class="ew-action-btn" onclick="submitEwAction('regenerate', '${token}', 'Translate the draft to another language natively.')">Translate ↗</button>
                            <button class="ew-action-btn" onclick="submitEwAction('reject', '${token}')">Reject & discard</button>
                        </div>
                        <div class="ew-footer-hint">AssistClaw will log this action and update your email queue. All approvals are reversible within 5 minutes.</div>
                    </div>

                </div>
            `;
            el.innerHTML = htmlContent;
            scrollToBottom();
            return;
        }
    }

    // Basic markdown rendering (no external lib dependency)
    let html = escHtml(text)
        .replace(/```([\s\S]*?)```/g, '<pre><code>$1</code></pre>')
        .replace(/`([^`]+)`/g, '<code>$1</code>')
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.+?)\*/g, '<em>$1</em>')
        .split('\n\n')
        .map(p => '<p>' + p.replace(/\n/g, '<br>') + '</p>')
        .join('');
    el.innerHTML = html;
    scrollToBottom();
}

// --- Interactive App Widget Functions ---

window.toggleEwSection = function (el) {
    if (!el) return;
    const panel = el.closest('.ew-panel');
    if (panel) panel.classList.toggle('collapsed');
};

window.copyToClipboard = function (id) {
    const el = document.getElementById(id);
    if (el) {
        navigator.clipboard.writeText(el.value).then(() => {
            showToast('Draft copied to clipboard!');
        }).catch(err => {
            showToast('Failed to copy: ' + err, 'error');
        });
    }
};

window.togglePriorityDropdown = function (token) {
    const el = document.getElementById(`ew-p-drop-${token}`);
    if (el) el.classList.toggle('hidden');
};

window.setPriority = function (token, label, cls) {
    const badge = document.getElementById(`ew-badge-${token}`);
    if (badge) {
        badge.textContent = label + ' Priority';
        badge.className = 'ew-badge ' + cls;
    }
    const drop = document.getElementById(`ew-p-drop-${token}`);
    if (drop) drop.classList.add('hidden');
};

window.injectTone = function (token, instruct) {
    const chatInput = document.getElementById('msg-input');
    chatInput.value = `regenerate ${token}: ${instruct}`;
    sendMessage();
    showToast('Sent AI Instruction: ' + instruct);
};

window.triggerRegenerateTone = function (token, tone) {
    if (!tone) return;
    const chatInput = document.getElementById('msg-input');
    chatInput.value = `regenerate ${token}: ${tone}`;
    sendMessage();
    showToast('Regenerating draft...');
};

window.triggerTranslate = function (token, lang) {
    if (!lang) return;
    const chatInput = document.getElementById('msg-input');
    chatInput.value = `regenerate ${token}: Translate this draft to ${lang}. Keep everything else identical.`;
    sendMessage();
    showToast('Translating to ' + lang + '...');
};

window.appendSignature = function (token, sig) {
    if (!sig) return;
    const area = document.getElementById(`draft-tx-${token}`);
    if (area) {
        area.value = area.value.trimEnd() + sig.replace(/\\n/g, '\n');
        showToast('Signature appended!');
    }
};

window.submitEwAction = function (action, token, extra) {
    // Support both old and new textarea ID formats
    const area = document.getElementById(`ac-draft-${token}`) || document.getElementById(`draft-tx-${token}`);
    const input = document.getElementById('msg-input');

    if (action === 'edit') {
        input.value = `edit ${token}: ${area.value}`;
        showToast('Draft edited!');
    } else if (action === 'regenerate' && extra) {
        input.value = `regenerate ${token}: ${extra}`;
        showToast('Command sent!');
    } else {
        input.value = `${action} ${token}`;
        if (action === 'reject') showToast('Draft Rejected', 'error');
        if (action === 'approve') showToast('Draft Approved & Setup for dispatch!');
    }
    sendMessage();
};

window.openEmailModal = function (token, currentDraft) { // fallback
    document.getElementById('email-modal').classList.remove('hidden');
};
window.closeEmailModal = function () {
    const el1 = document.getElementById('email-modal');
    if (el1) el1.classList.add('hidden');
    const regModal = document.getElementById('regenerate-modal');
    if (regModal) regModal.classList.add('hidden');
};

window.openRegenerateModal = function (token) {
    const modal = document.getElementById('regenerate-modal');
    if (!modal) return;
    const input = document.getElementById('regenerate-input');
    const btn = document.getElementById('submit-regenerate-btn');

    modal.classList.remove('hidden');
    input.value = '';
    input.focus();

    const newBtn = btn.cloneNode(true);
    btn.parentNode.replaceChild(newBtn, btn);

    newBtn.addEventListener('click', () => {
        const text = input.value.trim();
        if (text) {
            const chatInput = document.getElementById('msg-input');
            chatInput.value = `regenerate ${token}: ${text}`;
            window.closeEmailModal();
            sendMessage();
            showToast('Regeneration started...');
        }
    });
};

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
