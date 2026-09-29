package distill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/source"
)

type hostileAdapter struct{ reads int }

func (*hostileAdapter) Identify(context.Context, source.Locator) (source.Identity, error) {
	return source.Identity{}, nil
}
func (*hostileAdapter) CurrentRevision(context.Context, source.Source) (source.Revision, error) {
	return source.Revision{}, nil
}
func (*hostileAdapter) Diff(context.Context, source.Source, source.Revision, source.Revision) (source.ChangeSet, error) {
	return source.ChangeSet{}, nil
}
func (adapter *hostileAdapter) Read(context.Context, source.Source, source.Revision, string) ([]byte, error) {
	adapter.reads++
	return []byte("hostile"), nil
}
func (*hostileAdapter) List(context.Context, source.Source, source.Revision, source.Scope) ([]source.Resource, error) {
	return nil, nil
}

func TestRevisionPackageRejectsEveryUnsafeAdapterPathBeforeReadOrWrite(t *testing.T) {
	revision := source.Revision{Kind: "declared-version", Value: "v1", ContentDigest: source.Digest([]byte("revision")), ObservedAt: time.Now().UTC()}
	for _, path := range []string{"../escape", "a/../../escape", "/absolute", `windows\\escape`, "a//b", "a/./b"} {
		t.Run(path, func(t *testing.T) {
			root := t.TempDir()
			adapter := &hostileAdapter{}
			_, err := CreateRevisionPackage(context.Background(), root, adapter, source.Source{ID: "source-a"}, "RUN-safe", nil, revision, []ChangedResource{{Path: path, Status: "added"}}, time.Now().UTC())
			if err == nil {
				t.Fatalf("unsafe path %q was accepted", path)
			}
			if adapter.reads != 0 {
				t.Fatalf("adapter was read before path validation")
			}
			if _, statErr := os.Stat(filepath.Join(root, "runtime")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("package path was written before validation: %v", statErr)
			}
		})
	}
}

func TestValidateLocatorResolvesOnlySupportedPinnedLocations(t *testing.T) {
	contents := []byte("# Retry Review\nfirst\nsecond\n")
	for _, locator := range []string{"guide.md", "guide.md#retry-review", "guide.md#L1", "guide.md#L2-L3"} {
		if err := ValidateLocator("guide.md", locator, contents); err != nil {
			t.Fatalf("%s: %v", locator, err)
		}
	}
	for _, locator := range []string{"guide.md#arbitrary", "guide.md#L9", "guide.md#L3-L2", "other.md#L1"} {
		if err := ValidateLocator("guide.md", locator, contents); err == nil {
			t.Fatalf("unsupported locator %s was accepted", locator)
		}
	}
}
