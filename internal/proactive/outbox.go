package proactive

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Outbox wraps a Notifier with SQLite-backed persistence and exponential retry.
type Outbox struct {
	inner  Notifier
	db     *sql.DB
	log    *zap.Logger
	stop   chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex
	running bool
}

// NewOutbox creates a retrying notifier wrapper backed by SQLite.
func NewOutbox(inner Notifier, dbPath string, log *zap.Logger) (*Outbox, error) {
	if log == nil {
		log = zap.NewNop()
	}
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("outbox db open: %w", err)
	}
	o := &Outbox{
		inner: inner,
		db:    db,
		log:   log,
		stop:  make(chan struct{}),
	}
	if err := o.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return o, nil
}

func (o *Outbox) migrate() error {
	_, err := o.db.Exec(`
		CREATE TABLE IF NOT EXISTS outbox (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_id     TEXT NOT NULL,
			body        TEXT NOT NULL,
			meta        TEXT,
			attempts    INTEGER DEFAULT 0,
			next_retry  DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_outbox_next_retry ON outbox(next_retry);
	`)
	return err
}

// Name delegates to the inner notifier.
func (o *Outbox) Name() string { return o.inner.Name() }

// Send persists the notification and returns immediately. Delivery is asynchronous.
func (o *Outbox) Send(ctx context.Context, n Notification) error {
	meta, err := json.Marshal(n.Meta)
	if err != nil {
		return err
	}
	_, err = o.db.ExecContext(ctx,
		`INSERT INTO outbox (rule_id, body, meta, next_retry) VALUES (?, ?, ?, ?)`,
		n.RuleID, n.Body, string(meta), time.Now(),
	)
	return err
}

// Start begins the background retry worker.
func (o *Outbox) Start() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.running {
		return
	}
	o.running = true
	o.wg.Add(1)
	go o.worker()
}

// Stop signals the worker to shut down.
func (o *Outbox) Stop() {
	o.mu.Lock()
	if o.running {
		close(o.stop)
		o.running = false
	}
	o.mu.Unlock()
	o.wg.Wait()
}

// Close stops the worker and closes the underlying database connection.
func (o *Outbox) Close() error {
	o.Stop()
	return o.db.Close()
}

func (o *Outbox) worker() {
	defer o.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-o.stop:
			return
		case <-ticker.C:
			o.flush()
		}
	}
}

func (o *Outbox) flush() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		var id int
		var ruleID, body, metaStr string
		err := o.db.QueryRowContext(ctx,
			`SELECT id, rule_id, body, meta FROM outbox WHERE next_retry <= ? ORDER BY id LIMIT 1`,
			time.Now(),
		).Scan(&id, &ruleID, &body, &metaStr)
		if err == sql.ErrNoRows {
			return
		}
		if err != nil {
			o.log.Warn("outbox query failed", zap.Error(err))
			return
		}

		var meta map[string]string
		if unmarshalErr := json.Unmarshal([]byte(metaStr), &meta); unmarshalErr != nil {
			o.log.Warn("outbox meta unmarshal failed, continuing with empty meta",
				zap.Int("id", id),
				zap.Error(unmarshalErr),
			)
		}
		n := Notification{RuleID: ruleID, Body: body, Meta: meta}

		sendErr := o.inner.Send(ctx, n)
		if sendErr == nil {
			if _, delErr := o.db.ExecContext(ctx, `DELETE FROM outbox WHERE id = ?`, id); delErr != nil {
				o.log.Warn("outbox delete after success failed",
					zap.Int("id", id),
					zap.Error(delErr),
				)
			}
			continue
		}

		// Exponential backoff with jitter: base * 2^attempts + rand [0, 5s]
		var attempts int
		if scanErr := o.db.QueryRowContext(ctx, `SELECT attempts FROM outbox WHERE id = ?`, id).Scan(&attempts); scanErr != nil {
			o.log.Warn("outbox attempts scan failed, treating as 0",
				zap.Int("id", id),
				zap.Error(scanErr),
			)
		}
		const maxAttempts = 10
		if attempts >= maxAttempts {
			if _, delErr := o.db.ExecContext(ctx, `DELETE FROM outbox WHERE id = ?`, id); delErr != nil {
				o.log.Warn("outbox delete after max attempts failed",
					zap.Int("id", id),
					zap.Error(delErr),
				)
			}
			o.log.Error("outbox send failed permanently, poison message dropped",
				zap.String("notifier", o.inner.Name()),
				zap.Int("id", id),
				zap.Int("attempts", attempts),
				zap.Error(sendErr),
			)
			continue
		}
		delay := time.Duration(15*time.Second) * (1 << min(attempts, 6))
		delay += time.Duration(rand.Intn(5000)) * time.Millisecond
		next := time.Now().Add(delay)

		if _, upErr := o.db.ExecContext(ctx,
			`UPDATE outbox SET attempts = attempts + 1, next_retry = ? WHERE id = ?`,
			next, id,
		); upErr != nil {
			o.log.Warn("outbox update after failed send failed",
				zap.Int("id", id),
				zap.Error(upErr),
			)
		}
		o.log.Warn("outbox send failed, retry scheduled",
			zap.String("notifier", o.inner.Name()),
			zap.Int("id", id),
			zap.Duration("delay", delay),
			zap.Error(sendErr),
		)
	}
}
