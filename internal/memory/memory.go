// Package memory implements AssistClaw's three-tier memory system:
//
//  1. Working Memory — in-RAM session context (fast, volatile)
//  2. Episodic Memory — SQLite FTS5 conversation history (persistent, searchable)
//  3. Semantic Memory — sqlite-vec vector store (persistent, embedding-powered)
package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
)

func init() {
	AutoRegisterVec()
}

// ─────────────────────────────────────────────
// Shared types
// ─────────────────────────────────────────────

// Role represents a conversation participant.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single conversation turn.
type Message struct {
	ID        string                 `json:"id"`
	SessionID string                 `json:"session_id"`
	Role      Role                   `json:"role"`
	Content   string                 `json:"content"`
	Parts     []provider.ContentPart `json:"parts,omitempty"`
	Model     string                 `json:"model,omitempty"`
	Tokens    int                    `json:"tokens,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// Document is a chunk of content stored in semantic memory.
type Document struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"` // file path, URL, session ID, etc.
	Content   string    `json:"content"`
	Hash      string    `json:"hash,omitempty"`
	SourceType string   `json:"source_type,omitempty"`
	Palace    string    `json:"palace,omitempty"`
	Wing      string    `json:"wing,omitempty"`
	Room      string    `json:"room,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Model     string    `json:"model,omitempty"` // embedding model that generated this vector
	Embedding []float32 `json:"embedding,omitempty"`
	Score     float32   `json:"score,omitempty"` // similarity score (populated on search)
	CreatedAt time.Time `json:"created_at"`
}

// Lesson represents a "corrective memory" entry.
type Lesson struct {
	ID        string    `json:"id"`
	Query     string    `json:"query"` // The task/query that triggered this lesson
	Insights  string    `json:"insights"`
	Success   bool      `json:"success"`
	Embedding []float32 `json:"embedding,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ─────────────────────────────────────────────
// Tier 1: Working Memory (in-RAM)
// ─────────────────────────────────────────────

// WorkingMemory holds the current session's in-RAM message history.
// It is the fastest tier — O(1) append, O(n) scan.
type WorkingMemory struct {
	mu        sync.RWMutex
	messages  []Message
	maxTokens int
}

// NewWorkingMemory creates a working memory with the given token budget.
func NewWorkingMemory(maxTokens int) *WorkingMemory {
	if maxTokens <= 0 {
		maxTokens = 100_000
	}
	return &WorkingMemory{maxTokens: maxTokens}
}

// MaxTokens returns the token budget.
func (w *WorkingMemory) MaxTokens() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.maxTokens
}

// Append adds a message to the working memory.
func (w *WorkingMemory) Append(msg Message) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.messages = append(w.messages, msg)
}

// Messages returns a copy of all messages in chronological order.
func (w *WorkingMemory) Messages() []Message {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]Message, len(w.messages))
	copy(out, w.messages)
	return out
}

// TotalTokens returns the sum of all token counts in working memory.
func (w *WorkingMemory) TotalTokens() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	total := 0
	for _, m := range w.messages {
		total += m.Tokens
	}
	return total
}

// Compact removes oldest messages until the token count is within budget.
// System messages are always preserved.
func (w *WorkingMemory) Compact(budget int) []Message {
	w.mu.Lock()
	defer w.mu.Unlock()

	var dropped []Message
	for w.tokenCount() > budget && len(w.messages) > 1 {
		// Skip system message at index 0 — always keep it.
		dropIdx := 0
		if len(w.messages) > 0 && w.messages[0].Role == RoleSystem {
			dropIdx = 1
		}
		if dropIdx >= len(w.messages) {
			break
		}
		dropped = append(dropped, w.messages[dropIdx])
		w.messages = append(w.messages[:dropIdx], w.messages[dropIdx+1:]...)
	}
	return dropped
}

// Clear removes all messages.
func (w *WorkingMemory) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.messages = nil
}

func (w *WorkingMemory) tokenCount() int {
	total := 0
	for _, m := range w.messages {
		total += m.Tokens
	}
	return total
}

// ─────────────────────────────────────────────
// Tier 2: Episodic Memory (SQLite FTS5)
// ─────────────────────────────────────────────

// EpisodicMemory stores conversation history in SQLite with full-text search.
type EpisodicMemory struct {
	db *sql.DB
}

// NewEpisodicMemory opens (or creates) the episodic memory database.
func NewEpisodicMemory(dbPath string) (*EpisodicMemory, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("episodic memory open: %w", err)
	}
	m := &EpisodicMemory{db: db}
	if err := m.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("episodic memory migrate: %w", err)
	}
	return m, nil
}

func (m *EpisodicMemory) migrate() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id          TEXT PRIMARY KEY,
			session_id  TEXT NOT NULL,
			role        TEXT NOT NULL,
			content     TEXT NOT NULL,
			model       TEXT,
			tokens      INTEGER DEFAULT 0,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
		CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at);

		CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
			id UNINDEXED,
			session_id UNINDEXED,
			role UNINDEXED,
			content,
			content='messages',
			content_rowid='rowid'
		);

		CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, id, session_id, role, content)
			VALUES (new.rowid, new.id, new.session_id, new.role, new.content);
		END;

		CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, id, session_id, role, content)
			VALUES ('delete', old.rowid, old.id, old.session_id, old.role, old.content);
		END;
	`)
	return err
}

// Save persists a message to episodic memory.
func (m *EpisodicMemory) Save(ctx context.Context, msg Message) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO messages (id, session_id, role, content, model, tokens, created_at) VALUES (?,?,?,?,?,?,?)`,
		msg.ID, msg.SessionID, string(msg.Role), msg.Content, msg.Model, msg.Tokens, msg.CreatedAt,
	)
	return err
}

// GetSession returns all messages for a session in chronological order.
func (m *EpisodicMemory) GetSession(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, model, tokens, created_at
		 FROM messages WHERE session_id=? ORDER BY created_at ASC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Search performs full-text search across all sessions.
func (m *EpisodicMemory) Search(ctx context.Context, query string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := m.db.QueryContext(ctx,
		`SELECT m.id, m.session_id, m.role, m.content, m.model, m.tokens, m.created_at
		 FROM messages m
		 JOIN messages_fts fts ON m.rowid = fts.rowid
		 WHERE messages_fts MATCH ?
		 ORDER BY rank LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// ListSessions returns all distinct session IDs, newest first.
func (m *EpisodicMemory) ListSessions(ctx context.Context, limit int) ([]string, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT DISTINCT session_id FROM messages ORDER BY MAX(created_at) DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteSession removes all messages for a given session.
func (m *EpisodicMemory) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM messages WHERE session_id=?`, sessionID)
	return err
}

// Close closes the database.
func (m *EpisodicMemory) Close() error { return m.db.Close() }

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var msgs []Message
	for rows.Next() {
		var msg Message
		var role string
		if err := rows.Scan(&msg.ID, &msg.SessionID, &role, &msg.Content, &msg.Model, &msg.Tokens, &msg.CreatedAt); err != nil {
			return nil, err
		}
		msg.Role = Role(role)
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

// ─────────────────────────────────────────────
// Tier 3: Semantic Memory (sqlite-vec)
// ─────────────────────────────────────────────

// SemanticStore is the semantic memory contract used by the runner and tools.
// This allows introducing alternative backends without changing higher layers.
type SemanticStore interface {
	Index(ctx context.Context, doc Document) error
	Search(ctx context.Context, queryVec []float32, limit int) ([]Document, error)
	SearchWithModel(ctx context.Context, queryVec []float32, limit int) ([]Document, error)
	SaveLesson(ctx context.Context, lesson Lesson) error
	SearchLessons(ctx context.Context, queryVec []float32, limit int) ([]Lesson, error)
	Delete(ctx context.Context, id string) error
	DeleteBySource(ctx context.Context, source string) error
	GetSnippet(ctx context.Context, source string, startLine, endLine int) (string, error)
	SourceHash(ctx context.Context, source string) (string, error)
	ListSources(ctx context.Context) ([]string, error)
	Close() error
}

// SemanticMemory stores document embeddings in sqlite-vec for vector search.
type SemanticMemory struct {
	db         *sql.DB
	dimensions int
}

// NewSemanticMemory opens (or creates) the semantic memory database.
func NewSemanticMemory(dbPath string, dimensions int) (*SemanticMemory, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("semantic memory open: %w", err)
	}
	m := &SemanticMemory{db: db, dimensions: dimensions}
	if err := m.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("semantic memory migrate: %w", err)
	}
	return m, nil
}

func (m *SemanticMemory) migrate() error {
	_, err := m.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS documents (
			id         TEXT PRIMARY KEY,
			source     TEXT NOT NULL,
			content    TEXT NOT NULL,
			hash       TEXT,
			source_type TEXT,
			palace     TEXT,
			wing       TEXT,
			room       TEXT,
			tags_json  TEXT,
			model      TEXT,
			embedding  BLOB,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS vec_documents USING vec0(
			embedding float[%d]
		);

		CREATE TABLE IF NOT EXISTS lessons (
			id         TEXT PRIMARY KEY,
			query      TEXT NOT NULL,
			insights   TEXT NOT NULL,
			success    BOOLEAN,
			embedding  BLOB,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS vec_lessons USING vec0(
			embedding float[%d]
		);
	`, m.dimensions, m.dimensions))
	if err != nil {
		return err
	}
	// Backward-compatible schema evolution for existing deployments.
	maybeAddColumn := func(column, colType string) error {
		_, e := m.db.Exec(fmt.Sprintf(`ALTER TABLE documents ADD COLUMN %s %s`, column, colType))
		if e == nil || strings.Contains(strings.ToLower(e.Error()), "duplicate column name") {
			return nil
		}
		return e
	}
	if err := maybeAddColumn("source_type", "TEXT"); err != nil {
		return err
	}
	if err := maybeAddColumn("palace", "TEXT"); err != nil {
		return err
	}
	if err := maybeAddColumn("wing", "TEXT"); err != nil {
		return err
	}
	if err := maybeAddColumn("room", "TEXT"); err != nil {
		return err
	}
	if err := maybeAddColumn("tags_json", "TEXT"); err != nil {
		return err
	}
	_, err = m.db.Exec(`CREATE INDEX IF NOT EXISTS idx_documents_source ON documents(source);`)
	return err
}

// Index stores a document and its embedding.
func (m *SemanticMemory) Index(ctx context.Context, doc Document) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	embJSON, _ := json.Marshal(doc.Embedding)
	tagsJSON, _ := json.Marshal(doc.Tags)
	_, err = tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO documents (id, source, content, hash, source_type, palace, wing, room, tags_json, model, embedding, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		doc.ID, doc.Source, doc.Content, doc.Hash, doc.SourceType, doc.Palace, doc.Wing, doc.Room, string(tagsJSON), doc.Model, embJSON, doc.CreatedAt,
	)
	if err != nil {
		return err
	}

	if len(doc.Embedding) == m.dimensions {
		vecJSON, _ := json.Marshal(doc.Embedding)
		_, err = tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO vec_documents (rowid, embedding)
			 SELECT rowid, ? FROM documents WHERE id = ?`,
			vecJSON, doc.ID,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Search finds the top-k semantically similar documents using vector search.
func (m *SemanticMemory) Search(ctx context.Context, queryVec []float32, limit int) ([]Document, error) {
	if limit <= 0 {
		limit = 10
	}
	queryJSON, _ := json.Marshal(queryVec)
	rows, err := m.db.QueryContext(ctx, `
		SELECT d.id, d.source, d.content, d.source_type, d.palace, d.wing, d.room, d.tags_json, d.created_at, vd.distance
		FROM vec_documents vd
		JOIN documents d ON d.rowid = vd.rowid
		WHERE vd.embedding MATCH ? AND k = ?
		ORDER BY vd.distance ASC
	`, queryJSON, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var doc Document
		var dist float32
		var tagsJSON string
		if err := rows.Scan(&doc.ID, &doc.Source, &doc.Content, &doc.SourceType, &doc.Palace, &doc.Wing, &doc.Room, &tagsJSON, &doc.CreatedAt, &dist); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &doc.Tags)
		doc.Score = 1 - dist // convert distance to similarity
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

// SearchWithModel finds top-k documents and also returns the model that generated them.
func (m *SemanticMemory) SearchWithModel(ctx context.Context, queryVec []float32, limit int) ([]Document, error) {
	if limit <= 0 {
		limit = 10
	}
	queryJSON, _ := json.Marshal(queryVec)
	rows, err := m.db.QueryContext(ctx, `
		SELECT d.id, d.source, d.content, d.source_type, d.palace, d.wing, d.room, d.tags_json, d.model, d.created_at, vd.distance
		FROM vec_documents vd
		JOIN documents d ON d.rowid = vd.rowid
		WHERE vd.embedding MATCH ? AND k = ?
		ORDER BY vd.distance ASC
	`, queryJSON, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var doc Document
		var dist float32
		var tagsJSON string
		if err := rows.Scan(&doc.ID, &doc.Source, &doc.Content, &doc.SourceType, &doc.Palace, &doc.Wing, &doc.Room, &tagsJSON, &doc.Model, &doc.CreatedAt, &dist); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &doc.Tags)
		doc.Score = 1 - dist
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

// SaveLesson persists a lesson to semantic memory.
func (m *SemanticMemory) SaveLesson(ctx context.Context, lesson Lesson) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	embJSON, _ := json.Marshal(lesson.Embedding)
	_, err = tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO lessons (id, query, insights, success, embedding, created_at) VALUES (?,?,?,?,?,?)`,
		lesson.ID, lesson.Query, lesson.Insights, lesson.Success, embJSON, lesson.CreatedAt,
	)
	if err != nil {
		return err
	}

	if len(lesson.Embedding) == m.dimensions {
		_, err = tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO vec_lessons (rowid, embedding)
			 SELECT rowid, ? FROM lessons WHERE id = ?`,
			embJSON, lesson.ID,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SearchLessons finds relevant lessons from history.
func (m *SemanticMemory) SearchLessons(ctx context.Context, queryVec []float32, limit int) ([]Lesson, error) {
	if limit <= 0 {
		limit = 5
	}
	queryJSON, _ := json.Marshal(queryVec)
	rows, err := m.db.QueryContext(ctx, `
		SELECT l.id, l.query, l.insights, l.success, l.created_at
		FROM vec_lessons vl
		JOIN lessons l ON l.rowid = vl.rowid
		WHERE vl.embedding MATCH ? AND k = ?
		ORDER BY vl.distance ASC
	`, queryJSON, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lessons []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.Query, &l.Insights, &l.Success, &l.CreatedAt); err != nil {
			return nil, err
		}
		lessons = append(lessons, l)
	}
	return lessons, rows.Err()
}

// Delete removes a document from all indexes.
func (m *SemanticMemory) Delete(ctx context.Context, id string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM documents WHERE id=?`, id)
	return err
}

// DeleteBySource removes all documents from all indexes for a given source.
func (m *SemanticMemory) DeleteBySource(ctx context.Context, source string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM documents WHERE source=?`, source)
	return err
}

// Close closes the database.
func (m *SemanticMemory) Close() error { return m.db.Close() }

// GetSnippet returns a piece of content from a given source source, optionally restricted by line range.
func (m *SemanticMemory) GetSnippet(ctx context.Context, source string, startLine, endLine int) (string, error) {
	// If the source exists on disk, read it.
	data, err := os.ReadFile(source)
	if err == nil {
		content := string(data)
		if startLine > 0 || endLine > 0 {
			lines := strings.Split(content, "\n")
			start := startLine - 1
			if start < 0 {
				start = 0
			}
			end := endLine
			if end <= 0 || end > len(lines) {
				end = len(lines)
			}
			if start >= len(lines) {
				return "", nil
			}
			return strings.Join(lines[start:end], "\n"), nil
		}
		return content, nil
	}

	// Fallback: Query the database for the content of the document.
	// Since we only store chunks, we'll try to find the one that matches or contains the range.
	// For now, let's just return an error if file read fails, as Markdown memory is file-based.
	return "", fmt.Errorf("could not read source %q: %w", source, err)
}

// SourceHash returns the most recent hash we know for a source.
func (m *SemanticMemory) SourceHash(ctx context.Context, source string) (string, error) {
	var hash string
	err := m.db.QueryRowContext(ctx, "SELECT hash FROM documents WHERE source = ? LIMIT 1", source).Scan(&hash)
	if err != nil {
		return "", err
	}
	return hash, nil
}

// ListSources returns all distinct source paths currently indexed.
func (m *SemanticMemory) ListSources(ctx context.Context) ([]string, error) {
	rows, err := m.db.QueryContext(ctx, "SELECT DISTINCT source FROM documents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

// ─────────────────────────────────────────────
// Manager — unified facade over all three tiers
// ─────────────────────────────────────────────

// Manager provides a single interface over all memory tiers.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*WorkingMemory
	budget   int
	chunkSize    int
	chunkOverlap int

	Episodic *EpisodicMemory
	Semantic SemanticStore
}

// ListSessions returns all session IDs from both working and episodic memory.
func (m *Manager) ListSessions(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	workingIDs := make(map[string]bool)
	for id := range m.sessions {
		workingIDs[id] = true
	}
	m.mu.RUnlock()

	episodicIDs, err := m.Episodic.ListSessions(ctx, 100)
	if err != nil {
		return nil, err
	}

	for _, id := range episodicIDs {
		workingIDs[id] = true
	}

	var all []string
	for id := range workingIDs {
		all = append(all, id)
	}
	return all, nil
}

// DeleteSession clears working memory and episodic memory for a session.
func (m *Manager) DeleteSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	return m.Episodic.DeleteSession(ctx, sessionID)
}

func (m *Manager) GetWorking(sessionID string) *WorkingMemory {
	m.mu.Lock()
	defer m.mu.Unlock()

	if wm, ok := m.sessions[sessionID]; ok {
		return wm
	}

	wm := NewWorkingMemory(m.budget)
	m.sessions[sessionID] = wm
	return wm
}

// NewManager creates a Manager with all three tiers initialized.
func NewManager(cfg ManagerConfig) (*Manager, error) {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 512
	}
	if cfg.ChunkOverlap <= 0 {
		cfg.ChunkOverlap = 64
	}
	episodic, err := NewEpisodicMemory(cfg.EpisodicDBPath)
	if err != nil {
		return nil, fmt.Errorf("episodic memory: %w", err)
	}

	semantic, err := NewSemanticMemory(cfg.SemanticDBPath, cfg.EmbeddingDimensions)
	if err != nil {
		episodic.Close()
		return nil, fmt.Errorf("semantic memory: %w", err)
	}

	return &Manager{
		sessions: make(map[string]*WorkingMemory),
		budget:   cfg.WorkingTokenBudget,
		chunkSize: cfg.ChunkSize,
		chunkOverlap: cfg.ChunkOverlap,
		Episodic: episodic,
		Semantic: semantic,
	}, nil
}

// ManagerConfig holds configuration for all memory tiers.
type ManagerConfig struct {
	WorkingTokenBudget  int    // max tokens kept in working memory
	EpisodicDBPath      string // path to episodic SQLite DB
	SemanticDBPath      string // path to semantic sqlite-vec DB
	EmbeddingDimensions int    // dimensions for the configured embedding model
	ChunkSize           int
	ChunkOverlap        int
}

// Close shuts down all database connections.
func (m *Manager) Close() error {
	var errs []error
	if err := m.Episodic.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.Semantic.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("memory manager close errors: %v", errs)
	}
	return nil
}
