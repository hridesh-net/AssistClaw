package email

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store persists email assistant state under the state directory.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates email.db under stateDir/email/.
func OpenStore(stateDir string) (*Store, error) {
	dir := filepath.Join(stateDir, "email")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "email.db")
	db, err := sql.Open("sqlite3", path+"?_fk=1&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS imap_state (
			account_name TEXT PRIMARY KEY,
			last_uid INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS email_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_name TEXT NOT NULL,
			provider_msg_id TEXT NOT NULL,
			from_addr TEXT,
			subject TEXT,
			snippet TEXT,
			headers_json TEXT,
			created_at INTEGER NOT NULL,
			UNIQUE(account_name, provider_msg_id)
		)`,
		`CREATE TABLE IF NOT EXISTS email_drafts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id INTEGER NOT NULL REFERENCES email_messages(id) ON DELETE CASCADE,
			token TEXT NOT NULL UNIQUE,
			summary TEXT,
			body TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS email_audit (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			event TEXT NOT NULL,
			ref_token TEXT,
			detail TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_tokens (
			account_name TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			token_json TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w: %s", err, q)
		}
	}
	return nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) GetLastIMAPUID(account string) (uint32, error) {
	var u sql.NullInt64
	err := s.db.QueryRow(`SELECT last_uid FROM imap_state WHERE account_name = ?`, account).Scan(&u)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !u.Valid {
		return 0, nil
	}
	return uint32(u.Int64), nil
}

func (s *Store) SetLastIMAPUID(account string, uid uint32) error {
	_, err := s.db.Exec(`
		INSERT INTO imap_state(account_name, last_uid) VALUES(?, ?)
		ON CONFLICT(account_name) DO UPDATE SET last_uid = excluded.last_uid`,
		account, int64(uid))
	return err
}

// MessageExists returns whether we already ingested this provider message.
func (s *Store) MessageExists(account, providerMsgID string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM email_messages WHERE account_name = ? AND provider_msg_id = ?`,
		account, providerMsgID,
	).Scan(&n)
	return n > 0, err
}

// InsertMessage inserts a message row; returns id.
func (s *Store) InsertMessage(account, providerMsgID, fromAddr, subject, snippet string, headers map[string]string) (int64, error) {
	hj, _ := json.Marshal(headers)
	ts := time.Now().Unix()
	res, err := s.db.Exec(`
		INSERT INTO email_messages(account_name, provider_msg_id, from_addr, subject, snippet, headers_json, created_at)
		VALUES(?,?,?,?,?,?,?)`,
		account, providerMsgID, fromAddr, subject, snippet, string(hj), ts)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertDraft creates a pending draft with a unique token.
func (s *Store) InsertDraft(ctx context.Context, messageID int64, token, summary, body string) error {
	ts := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO email_drafts(message_id, token, summary, body, status, created_at)
		VALUES(?,?,?,?,?,?)`,
		messageID, token, summary, body, string(DraftPending), ts)
	return err
}

// GetDraftByToken loads a draft joined with message for reply metadata.
func (s *Store) GetDraftByToken(ctx context.Context, token string) (*StoredDraft, *StoredMessage, map[string]string, error) {
	token = strings.TrimSpace(strings.ToLower(token))
	var d StoredDraft
	var m StoredMessage
	var hj sql.NullString
	var dCreated, mCreated int64
	var st string
	err := s.db.QueryRowContext(ctx, `
		SELECT d.id, d.message_id, d.token, d.summary, d.body, d.status, d.created_at,
		       m.id, m.account_name, m.provider_msg_id, m.from_addr, m.subject, m.snippet, m.headers_json, m.created_at
		FROM email_drafts d JOIN email_messages m ON m.id = d.message_id
		WHERE lower(d.token) = ?`, token).Scan(
		&d.ID, &d.MessageID, &d.Token, &d.Summary, &d.Body, &st, &dCreated,
		&m.ID, &m.AccountName, &m.ProviderMsgID, &m.FromAddr, &m.Subject, &m.Snippet, &hj, &mCreated,
	)
	d.Status = DraftStatus(st)
	if err == sql.ErrNoRows {
		return nil, nil, nil, err
	}
	if err != nil {
		return nil, nil, nil, err
	}
	d.CreatedAt = time.Unix(dCreated, 0)
	m.CreatedAt = time.Unix(mCreated, 0)
	headers := map[string]string{}
	if hj.Valid && hj.String != "" {
		_ = json.Unmarshal([]byte(hj.String), &headers)
	}
	return &d, &m, headers, nil
}

func (s *Store) UpdateDraftBody(ctx context.Context, token, body string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE email_drafts SET body = ? WHERE lower(token) = ?`, body, strings.TrimSpace(strings.ToLower(token)))
	return err
}

func (s *Store) SetDraftStatus(ctx context.Context, token string, st DraftStatus) error {
	_, err := s.db.ExecContext(ctx, `UPDATE email_drafts SET status = ? WHERE lower(token) = ?`, string(st), strings.TrimSpace(strings.ToLower(token)))
	return err
}

func (s *Store) AppendAudit(ctx context.Context, event, refToken, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO email_audit(created_at, event, ref_token, detail) VALUES(?,?,?,?)`,
		time.Now().Unix(), event, refToken, detail)
	return err
}

// RateLimitDrafts returns false if account exceeded max per rolling hour.
func (s *Store) RateLimitDrafts(ctx context.Context, account string, maxPerHour int) (bool, error) {
	if maxPerHour < 1 {
		maxPerHour = 30
	}
	since := time.Now().Add(-time.Hour).Unix()
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM email_drafts d
		JOIN email_messages m ON m.id = d.message_id
		WHERE m.account_name = ? AND d.created_at >= ?`, account, since).Scan(&n)
	if err != nil {
		return false, err
	}
	return n < maxPerHour, nil
}

// ListPendingDrafts returns tokens still pending (for CLI).
func (s *Store) ListPendingDrafts(ctx context.Context) ([]struct {
	Token, Account, Subject string
	CreatedAt                 time.Time
}, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.token, m.account_name, m.subject, d.created_at
		FROM email_drafts d JOIN email_messages m ON m.id = d.message_id
		WHERE d.status = ? ORDER BY d.created_at DESC`, string(DraftPending))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		Token, Account, Subject string
		CreatedAt                 time.Time
	}
	for rows.Next() {
		var t, a, subj string
		var ts int64
		if err := rows.Scan(&t, &a, &subj, &ts); err != nil {
			return nil, err
		}
		out = append(out, struct {
			Token, Account, Subject string
			CreatedAt                 time.Time
		}{t, a, subj, time.Unix(ts, 0)})
	}
	return out, rows.Err()
}

// SaveOAuthToken persists refreshable OAuth JSON for an account.
func (s *Store) SaveOAuthToken(account, provider string, tokenJSON []byte) error {
	_, err := s.db.Exec(`
		INSERT INTO oauth_tokens(account_name, provider, token_json, updated_at)
		VALUES(?,?,?,?)
		ON CONFLICT(account_name) DO UPDATE SET provider=excluded.provider, token_json=excluded.token_json, updated_at=excluded.updated_at`,
		account, provider, string(tokenJSON), time.Now().Unix())
	return err
}

// LoadOAuthToken returns token JSON for account (from DB or falls back to reading token_file path in cfg — caller handles file).
func (s *Store) LoadOAuthToken(account string) ([]byte, string, error) {
	var prov, js string
	err := s.db.QueryRow(`SELECT provider, token_json FROM oauth_tokens WHERE account_name = ?`, account).Scan(&prov, &js)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return []byte(js), prov, nil
}
