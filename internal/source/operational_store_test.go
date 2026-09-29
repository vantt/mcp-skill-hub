package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOperationalStoreResetsCorruptDatabaseAndTreatsSourcesAsDue(t *testing.T) {
	root := t.TempDir()
	runtime := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, "operational.db"), []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, found, err := (OperationalStore{Root: root}).Get(context.Background(), "source-a")
	if err != nil || found || state.SourceID != "" {
		t.Fatalf("Get after corruption = %#v, %t, %v", state, found, err)
	}
	if _, err := os.Stat(filepath.Join(runtime, "operational.db.corrupt")); err != nil {
		t.Fatalf("corrupt database was not retained safely: %v", err)
	}
	states, err := (OperationalStore{Root: root}).List(context.Background())
	if err != nil || len(states) != 0 {
		t.Fatalf("List after reset = %#v, %v", states, err)
	}
}
