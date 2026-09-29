package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// CheckState is disposable machine-local monitoring state.
type CheckState struct {
	SourceID      string
	LastCheckedAt time.Time
	Latency       time.Duration
	RetryCount    int
	Availability  string
	NextCheckAt   time.Time
	LastError     string
}

// OperationalStore owns runtime/operational.db. It never stores durable revisions.
type OperationalStore struct{ Root string }

var operationalResetMu sync.Mutex

func (store OperationalStore) Open(ctx context.Context) (*sql.DB, error) {
	db, err := store.openOnce(ctx)
	if err == nil {
		return db, nil
	}
	if !isOperationalCorruption(err) {
		return nil, err
	}
	if resetErr := store.resetCorrupt(); resetErr != nil {
		return nil, fmt.Errorf("reset corrupt operational state: %w", resetErr)
	}
	return store.openOnce(ctx)
}

func (store OperationalStore) openOnce(ctx context.Context) (*sql.DB, error) {
	database, err := store.databasePath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+database+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	closeOnError := func(err error) (*sql.DB, error) { _ = db.Close(); return nil, err }
	if err := db.PingContext(ctx); err != nil {
		return closeOnError(err)
	}
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil {
		return closeOnError(err)
	}
	if integrity != "ok" {
		return closeOnError(fmt.Errorf("operational database quick_check: %s", integrity))
	}
	if _, err = db.ExecContext(ctx, `PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;
CREATE TABLE IF NOT EXISTS source_check_state (
 source_id TEXT PRIMARY KEY,
 last_checked_at TEXT NOT NULL,
 latency_ms INTEGER NOT NULL CHECK(latency_ms >= 0),
 retry_count INTEGER NOT NULL CHECK(retry_count >= 0),
 availability TEXT NOT NULL CHECK(availability IN ('available','unavailable')),
 next_check_at TEXT NOT NULL,
 last_error TEXT NOT NULL
) STRICT;`); err != nil {
		return closeOnError(fmt.Errorf("initialize operational state: %w", err))
	}
	return db, nil
}

func (store OperationalStore) databasePath() (string, error) {
	if store.Root == "" {
		return "", errors.New("workspace root is required")
	}
	runtime := filepath.Join(store.Root, "runtime")
	if err := os.MkdirAll(runtime, 0o700); err != nil {
		return "", err
	}
	if info, err := os.Lstat(runtime); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if err != nil {
			return "", err
		}
		return "", errors.New("unsafe runtime directory")
	}
	database := filepath.Join(runtime, "operational.db")
	if info, statErr := os.Lstat(database); statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return "", errors.New("unsafe operational database path")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	return database, nil
}

func (store OperationalStore) resetCorrupt() error {
	operationalResetMu.Lock()
	defer operationalResetMu.Unlock()
	database, err := store.databasePath()
	if err != nil {
		return err
	}
	// Another caller may have completed the reset while this caller waited.
	// Never replace that newly healthy database with the stale corruption report.
	if db, openErr := sql.Open("sqlite", "file:"+database+"?_pragma=busy_timeout(5000)"); openErr == nil {
		var integrity string
		checkErr := db.QueryRow(`PRAGMA quick_check`).Scan(&integrity)
		_ = db.Close()
		if checkErr == nil && integrity == "ok" {
			return nil
		}
	}
	backup := database + ".corrupt"
	if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(database, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(database + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func isOperationalCorruption(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"malformed", "not a database", "database disk image is malformed", "quick_check", "file is encrypted", "no such table", "no such column", "datatype mismatch", "constraint failed"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (store OperationalStore) Record(ctx context.Context, state CheckState) error {
	for attempt := 0; attempt < 2; attempt++ {
		db, err := store.Open(ctx)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `INSERT INTO source_check_state(source_id,last_checked_at,latency_ms,retry_count,availability,next_check_at,last_error)
VALUES(?,?,?,?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET last_checked_at=excluded.last_checked_at,latency_ms=excluded.latency_ms,retry_count=excluded.retry_count,availability=excluded.availability,next_check_at=excluded.next_check_at,last_error=excluded.last_error`,
			state.SourceID, state.LastCheckedAt.UTC().Format(time.RFC3339Nano), state.Latency.Milliseconds(), state.RetryCount, state.Availability, state.NextCheckAt.UTC().Format(time.RFC3339Nano), state.LastError)
		_ = db.Close()
		if err == nil {
			return nil
		}
		if !isOperationalCorruption(err) || attempt != 0 {
			return err
		}
		if err := store.resetCorrupt(); err != nil {
			return err
		}
	}
	return nil
}

func (store OperationalStore) Get(ctx context.Context, sourceID string) (CheckState, bool, error) {
	db, err := store.Open(ctx)
	if err != nil {
		if isOperationalCorruption(err) {
			return CheckState{}, false, nil
		}
		return CheckState{}, false, err
	}
	var state CheckState
	var checked, next string
	var latency int64
	err = db.QueryRowContext(ctx, `SELECT source_id,last_checked_at,latency_ms,retry_count,availability,next_check_at,last_error FROM source_check_state WHERE source_id=?`, sourceID).Scan(&state.SourceID, &checked, &latency, &state.RetryCount, &state.Availability, &next, &state.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		_ = db.Close()
		return CheckState{}, false, nil
	}
	if err != nil {
		return store.emptyAfterCorruption(db, err)
	}
	state.LastCheckedAt, err = time.Parse(time.RFC3339Nano, checked)
	if err != nil {
		return store.emptyAfterCorruption(db, fmt.Errorf("malformed operational timestamp: %w", err))
	}
	state.NextCheckAt, err = time.Parse(time.RFC3339Nano, next)
	if err != nil {
		return store.emptyAfterCorruption(db, fmt.Errorf("malformed operational timestamp: %w", err))
	}
	state.Latency = time.Duration(latency) * time.Millisecond
	if err := db.Close(); err != nil {
		return CheckState{}, false, err
	}
	return state, true, nil
}

func (store OperationalStore) emptyAfterCorruption(db *sql.DB, cause error) (CheckState, bool, error) {
	_ = db.Close()
	if !isOperationalCorruption(cause) {
		return CheckState{}, false, cause
	}
	if err := store.resetCorrupt(); err != nil {
		return CheckState{}, false, err
	}
	return CheckState{}, false, nil
}

func (store OperationalStore) List(ctx context.Context) ([]CheckState, error) {
	db, err := store.Open(ctx)
	if err != nil {
		if isOperationalCorruption(err) {
			return []CheckState{}, nil
		}
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT source_id,last_checked_at,latency_ms,retry_count,availability,next_check_at,last_error FROM source_check_state ORDER BY source_id`)
	if err != nil {
		if isOperationalCorruption(err) {
			_ = db.Close()
			if resetErr := store.resetCorrupt(); resetErr != nil {
				return nil, resetErr
			}
			return []CheckState{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	states := []CheckState{}
	for rows.Next() {
		var state CheckState
		var checked, next string
		var latency int64
		if err := rows.Scan(&state.SourceID, &checked, &latency, &state.RetryCount, &state.Availability, &next, &state.LastError); err != nil {
			_ = rows.Close()
			return store.listAfterCorruption(db, err)
		}
		state.LastCheckedAt, err = time.Parse(time.RFC3339Nano, checked)
		if err != nil {
			_ = rows.Close()
			return store.listAfterCorruption(db, fmt.Errorf("malformed operational timestamp: %w", err))
		}
		state.NextCheckAt, err = time.Parse(time.RFC3339Nano, next)
		if err != nil {
			_ = rows.Close()
			return store.listAfterCorruption(db, fmt.Errorf("malformed operational timestamp: %w", err))
		}
		state.Latency = time.Duration(latency) * time.Millisecond
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return store.listAfterCorruption(db, err)
	}
	return states, nil
}

func (store OperationalStore) listAfterCorruption(db *sql.DB, cause error) ([]CheckState, error) {
	if !isOperationalCorruption(cause) {
		return nil, cause
	}
	_ = db.Close()
	if err := store.resetCorrupt(); err != nil {
		return nil, err
	}
	return []CheckState{}, nil
}
