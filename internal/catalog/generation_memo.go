package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"

	"github.com/vantt/skillhub-mcp/internal/canonical"
)

const (
	// maxMemoRoots bounds the process-wide store; a server normally serves one workspace.
	maxMemoRoots = 8
	// maxMemoValues bounds one generation's memo: a few shared values plus a
	// handful per skill. Past it the memo is cleared and refilled, so memory
	// follows the size of the catalog, never the number of requests.
	maxMemoValues = 1024
)

// memoIdentity names one exact generation file: the generation id and the
// identity of its database file. A generation is immutable, so everything
// derived from it stays valid while this identity is unchanged.
type memoIdentity struct {
	generation string
	file       canonical.FileIdentity
}

// GenerationMemo holds values derived from one immutable generation. It is
// replaced as a whole when the published generation changes, so no value ever
// outlives the generation it was computed from. Values must be treated as
// immutable by every reader.
type GenerationMemo struct {
	identity memoIdentity
	mu       sync.Mutex
	values   map[string]any
}

// generationMemos keeps the newest memo of each workspace root.
var generationMemos = struct {
	mu     sync.Mutex
	byRoot map[string]*GenerationMemo
}{byRoot: map[string]*GenerationMemo{}}

// memoFor returns the memo of the observed generation, replacing the one a
// previous generation of the same root left behind. A generation file that
// cannot be identified gets a private memo that is never shared.
func memoFor(root string, pointer Pointer) *GenerationMemo {
	file, err := statGeneration(root, pointer.Database)
	if err != nil {
		return &GenerationMemo{values: map[string]any{}}
	}
	identity := memoIdentity{generation: pointer.Generation, file: file}
	key := filepath.Clean(root)
	generationMemos.mu.Lock()
	defer generationMemos.mu.Unlock()
	if memo, ok := generationMemos.byRoot[key]; ok && memo.identity == identity {
		return memo
	}
	if _, known := generationMemos.byRoot[key]; !known && len(generationMemos.byRoot) >= maxMemoRoots {
		clear(generationMemos.byRoot)
	}
	memo := &GenerationMemo{identity: identity, values: map[string]any{}}
	generationMemos.byRoot[key] = memo
	return memo
}

// MemoGet returns the value stored under key for the observed generation.
func MemoGet[T any](view *View, key string) (T, bool) {
	view.memo.mu.Lock()
	stored, ok := view.memo.values[key]
	view.memo.mu.Unlock()
	if !ok {
		var zero T
		return zero, false
	}
	value, ok := stored.(T)
	return value, ok
}

// MemoPut stores a value derived from the observed generation. Two requests
// that miss at the same time both compute the value and the last one wins; the
// values are equal because the generation is immutable.
func MemoPut[T any](view *View, key string, value T) {
	view.memo.mu.Lock()
	defer view.memo.mu.Unlock()
	if _, exists := view.memo.values[key]; !exists && len(view.memo.values) >= maxMemoValues {
		clear(view.memo.values)
	}
	view.memo.values[key] = value
}

// Memoized returns the value stored under key, computing and storing it with
// the generation's database on a miss. A failed computation is not stored.
func Memoized[T any](ctx context.Context, view *View, key string, compute func(*sql.DB) (T, error)) (T, error) {
	if value, ok := MemoGet[T](view, key); ok {
		return value, nil
	}
	var zero T
	database, err := view.DB(ctx)
	if err != nil {
		return zero, err
	}
	value, err := compute(database)
	if err != nil {
		return zero, err
	}
	MemoPut(view, key, value)
	return value, nil
}
