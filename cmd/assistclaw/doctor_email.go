package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/assistclaw/assistclaw/internal/config"
)

func checkEmailAssistant(cfg *config.Config) []doctorCheck {
	if cfg == nil || !cfg.Email.Enabled {
		return []doctorCheck{{
			ID:       "email.enabled",
			Severity: "skipped",
			Message:  "email.enabled is false.",
		}}
	}
	var out []doctorCheck
	n := cfg.Email.Notify
	ch := strings.ToLower(strings.TrimSpace(n.Channel))
	sid := strings.TrimSpace(n.SessionID)
	if ch == "" || sid == "" {
		out = append(out, doctorCheck{
			ID:       "email.notify",
			Severity: "error",
			Message:  "email.notify.channel and session_id are required when email is enabled.",
		})
		return out
	}
	if err := validateEmailNotifySession(ch, sid); err != nil {
		out = append(out, doctorCheck{
			ID:       "email.notify.session",
			Severity: "error",
			Message:  err.Error(),
		})
	} else {
		out = append(out, doctorCheck{
			ID:       "email.notify.session",
			Severity: "ok",
			Message:  fmt.Sprintf("email.notify session_id format matches channel %q.", ch),
		})
	}
	if len(cfg.Email.Accounts) == 0 {
		out = append(out, doctorCheck{
			ID:       "email.accounts",
			Severity: "error",
			Message:  "email.enabled but email.accounts is empty.",
		})
		return out
	}
	for _, acc := range cfg.Email.Accounts {
		prefix := "email.account." + acc.Name
		switch strings.ToLower(strings.TrimSpace(acc.Backend)) {
		case "gmail":
			if acc.Gmail == nil || strings.TrimSpace(acc.Gmail.TokenFile) == "" {
				out = append(out, doctorCheck{ID: prefix, Severity: "error", Message: fmt.Sprintf("account %q (gmail): gmail.token_file is required", acc.Name)})
				continue
			}
			p := resolvePathUnderState(cfg.StateDir, acc.Gmail.TokenFile)
			if _, err := os.Stat(p); err != nil {
				out = append(out, doctorCheck{
					ID:       prefix + ".token",
					Severity: "warn",
					Message:  fmt.Sprintf("account %q: Gmail OAuth token not found at %q (%v). Run `assistclaw email login-gmail --account=%s`.", acc.Name, p, err, acc.Name),
				})
			} else {
				out = append(out, doctorCheck{ID: prefix + ".token", Severity: "ok", Message: fmt.Sprintf("account %q: Gmail token file present.", acc.Name)})
			}
		case "graph":
			if acc.Graph == nil || strings.TrimSpace(acc.Graph.TokenFile) == "" {
				out = append(out, doctorCheck{ID: prefix, Severity: "error", Message: fmt.Sprintf("account %q (graph): graph.token_file is required", acc.Name)})
				continue
			}
			p := resolvePathUnderState(cfg.StateDir, acc.Graph.TokenFile)
			if _, err := os.Stat(p); err != nil {
				out = append(out, doctorCheck{
					ID:       prefix + ".token",
					Severity: "warn",
					Message:  fmt.Sprintf("account %q: Graph OAuth token not found at %q (%v). Run `assistclaw email login-graph --account=%s`.", acc.Name, p, err, acc.Name),
				})
			} else {
				out = append(out, doctorCheck{ID: prefix + ".token", Severity: "ok", Message: fmt.Sprintf("account %q: Graph token file present.", acc.Name)})
			}
		case "imap":
			if acc.IMAP == nil || strings.TrimSpace(acc.IMAP.Host) == "" {
				out = append(out, doctorCheck{ID: prefix + ".imap", Severity: "error", Message: fmt.Sprintf("account %q (imap): imap.host is required", acc.Name)})
				continue
			}
			switch {
			case strings.TrimSpace(acc.IMAP.Username) == "":
				out = append(out, doctorCheck{ID: prefix + ".imap.user", Severity: "warn", Message: fmt.Sprintf("account %q: imap.username is empty.", acc.Name)})
			case strings.TrimSpace(acc.IMAP.Password) == "":
				out = append(out, doctorCheck{ID: prefix + ".imap.pass", Severity: "warn", Message: fmt.Sprintf("account %q: imap.password is empty (set in config or env expansion).", acc.Name)})
			default:
				out = append(out, doctorCheck{ID: prefix + ".imap", Severity: "ok", Message: fmt.Sprintf("account %q: IMAP host and credentials are set (doctor does not log in to IMAP).", acc.Name)})
			}
		default:
			out = append(out, doctorCheck{ID: prefix, Severity: "warn", Message: fmt.Sprintf("account %q: unknown backend %q.", acc.Name, acc.Backend)})
		}
		if acc.Notify != nil && strings.TrimSpace(acc.Notify.Channel) != "" {
			ach := strings.ToLower(strings.TrimSpace(acc.Notify.Channel))
			asid := strings.TrimSpace(acc.Notify.SessionID)
			if asid == "" {
				out = append(out, doctorCheck{ID: prefix + ".notify", Severity: "error", Message: fmt.Sprintf("account %q: per-account notify.session_id is required when notify.channel is set.", acc.Name)})
			} else if err := validateEmailNotifySession(ach, asid); err != nil {
				out = append(out, doctorCheck{ID: prefix + ".notify", Severity: "error", Message: fmt.Sprintf("account %q notify: %v", acc.Name, err)})
			}
		}
	}
	return out
}

func validateEmailNotifySession(ch, sid string) error {
	switch ch {
	case "telegram":
		if !strings.HasPrefix(sid, "tg:") {
			return fmt.Errorf("for telegram, session_id must look like tg:<chatId> (optionally :threadId)")
		}
	case "slack":
		if !strings.HasPrefix(sid, "slack:") {
			return fmt.Errorf("for slack, session_id must start with slack:")
		}
	case "discord":
		if !strings.HasPrefix(sid, "discord:") {
			return fmt.Errorf("for discord, session_id must start with discord:")
		}
	default:
		return fmt.Errorf("unsupported notify channel %q (use telegram, slack, or discord)", ch)
	}
	return nil
}

func resolvePathUnderState(stateDir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(stateDir, p)
}
