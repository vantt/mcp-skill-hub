package catalog

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/version"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"io/fs"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// BuildCatalogGeneration creates, verifies, and atomically publishes one immutable catalog.
func BuildCatalogGeneration(ctx context.Context, root string, options BuildOptions) (BuildResult, error) {
	return buildCatalogGeneration(ctx, root, options, false)
}

// BuildCatalogGenerationWhileLocked publishes the exact expected snapshot while
// its caller holds the exclusive mutation lock. It is intended for a mutation
// post-canonical callback and deliberately tolerates that transaction's WAL.
func BuildCatalogGenerationWhileLocked(ctx context.Context, root, expectedCatalogSnapshot string, options BuildOptions) (BuildResult, error) {
	if expectedCatalogSnapshot == "" {
		return BuildResult{}, errors.New("expected catalog snapshot is required")
	}
	// A source-only mutation can intentionally keep the routing catalog snapshot
	// unchanged while changing the projection input digest. Always rebuild here;
	// catalog-snapshot equality alone cannot prove the published projection is current.
	options.ExpectedCatalogSnapshot = expectedCatalogSnapshot
	return buildCatalogGeneration(ctx, root, options, true)
}

func buildCatalogGeneration(ctx context.Context, root string, options BuildOptions, lockHeld bool) (BuildResult, error) {
	options.report("capture", 0, 5, "Reading and validating canonical files")
	var input buildInput
	var err error
	if lockHeld {
		input, err = captureInputWhileLocked(ctx, root, true)
	} else {
		input, err = captureInput(ctx, root)
	}
	if err != nil {
		return BuildResult{}, err
	}
	if options.ExpectedCatalogSnapshot != "" && input.CatalogSnapshot != options.ExpectedCatalogSnapshot {
		return BuildResult{}, fmt.Errorf("%w: canonical result does not match transaction snapshot", ErrCanonicalChanged)
	}
	options.report("capture", 1, 5, "Canonical files captured")
	builderVersion := options.BuilderVersion
	if builderVersion == "" {
		builderVersion = version.Current().Version
		if builderVersion == "" {
			builderVersion = defaultBuilderVersion
		}
	}
	generation, err := newGenerationID()
	if err != nil {
		return BuildResult{}, err
	}
	if err := ensureRuntimeDirectory(root, "runtime/catalog/generations"); err != nil {
		return BuildResult{}, err
	}
	databasePath := filepath.Join(root, "runtime", "catalog", "generations", generation+".db")
	counts := expectedRowCounts(input)
	options.report("build", 1, 5, "Building immutable catalog generation")
	if err := buildDatabase(ctx, databasePath, input, builderVersion, counts, options); err != nil {
		removeGeneration(databasePath)
		return BuildResult{}, err
	}
	mayBePublished := false
	defer func() {
		if !mayBePublished {
			removeGeneration(databasePath)
		}
	}()
	options.report("build", 2, 5, "Catalog generation built")
	if err := waitForContext(ctx); err != nil {
		return BuildResult{}, err
	}
	if options.BeforeVerify != nil {
		if err := options.BeforeVerify(databasePath); err != nil {
			return BuildResult{}, err
		}
	}
	options.report("verify", 2, 5, "Verifying catalog integrity")
	if err := verifyGeneration(ctx, databasePath, input, builderVersion, counts); err != nil {
		return BuildResult{}, err
	}
	options.report("verify", 3, 5, "Catalog integrity verified")
	if err := os.Chmod(databasePath, 0o400); err != nil {
		return BuildResult{}, err
	}
	if options.BeforePublish != nil {
		if err := options.BeforePublish(); err != nil {
			return BuildResult{}, err
		}
	}
	if err := waitForContext(ctx); err != nil {
		return BuildResult{}, err
	}
	warnings := append(ensureDisposableDatabases(ctx, root), input.Warnings...)
	if err := waitForContext(ctx); err != nil {
		return BuildResult{}, err
	}
	pointer := Pointer{
		Generation: generation, Database: "generations/" + generation + ".db",
		CatalogSnapshot: input.CatalogSnapshot, ProjectionInputDigest: input.ProjectionInputDigest,
		CanonicalSchemaVersion: input.CanonicalSchemaVersion, DerivedSchemaVersion: DerivedSchemaVersion,
		BuilderVersion: builderVersion,
	}
	options.report("publish", 3, 5, "Waiting to publish verified generation")
	var lock *mutation.WorkspaceLock
	if !lockHeld {
		lock, err = mutation.AcquireExclusiveLock(ctx, root, mutation.DefaultLockTimeout)
		if err != nil {
			return BuildResult{}, err
		}
	}
	var publishWarning string
	publish := func() error {
		if err := waitForContext(ctx); err != nil {
			return err
		}
		if !lockHeld {
			if recoveries, err := mutation.InspectRecoveryWhileLocked(root); err != nil {
				return err
			} else if len(recoveries) != 0 {
				return mutation.ErrRecoveryRequired
			}
		}
		// Final canonical validation and inventory occur immediately before swap.
		issues, err := canonical.Validate(root)
		if err != nil {
			return err
		}
		if len(issues) != 0 {
			return fmt.Errorf("%w: %s: %s", ErrCanonicalChanged, issues[0].Path, issues[0].Message)
		}
		current, err := readInput(root)
		if err != nil {
			return err
		}
		if current.ProjectionInputDigest != input.ProjectionInputDigest || current.CatalogSnapshot != input.CatalogSnapshot || !sameInputFiles(current.Files, input.Files) {
			return fmt.Errorf("%w: canonical inputs changed before publish", ErrCanonicalChanged)
		}
		if err := verifyGeneration(ctx, databasePath, input, builderVersion, counts); err != nil {
			return err
		}
		// This is the final safe cancellation point: after pointer replacement the
		// verified generation may already be visible and must be reported as published.
		if err := waitForContext(ctx); err != nil {
			return err
		}
		renamed, err := writePointer(root, pointer, options)
		if renamed {
			mayBePublished = true
			if err != nil {
				publishWarning = "catalog pointer was published, but directory synchronization failed; publication durability is indeterminate: " + err.Error()
				return nil
			}
		}
		return err
	}
	var publishErr error
	if lockHeld {
		publishErr = publish()
	} else {
		publishErr = publish()
		if unlockErr := lock.Unlock(); publishErr == nil && unlockErr != nil {
			publishErr = unlockErr
		}
	}
	if publishErr != nil {
		return BuildResult{}, publishErr
	}
	mayBePublished = true
	options.report("publish", 4, 5, "Catalog generation published")
	if options.AfterPointerPublish != nil {
		options.AfterPointerPublish()
	}
	result := BuildResult{Pointer: pointer, RowCounts: counts, Freshness: FreshnessCurrent, Warnings: warnings}
	if publishWarning != "" {
		result.Freshness = FreshnessIndeterminate
		result.FreshnessDetail = publishWarning
		result.Warnings = append(result.Warnings, publishWarning)
	} else {
		post, postErr := canonical.Scan(root)
		if postErr != nil {
			result.Freshness = FreshnessIndeterminate
			result.FreshnessDetail = postErr.Error()
		} else if post.CatalogSnapshot != input.CatalogSnapshot || post.ProjectionInputDigest != input.ProjectionInputDigest {
			result.Freshness = FreshnessStale
			result.FreshnessDetail = "canonical inputs changed after pointer publication"
		}
	}
	minimumAge := options.GCMinimumAge
	if minimumAge <= 0 {
		minimumAge = defaultGCMinimumAge
	}
	// A mutation callback already holds the workspace's exclusive lock. Garbage
	// collection acquires that same lock, so defer it to a later ordinary rebuild
	// rather than blocking every atomic publish until the lock timeout.
	if !lockHeld {
		if err := CollectGarbage(root, minimumAge); err != nil {
			result.Warnings = append(result.Warnings, "catalog garbage collection failed: "+err.Error())
		}
	}
	options.report("complete", 5, 5, "Catalog rebuild complete")
	return result, nil
}

func buildDatabase(ctx context.Context, path string, input buildInput, builderVersion string, counts map[string]int64, options BuildOptions) error {
	database, err := sql.Open("sqlite", sqliteDSN(path, false))
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	closed := false
	defer func() {
		if !closed {
			_ = database.Close()
		}
	}()
	if _, err := database.ExecContext(ctx, `PRAGMA foreign_keys=ON; PRAGMA journal_mode=DELETE; PRAGMA synchronous=FULL;`); err != nil {
		return fmt.Errorf("configure generation database: %w", err)
	}
	if _, err := database.ExecContext(ctx, catalogSchema); err != nil {
		return fmt.Errorf("create generation schema: %w", err)
	}
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := populate(ctx, transaction, input, builderVersion, counts, options); err != nil {
		_ = transaction.Rollback()
		return err
	}
	foreignKeys, err := transaction.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		_ = transaction.Rollback()
		return err
	}
	if foreignKeys.Next() {
		var table, parent string
		var rowID, constraint int64
		_ = foreignKeys.Scan(&table, &rowID, &parent, &constraint)
		_ = foreignKeys.Close()
		_ = transaction.Rollback()
		return fmt.Errorf("catalog foreign key violation: %s row %d references %s", table, rowID, parent)
	}
	if err := foreignKeys.Close(); err != nil {
		_ = transaction.Rollback()
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	if err := options.fail(FaultDatabaseClose); err != nil {
		return err
	}
	if err := database.Close(); err != nil {
		return err
	}
	closed = true
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		file, err = os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			return err
		}
	}
	if err = options.fail(FaultGenerationSync); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := options.fail(FaultGenerationsDirSync); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return nil
}

func verifyGeneration(ctx context.Context, path string, input buildInput, builderVersion string, expectedCounts map[string]int64) error {
	database, err := sql.Open("sqlite", sqliteDSN(path, true))
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	defer database.Close()
	var integrity string
	if err := database.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		if err == nil {
			err = fmt.Errorf("integrity_check returned %q", integrity)
		}
		return fmt.Errorf("catalog integrity check failed: %w", err)
	}
	rows, err := database.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("catalog foreign key check failed: %w", err)
	}
	if rows.Next() {
		rows.Close()
		return fmt.Errorf("catalog foreign key check reported a violation")
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var canonicalVersion, derivedVersion int
	var actualBuilder, catalogDigest, projectionDigest string
	if err := database.QueryRowContext(ctx, `SELECT canonical_schema_version,derived_schema_version,builder_version,catalog_snapshot,projection_input_digest FROM generation_metadata WHERE singleton=1`).Scan(&canonicalVersion, &derivedVersion, &actualBuilder, &catalogDigest, &projectionDigest); err != nil {
		return fmt.Errorf("read generation metadata: %w", err)
	}
	if canonicalVersion != input.CanonicalSchemaVersion || derivedVersion != DerivedSchemaVersion || actualBuilder != builderVersion || catalogDigest != input.CatalogSnapshot || projectionDigest != input.ProjectionInputDigest {
		return fmt.Errorf("generation metadata does not match immutable build input")
	}
	actualCounts, err := queryRowCounts(database)
	if err != nil {
		return err
	}
	for _, table := range countedTables {
		if actualCounts[table] != expectedCounts[table] {
			return fmt.Errorf("generation row count mismatch for %s", table)
		}
		var stored int64
		if err := database.QueryRowContext(ctx, `SELECT row_count FROM generation_row_counts WHERE table_name=?`, table).Scan(&stored); err != nil || stored != expectedCounts[table] {
			return fmt.Errorf("stored generation row count mismatch for %s", table)
		}
	}
	seenKind := make(map[string]bool)
	for _, item := range input.Entities {
		var count int64
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM canonical_entities WHERE id=? AND path=? AND digest=?`, item.ID, item.Path, item.Digest).Scan(&count); err != nil || count != 1 {
			return fmt.Errorf("catalog identity smoke query failed for %s", item.Path)
		}
		table := map[string]string{"skill": "skills", "source": "sources", "observation": "findings", "finding": "findings", "comparison": "comparisons", "insight": "insights", "outcome": "outcomes", "operation": "operations", "run": "provenance", "proposal": "provenance", "incorporation": "provenance", "source_candidate": "provenance", "skill_source_link": "provenance", "routing_evaluation": "provenance"}[item.Kind]
		if table != "" && !seenKind[table] {
			if err := database.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` WHERE id=?`, item.ID).Scan(&count); err != nil || count != 1 {
				return fmt.Errorf("catalog representative query failed for %s", table)
			}
			seenKind[table] = true
		}
		if item.Kind == "skill" {
			if token := ftsToken(firstString(item.Document, "name", "description")); token != "" {
				if err := database.QueryRowContext(ctx, `SELECT count(*) FROM skill_fts WHERE skill_id=? AND skill_fts MATCH ?`, item.ID, `"`+token+`"`).Scan(&count); err != nil || count < 1 {
					return fmt.Errorf("skill FTS smoke query failed for %s", item.ID)
				}
			}
		} else if item.Kind != "operation" {
			if token := ftsToken(item.SearchText); token != "" {
				if err := database.QueryRowContext(ctx, `SELECT count(*) FROM curation_fts WHERE entity_id=? AND curation_fts MATCH ?`, item.ID, `"`+token+`"`).Scan(&count); err != nil || count < 1 {
					return fmt.Errorf("curation FTS smoke query failed for %s", item.ID)
				}
			}
		}
	}
	for _, file := range input.Files {
		if strings.HasPrefix(file.Path, "skills/") && !workspace.IsHubMeta(file.Path) && searchableResource(file.Path) {
			token := ftsToken(string(file.Bytes))
			if token == "" {
				continue
			}
			var count int64
			if err := database.QueryRowContext(ctx, `SELECT count(*) FROM resource_fts WHERE path=? AND resource_fts MATCH ?`, file.Path, `"`+token+`"`).Scan(&count); err != nil || count < 1 {
				return fmt.Errorf("resource FTS smoke query failed for %s", file.Path)
			}
			break
		}
	}
	return nil
}

func ftsToken(value string) string {
	for _, field := range strings.Fields(value) {
		token := strings.TrimFunc(field, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if token != "" {
			return token
		}
	}
	return ""
}

func ensureDisposableDatabases(ctx context.Context, root string) []string {
	var warnings []string
	if err := ensureRuntimeDirectory(root, "runtime"); err != nil {
		return []string{"disposable database directory unavailable: " + err.Error()}
	}
	for _, item := range []struct {
		name   string
		schema string
	}{
		{"operational.db", `CREATE TABLE IF NOT EXISTS operational_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL) STRICT;`},
		{"telemetry.db", `CREATE TABLE IF NOT EXISTS telemetry_events (id TEXT PRIMARY KEY, occurred_at TEXT NOT NULL, kind TEXT NOT NULL, payload_json TEXT NOT NULL) STRICT;`},
	} {
		path := filepath.Join(root, "runtime", item.name)
		if info, err := os.Lstat(path); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			warnings = append(warnings, "unsafe "+item.name+" was not opened")
			continue
		} else if err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, item.name+": "+err.Error())
			continue
		}
		if err := validateDisposableDatabase(ctx, path, item.schema); err == nil {
			continue
		} else {
			warnings = append(warnings, item.name+" was missing or invalid and was reset: "+err.Error())
		}
		// Disposable databases are not reconstructed. Invalid bytes are removed
		// and a fresh empty schema is created; telemetry history is intentionally lost.
		_ = os.Remove(path + "-wal")
		_ = os.Remove(path + "-shm")
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, "could not reset "+item.name+": "+err.Error())
			continue
		}
		if err := createDisposableDatabase(ctx, path, item.schema); err != nil {
			warnings = append(warnings, "could not create "+item.name+": "+err.Error())
		}
	}
	return warnings
}

func validateDisposableDatabase(ctx context.Context, path, schema string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	database, err := sql.Open("sqlite", sqliteDSN(path, false))
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	var check string
	queryErr := database.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check)
	if queryErr == nil && check != "ok" {
		queryErr = fmt.Errorf("quick_check returned %q", check)
	}
	if queryErr == nil {
		_, queryErr = database.ExecContext(ctx, schema)
	}
	closeErr := database.Close()
	if queryErr != nil {
		return queryErr
	}
	return closeErr
}

func createDisposableDatabase(ctx context.Context, path, schema string) error {
	database, err := sql.Open("sqlite", sqliteDSN(path, false))
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	_, execErr := database.ExecContext(ctx, `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; `+schema)
	closeErr := database.Close()
	if execErr != nil {
		return execErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(path, 0o600)
}

func writePointer(root string, pointer Pointer, options BuildOptions) (bool, error) {
	if err := ensureRuntimeDirectory(root, "runtime/catalog"); err != nil {
		return false, err
	}
	contents, err := json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		return false, err
	}
	contents = append(contents, '\n')
	directory := filepath.Join(root, "runtime", "catalog")
	temporary, err := os.CreateTemp(directory, ".current-*.json")
	if err != nil {
		return false, err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(contents)
	}
	if err == nil {
		err = options.fail(FaultPointerTempSync)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err := options.fail(FaultPointerRename); err != nil {
		return false, err
	}
	if err := atomicReplace(name, filepath.Join(directory, "current.json")); err != nil {
		return false, err
	}
	if err := options.fail(FaultPointerDirSync); err != nil {
		return true, err
	}
	return true, syncDirectory(directory)
}

func ensureRuntimeDirectory(root, relative string) error {
	current := root
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("invalid runtime directory %s", relative)
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe runtime directory: %s", current)
		}
	}
	return nil
}

func sqliteDSN(path string, readOnly bool) string {
	p := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	value := (&url.URL{Scheme: "file", Path: p}).String()
	if readOnly {
		value += "?mode=ro&immutable=1"
	}
	return value
}

func newGenerationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "gen-" + hex.EncodeToString(value[:]), nil
}

func removeGeneration(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + "-journal")
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return err
	}
	return nil
}
