package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS telemetry_events (
  id TEXT PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  kind TEXT NOT NULL,
  resolution_id TEXT,
  payload_json TEXT NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS telemetry_events_time ON telemetry_events(occurred_at, id);
CREATE TABLE IF NOT EXISTS telemetry_cases (
  case_id TEXT PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  kind TEXT NOT NULL,
  session_hash TEXT,
  resolution_id TEXT,
  event_id TEXT,
  payload_json TEXT NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS telemetry_cases_time ON telemetry_cases(occurred_at, case_id);
CREATE TABLE IF NOT EXISTS telemetry_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
) STRICT;
INSERT OR REPLACE INTO telemetry_meta(key,value) VALUES('event_version','1');`

var (
	initializeBeforeOpen  = func() error { return nil }
	storeBeforeSQLiteOpen = func() error { return nil }
	purgeBeforeRemove     = func() error { return nil }
)

type storeAnchor struct {
	root          *os.Root
	directory     *os.File
	directoryPath string
	databaseName  string
	sqlitePath    string
	closeOnce     sync.Once
	closeErr      error
}

func (anchor *storeAnchor) close() error {
	if anchor == nil {
		return nil
	}
	anchor.closeOnce.Do(func() {
		rootErr := anchor.root.Close()
		directoryErr := anchor.directory.Close()
		if rootErr != nil {
			anchor.closeErr = rootErr
		} else {
			anchor.closeErr = directoryErr
		}
	})
	return anchor.closeErr
}

func normalizeStoreConfig(config Config) (Config, error) {
	path, err := filepath.Abs(config.Path)
	if err != nil {
		return Config{}, fmt.Errorf("resolve telemetry path: %w", err)
	}
	explicitRoot := config.WorkspaceRoot != ""
	root := config.WorkspaceRoot
	if !explicitRoot {
		root = filepath.Dir(path)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Config{}, fmt.Errorf("resolve telemetry workspace root: %w", err)
	}
	config.Path = filepath.Clean(path)
	config.WorkspaceRoot = filepath.Clean(root)
	config.storeAnchor, err = openStoreAnchor(config)
	if err != nil {
		if explicitRoot {
			return Config{}, err
		}
		// An implicit default path may be unavailable (read-only parent, a
		// non-directory component, and so on). Preserve telemetry's failure
		// isolation without ever attempting SQLite through an unanchored path.
		config.storeAnchorErr = err
		return config, nil
	}
	return config, nil
}

func initialize(config Config) error {
	if err := initializeBeforeOpen(); err != nil {
		return fmt.Errorf("initialize telemetry: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.OperationTimeout)
	defer cancel()
	if err := maintainStore(ctx, config); err != nil {
		return fmt.Errorf("initialize telemetry: %w", err)
	}
	return nil
}

func openDatabase(ctx context.Context, config Config) (*sql.DB, error) {
	anchor := config.storeAnchor
	if anchor == nil {
		if config.storeAnchorErr != nil {
			return nil, config.storeAnchorErr
		}
		return nil, errors.New("telemetry store is not anchored")
	}
	if err := anchor.ensureCurrent(config); err != nil {
		return nil, err
	}
	if err := storeBeforeSQLiteOpen(); err != nil {
		return nil, err
	}
	if err := anchor.validateIdentity(); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", sqliteURL(anchor.sqlitePath))
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	closeOnError := func(cause error) (*sql.DB, error) { _ = database.Close(); return nil, cause }
	for _, pragma := range []string{
		"PRAGMA busy_timeout=5000",
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := database.ExecContext(ctx, pragma); err != nil {
			return closeOnError(err)
		}
	}
	var check string
	if err := database.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check); err != nil {
		return closeOnError(err)
	}
	if check != "ok" {
		return closeOnError(fmt.Errorf("quick_check returned %q", check))
	}
	if _, err := database.ExecContext(ctx, schema); err != nil {
		return closeOnError(err)
	}
	if _, err := database.ExecContext(ctx, rollupSchema); err != nil {
		return closeOnError(err)
	}
	if err := ensureResolutionColumn(ctx, database); err != nil {
		return closeOnError(err)
	}
	file, err := anchor.root.OpenFile(anchor.databaseName, os.O_RDWR, 0)
	if err != nil {
		return closeOnError(err)
	}
	chmodErr := file.Chmod(0o600)
	fileCloseErr := file.Close()
	if chmodErr != nil {
		return closeOnError(chmodErr)
	}
	if fileCloseErr != nil {
		return closeOnError(fileCloseErr)
	}
	return database, nil
}

func ensureResolutionColumn(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, `PRAGMA table_info(telemetry_events)`)
	if err != nil {
		return err
	}
	hasColumn := false
	for rows.Next() {
		var sequence int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if name == "resolution_id" {
			hasColumn = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !hasColumn {
		if _, err := database.ExecContext(ctx, `ALTER TABLE telemetry_events ADD COLUMN resolution_id TEXT`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
		// Older rows remain disposable telemetry, but valid envelopes can still be
		// promoted after the index column is derived from their sanitized JSON.
		if _, err := database.ExecContext(ctx, `UPDATE telemetry_events SET resolution_id = CASE WHEN json_valid(payload_json) THEN CASE WHEN json_type(payload_json, '$.resolution_id') = 'text' THEN json_extract(payload_json, '$.resolution_id') ELSE NULL END ELSE NULL END`); err != nil {
			return err
		}
	}
	if _, err = database.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS telemetry_events_resolution ON telemetry_events(resolution_id, occurred_at, id)`); err != nil {
		return err
	}
	return removeLegacyFeedback(ctx, database)
}

func removeLegacyFeedback(ctx context.Context, database *sql.DB) error {
	const migrationKey = "legacy_skill_feedback_removed"
	var applied string
	err := database.QueryRowContext(ctx, `SELECT value FROM telemetry_meta WHERE key = ?`, migrationKey).Scan(&applied)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()
	// Older builds wrote an unversioned raw payload under skill.feedback.
	// Telemetry is disposable, so remove those incompatible rows instead of
	// exporting or promoting data that cannot satisfy the privacy envelope.
	if _, err := transaction.ExecContext(ctx, `DELETE FROM telemetry_events WHERE kind = 'skill.feedback'`); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `INSERT OR IGNORE INTO telemetry_meta(key,value) VALUES(?,?)`, migrationKey, "1"); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func openStoreAnchor(config Config) (*storeAnchor, error) {
	rootInfo, err := os.Lstat(config.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect telemetry workspace root: %w", err)
	}
	if rootInfo.Mode()&fs.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, errors.New("unsafe telemetry workspace root")
	}
	relative, err := filepath.Rel(config.WorkspaceRoot, config.Path)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("telemetry database path escapes workspace root")
	}
	workspace, err := os.OpenRoot(config.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("open telemetry workspace root: %w", err)
	}
	openedRootInfo, err := workspace.Stat(".")
	if err != nil {
		_ = workspace.Close()
		return nil, fmt.Errorf("inspect opened telemetry workspace root: %w", err)
	}
	if !os.SameFile(rootInfo, openedRootInfo) {
		_ = workspace.Close()
		return nil, errors.New("telemetry workspace root identity changed")
	}
	failWorkspace := func(cause error) (*storeAnchor, error) {
		_ = workspace.Close()
		return nil, cause
	}
	parent := filepath.Dir(relative)
	if err := rejectSymlinkComponents(workspace, parent); err != nil {
		return failWorkspace(err)
	}
	if err := workspace.MkdirAll(parent, 0o700); err != nil {
		return failWorkspace(fmt.Errorf("create telemetry directory: %w", err))
	}
	if err := rejectSymlinkComponents(workspace, parent); err != nil {
		return failWorkspace(err)
	}
	if info, err := workspace.Lstat(relative); err == nil {
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return failWorkspace(errors.New("unsafe telemetry database path"))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return failWorkspace(fmt.Errorf("inspect telemetry database path: %w", err))
	}

	root, err := workspace.OpenRoot(parent)
	if err != nil {
		return failWorkspace(fmt.Errorf("anchor telemetry directory: %w", err))
	}
	directoryPath := filepath.Join(config.WorkspaceRoot, parent)
	// Use os.Open in addition to os.Root so Windows obtains a directory handle
	// without FILE_SHARE_DELETE. validateHandles below rejects a path swap that
	// occurs between the root-relative open and this absolute open.
	directory, err := os.Open(directoryPath)
	if err != nil {
		_ = root.Close()
		return failWorkspace(fmt.Errorf("open telemetry directory handle: %w", err))
	}
	if err := workspace.Close(); err != nil {
		_ = root.Close()
		_ = directory.Close()
		return nil, err
	}
	anchor := &storeAnchor{
		root:          root,
		directory:     directory,
		directoryPath: directoryPath,
		databaseName:  filepath.Base(relative),
	}
	if err := anchor.validateHandles(); err != nil {
		_ = anchor.close()
		return nil, err
	}
	anchor.sqlitePath, err = anchorSQLitePath(directory, anchor.directoryPath, anchor.databaseName)
	if err != nil {
		_ = anchor.close()
		return nil, err
	}
	if err := anchor.validateIdentity(); err != nil {
		_ = anchor.close()
		return nil, err
	}
	return anchor, nil
}

func (anchor *storeAnchor) validateHandles() error {
	directoryInfo, err := anchor.directory.Stat()
	if err != nil {
		return fmt.Errorf("inspect telemetry directory handle: %w", err)
	}
	rootInfo, err := anchor.root.Stat(".")
	if err != nil {
		return fmt.Errorf("inspect anchored telemetry root: %w", err)
	}
	if !directoryInfo.IsDir() || !rootInfo.IsDir() || !os.SameFile(directoryInfo, rootInfo) {
		return errors.New("telemetry directory handles do not identify the same directory")
	}
	return nil
}

func (anchor *storeAnchor) validateIdentity() error {
	if err := anchor.validateHandles(); err != nil {
		return err
	}
	pathInfo, err := os.Lstat(anchor.directoryPath)
	if err != nil {
		return fmt.Errorf("revalidate telemetry directory: %w", err)
	}
	if pathInfo.Mode()&fs.ModeSymlink != 0 || !pathInfo.IsDir() {
		return errors.New("telemetry directory identity changed")
	}
	directoryInfo, err := anchor.directory.Stat()
	if err != nil {
		return fmt.Errorf("inspect telemetry directory handle: %w", err)
	}
	if !os.SameFile(pathInfo, directoryInfo) {
		return errors.New("telemetry directory identity changed")
	}
	return nil
}

// ensureCurrent validates the anchored directory and, when the directory was
// deleted or replaced (for example by removing runtime/), re-anchors once with
// the same confinement checks instead of leaving a long-lived recorder broken.
// The store is only touched from the recorder goroutine, so swapping the
// anchor's handles in place is race free.
func (anchor *storeAnchor) ensureCurrent(config Config) error {
	if anchor.validateIdentity() == nil {
		return nil
	}
	fresh, err := openStoreAnchor(config)
	if err != nil {
		return err
	}
	_ = anchor.close()
	anchor.root, anchor.directory = fresh.root, fresh.directory
	anchor.directoryPath, anchor.databaseName, anchor.sqlitePath = fresh.directoryPath, fresh.databaseName, fresh.sqlitePath
	anchor.closeOnce = sync.Once{}
	anchor.closeErr = nil
	return nil
}

func anchorSQLitePath(directory *os.File, directoryPath, databaseName string) (string, error) {
	if runtime.GOOS == "windows" {
		// os.Open directory handles on Windows omit FILE_SHARE_DELETE. Keeping
		// this handle open prevents the validated directory from being renamed;
		// every database open also revalidates the directory identity.
		return filepath.Join(directoryPath, databaseName), nil
	}
	var candidates []string
	switch runtime.GOOS {
	case "linux":
		candidates = []string{fmt.Sprintf("/proc/self/fd/%d", directory.Fd())}
	case "aix", "darwin", "dragonfly", "freebsd", "netbsd", "openbsd", "solaris":
		candidates = []string{fmt.Sprintf("/dev/fd/%d", directory.Fd())}
	default:
		return "", fmt.Errorf("telemetry directory anchoring is unsupported on %s", runtime.GOOS)
	}
	directoryInfo, err := directory.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect telemetry directory handle: %w", err)
	}
	for _, candidate := range candidates {
		candidateInfo, statErr := os.Stat(candidate)
		if statErr == nil && candidateInfo.IsDir() && os.SameFile(directoryInfo, candidateInfo) {
			return filepath.Join(candidate, databaseName), nil
		}
	}
	return filepath.Join(directoryPath, databaseName), nil
}

func rejectSymlinkComponents(root *os.Root, relative string) error {
	if relative == "." || relative == "" {
		return nil
	}
	current := ""
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect telemetry path component %s: %w", current, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe telemetry path component: %s", current)
		}
	}
	return nil
}

func sqliteURL(path string) string {
	p := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{
		Scheme:   "file",
		Path:     p,
		RawQuery: "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)",
	}).String()
}

func writeEvents(ctx context.Context, config Config, events []storedEnvelope) error {
	database, err := openDatabase(ctx, config)
	if err != nil {
		return err
	}
	defer database.Close()
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()
	for _, event := range events {
		if _, err := insertEvent(ctx, transaction, event, insertEventOrIgnoreSQL); err != nil {
			return err
		}
	}
	cutoff := formatStoredTime(config.Clock().Add(-config.Retention))
	if _, err := transaction.ExecContext(ctx, `DELETE FROM telemetry_events WHERE occurred_at < ?`, cutoff); err != nil {
		return err
	}
	if err := pruneRollups(ctx, transaction, config); err != nil {
		return err
	}
	if err := trimLogicalSize(ctx, transaction, config.MaxSizeBytes); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	committed = true
	return trimPhysicalSize(ctx, database, config.Path, config.MaxSizeBytes)
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func trimLogicalSize(ctx context.Context, transaction *sql.Tx, maximum int64) error {
	for {
		var size int64
		if err := transaction.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(id)+length(occurred_at)+length(kind)+COALESCE(length(resolution_id),0)+length(payload_json)),0) FROM telemetry_events`).Scan(&size); err != nil {
			return err
		}
		if size <= maximum {
			return nil
		}
		result, err := transaction.ExecContext(ctx, `DELETE FROM telemetry_events WHERE id IN (SELECT id FROM telemetry_events ORDER BY occurred_at,id LIMIT 100)`)
		if err != nil {
			return err
		}
		removed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if removed == 0 {
			return nil
		}
	}
}

func trimPhysicalSize(ctx context.Context, database *sql.DB, _ string, maximum int64) error {
	size, err := sqliteStorageSize(ctx, database)
	if err != nil {
		return err
	}
	if size <= maximum {
		return nil
	}
	if _, err := database.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	if _, err := database.ExecContext(ctx, `VACUUM`); err != nil {
		return err
	}
	_, err = database.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

func sqliteStorageSize(ctx context.Context, database *sql.DB) (int64, error) {
	var pageSize, pageCount int64
	if err := database.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return 0, err
	}
	if err := database.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount); err != nil {
		return 0, err
	}
	var busy, walPages, checkpointed int64
	if err := database.QueryRowContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`).Scan(&busy, &walPages, &checkpointed); err != nil {
		return 0, err
	}
	size := pageSize * pageCount
	if walPages > 0 {
		// A WAL contains a 32-byte header and a 24-byte header per page. The
		// shared-memory index is allocated in 32 KiB regions. This conservative
		// estimate avoids consulting the caller-visible absolute path after the
		// store has been anchored.
		size += 32 + walPages*(pageSize+24) + 32*1024
	}
	return size, nil
}

func databaseFilesSize(path string) (int64, error) {
	var total int64
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func maintainStore(ctx context.Context, config Config) error {
	database, err := openDatabase(ctx, config)
	if err != nil {
		return err
	}
	defer database.Close()
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()
	cutoff := formatStoredTime(config.Clock().Add(-config.Retention))
	if _, err := transaction.ExecContext(ctx, `DELETE FROM telemetry_events WHERE occurred_at < ?`, cutoff); err != nil {
		return err
	}
	if err := pruneRollups(ctx, transaction, config); err != nil {
		return err
	}
	caseRetention := config.CaseRetention
	if caseRetention <= 0 {
		caseRetention = DefaultCaseRetention
	}
	caseCutoff := config.Clock().Add(-caseRetention).UTC().Format(time.RFC3339)
	if _, err := transaction.ExecContext(ctx, `DELETE FROM telemetry_cases WHERE occurred_at < ?`, caseCutoff); err != nil {
		return err
	}
	if err := trimLogicalSize(ctx, transaction, config.MaxSizeBytes); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	committed = true
	return trimPhysicalSize(ctx, database, config.Path, config.MaxSizeBytes)
}

func purgeStore(ctx context.Context, config Config) error {
	anchor := config.storeAnchor
	if anchor == nil {
		if config.storeAnchorErr != nil {
			return config.storeAnchorErr
		}
		return errors.New("telemetry store is not anchored")
	}
	if err := anchor.ensureCurrent(config); err != nil {
		return err
	}
	if err := purgeBeforeRemove(); err != nil {
		return err
	}
	if err := anchor.validateIdentity(); err != nil {
		return err
	}
	for _, candidate := range []string{anchor.databaseName + "-wal", anchor.databaseName + "-shm", anchor.databaseName} {
		if err := anchor.root.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove telemetry store: %w", err)
		}
	}
	database, err := openDatabase(ctx, config)
	if err != nil {
		return err
	}
	return database.Close()
}

var counterMetaKeys = [...]string{"counter_accepted", "counter_written", "counter_dropped", "counter_rejected", "counter_errors"}

// persistCounters adds this recorder's counters to the cumulative totals kept in
// telemetry_meta so health remains meaningful across short-lived command recorders.
func persistCounters(ctx context.Context, config Config, deltas [len(counterMetaKeys)]uint64) error {
	var nonzero bool
	for _, delta := range deltas {
		nonzero = nonzero || delta != 0
	}
	if !nonzero {
		return nil
	}
	database, err := openDatabase(ctx, config)
	if err != nil {
		return err
	}
	defer database.Close()
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	for index, key := range counterMetaKeys {
		if deltas[index] == 0 {
			continue
		}
		if _, err := transaction.ExecContext(ctx, `INSERT INTO telemetry_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value = CAST(CAST(value AS INTEGER) + ? AS TEXT)`, key, fmt.Sprint(deltas[index]), int64(deltas[index])); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

// loadCounters reads the cumulative totals; missing or malformed values count as zero.
func loadCounters(ctx context.Context, config Config) ([len(counterMetaKeys)]uint64, error) {
	var totals [len(counterMetaKeys)]uint64
	database, err := openDatabase(ctx, config)
	if err != nil {
		return totals, err
	}
	defer database.Close()
	for index, key := range counterMetaKeys {
		var value string
		err := database.QueryRowContext(ctx, `SELECT value FROM telemetry_meta WHERE key = ?`, key).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return totals, err
		}
		if _, scanErr := fmt.Sscan(value, &totals[index]); scanErr != nil {
			totals[index] = 0
		}
	}
	return totals, nil
}
