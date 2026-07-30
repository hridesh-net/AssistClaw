#ifndef TUI_RS_H
#define TUI_RS_H

#ifdef __cplusplus
extern "C" {
#endif

/* ── Lifecycle ───────────────────────────────────────────── */

int tui_init(void);
void tui_shutdown(void);

/* ── REPL Screen ─────────────────────────────────────────── */

int tui_repl_start(const char* config_json);
void tui_repl_stop(void);
void tui_repl_send_token(const char* token);
void tui_repl_send_tool(const char* name, const char* input_json);
void tui_repl_send_done(int iterations, int total_tokens);
void tui_repl_send_error(const char* message);

/* ── Status Dashboard ────────────────────────────────────── */

int tui_status_run(const char* info_json);
void tui_status_update(const char* status_json);

/* ── Onboarding Wizard ───────────────────────────────────── */

char* tui_onboard_run(const char* config_json);

/* ── Skills Configuration ────────────────────────────────── */

char* tui_skills_run(const char* catalog_json, const char* current_skills_json);

/* ── One-shot Renderers (banner, version, CLI header) ───── */

char* tui_render_banner(const char* version, const char* session_id, int providers, int skills);
char* tui_render_onboard_banner(const char* version);
char* tui_render_cli_header(const char* version, int term_width);
char* tui_render_version_block(const char* version);

/* ── Event Polling ───────────────────────────────────────── */

char* tui_poll_event(void);
void tui_free_string(char* s);

#ifdef __cplusplus
}
#endif

#endif
