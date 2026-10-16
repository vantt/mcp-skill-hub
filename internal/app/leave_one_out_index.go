package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
)

// exampleOmitter is a catalog that can answer a search as if one example of
// one skill were not in its index. Leave-one-out needs it: the phrase under
// test must not find its skill through its own text.
type exampleOmitter interface {
	searchOmitting(ctx context.Context, query string, limit int, skillID, phrase string) ([]resolverpkg.SearchHit, error)
}

// errCannotOmit marks a catalog whose search cannot leave an example out.
var errCannotOmit = errors.New("the catalog cannot search with an example left out of its index")

// exampleIndex is a private, writable in-memory copy of the generation's
// full-text skill index. To search without one example it rewrites that
// skill's examples column, runs the served search query, and writes the column
// back. Every other row, the index statistics and the query are those of the
// served index, so the hits, their order and their ranks are the ones a
// library built without that example would give. The copy is made on first use
// and only by leave-one-out runs; resolving real requests never touches it.
type exampleIndex struct {
	view *catalog.View

	mu       sync.Mutex
	db       *sql.DB
	search   *resolverpkg.SQLiteCatalog
	examples map[string][]string // skill id -> examples in index order
	err      error               // a failed copy or restore; the index is unusable
}

func newExampleIndex(view *catalog.View) *exampleIndex {
	return &exampleIndex{view: view}
}

// Close releases the copy, if one was made, and makes later searches fail.
// The copy is a private in-memory database that holds nothing to persist, so
// a failure to close it only leaves memory to the garbage collector and is not
// reported.
func (index *exampleIndex) Close() {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.db != nil {
		_ = index.db.Close()
	}
	index.db, index.search, index.examples = nil, nil, nil
	index.err = errors.New("the leave-one-out index is closed")
}

func (index *exampleIndex) searchOmitting(ctx context.Context, query string, limit int, skillID, phrase string) ([]resolverpkg.SearchHit, error) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if err := index.loadLocked(ctx); err != nil {
		return nil, err
	}
	examples, indexed := index.examples[skillID]
	if !indexed {
		return index.search.Search(ctx, query, limit)
	}
	remaining, found := withoutExample(examples, phrase)
	if !found {
		return nil, fmt.Errorf("example %q of %s is not in the full-text index", phrase, skillID)
	}
	if err := index.setExamples(ctx, skillID, remaining); err != nil {
		index.err = fmt.Errorf("leave an example of %s out of the copied index: %w", skillID, err)
		return nil, index.err
	}
	hits, searchErr := index.search.Search(ctx, query, limit)
	// Restore even when the caller gave up, so the copy stays usable.
	if err := index.setExamples(context.WithoutCancel(ctx), skillID, examples); err != nil {
		index.err = fmt.Errorf("restore the examples of %s in the copied index: %w", skillID, err)
		return nil, errors.Join(searchErr, index.err)
	}
	return hits, searchErr
}

func (index *exampleIndex) setExamples(ctx context.Context, skillID string, examples []string) error {
	_, err := index.db.ExecContext(ctx, `UPDATE skill_fts SET examples=? WHERE skill_id=?`, strings.Join(examples, " "), skillID)
	return err
}

func (index *exampleIndex) loadLocked(ctx context.Context) error {
	if index.err != nil {
		return index.err
	}
	if index.db != nil {
		return nil
	}
	if err := index.copyLocked(ctx); err != nil {
		index.err = fmt.Errorf("copy the full-text index for leave-one-out: %w", err)
		return index.err
	}
	return nil
}

func (index *exampleIndex) copyLocked(ctx context.Context) (err error) {
	source, err := index.view.DB(ctx)
	if err != nil {
		return err
	}
	var schema string
	if err := source.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='skill_fts'`).Scan(&schema); err != nil {
		return fmt.Errorf("read the full-text index schema: %w", err)
	}
	examples, err := indexedExamples(ctx, source)
	if err != nil {
		return err
	}

	// One connection keeps the in-memory database alive and private.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	defer func() {
		if err != nil {
			db.Close()
		}
	}()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // a no-op after Commit
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create the copied index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE skills(id TEXT PRIMARY KEY, status TEXT NOT NULL)`); err != nil {
		return err
	}
	if err := copyRows(ctx, source, tx, `SELECT id,status FROM skills`, `INSERT INTO skills(id,status) VALUES(?,?)`, nil); err != nil {
		return fmt.Errorf("copy skill statuses: %w", err)
	}
	// The examples column must be the skill's examples joined in index order,
	// as the catalog build writes it; otherwise rewriting it would change more
	// than the one example.
	check := func(row []any) error {
		id, stored := columnText(row[1]), columnText(row[6])
		if joined := strings.Join(examples[id], " "); joined != stored {
			return fmt.Errorf("the indexed examples of %s are not its routing examples in order", id)
		}
		return nil
	}
	if err := copyRows(ctx, source, tx,
		`SELECT rowid,skill_id,name,aliases,description,triggers,examples,keywords FROM skill_fts`,
		`INSERT INTO skill_fts(rowid,skill_id,name,aliases,description,triggers,examples,keywords) VALUES(?,?,?,?,?,?,?,?)`, check); err != nil {
		return fmt.Errorf("copy the full-text rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	search, err := resolverpkg.NewSQLiteCatalog(db, index.view.Pointer.CatalogSnapshot)
	if err != nil {
		return err
	}
	index.db, index.search, index.examples = db, search, examples
	return nil
}

// indexedExamples lists each skill's routing examples in the order the catalog
// build joined them into the full-text index.
func indexedExamples(ctx context.Context, source *sql.DB) (map[string][]string, error) {
	rows, err := source.QueryContext(ctx, `SELECT skill_id,value FROM routing_metadata WHERE category='example' ORDER BY skill_id,ordinal`)
	if err != nil {
		return nil, fmt.Errorf("read routing examples: %w", err)
	}
	defer rows.Close()
	examples := map[string][]string{}
	for rows.Next() {
		var id, value string
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		examples[id] = append(examples[id], value)
	}
	return examples, rows.Err()
}

func copyRows(ctx context.Context, source *sql.DB, target *sql.Tx, query, insert string, check func([]any) error) error {
	rows, err := source.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	statement, err := target.PrepareContext(ctx, insert)
	if err != nil {
		return err
	}
	defer statement.Close()
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return err
		}
		if check != nil {
			if err := check(values); err != nil {
				return err
			}
		}
		if _, err := statement.ExecContext(ctx, values...); err != nil {
			return err
		}
	}
	return rows.Err()
}

// columnText reads a scanned text column; NULL reads as empty.
func columnText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

// withoutExample returns examples without the first occurrence of phrase.
func withoutExample(examples []string, phrase string) ([]string, bool) {
	at := slices.Index(examples, phrase)
	if at < 0 {
		return examples, false
	}
	return slices.Delete(slices.Clone(examples), at, at+1), true
}

// omittingCatalog is the served catalog with a leave-one-out index.
type omittingCatalog struct {
	resolverpkg.Catalog
	index *exampleIndex
}

func (c omittingCatalog) searchOmitting(ctx context.Context, query string, limit int, skillID, phrase string) ([]resolverpkg.SearchHit, error) {
	return c.index.searchOmitting(ctx, query, limit, skillID, phrase)
}
