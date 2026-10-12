package catalog

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
)

// maxVerifiedGenerations bounds the process-wide cache; a server normally
// serves one workspace and one current generation.
const maxVerifiedGenerations = 16

// integrityVerified remembers which generation files passed PRAGMA quick_check.
// The check reads the whole database, so repeating it on every request costs
// time that grows with the catalog. A published generation is never edited, it
// is replaced through rename, so a file whose identity (size, modification and
// change time, volume and file index) is unchanged is the file that passed. Any
// write, replacement or metadata restore changes the identity and the file is
// checked again, so corruption that arrives with a file change is still found.
// Damage that leaves the identity untouched is not caught by an up-front check,
// only when a query reads the damaged page. The cache lives in process memory.
// The builder that just ran integrity_check on a generation seeds its result,
// so the first requests after a rebuild do not repeat the check.
var integrityVerified integrityCache

type integrityCache struct {
	mu     sync.Mutex
	files  map[string]canonical.FileIdentity
	checks atomic.Int64 // quick checks run, so tests can tell a reused result from a repeated check
}

// known reports whether the file at path still has the identity that passed.
func (cache *integrityCache) known(path string, identity canonical.FileIdentity) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	verified, ok := cache.files[path]
	return ok && verified == identity
}

func (cache *integrityCache) remember(path string, identity canonical.FileIdentity) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.files == nil {
		cache.files = make(map[string]canonical.FileIdentity)
	}
	if _, exists := cache.files[path]; !exists && len(cache.files) >= maxVerifiedGenerations {
		clear(cache.files)
	}
	cache.files[path] = identity
}

// statGeneration returns the identity of a generation database file.
func statGeneration(root, database string) (canonical.FileIdentity, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return canonical.FileIdentity{}, err
	}
	defer rootHandle.Close()
	return canonical.StatFile(rootHandle, "runtime/catalog/"+database)
}
