package app

import (
	"os"
	"testing"
	"time"
)

func TestReuseOfARecentSnapshotWritesNothing(t *testing.T) {
	root := newBundleWorkspace(t)
	settleHub(t, root)
	snapshots := NewSnapshotService()
	local, err := snapshots.Ensure(t.Context(), root, "bundle-skill")
	if err != nil || local.Status != LocalStatusReady {
		t.Fatalf("ensure = %+v, %v", local, err)
	}
	recent := time.Now().Add(-time.Minute).Truncate(time.Second)
	for _, directory := range []string{local.Path, local.StateDirectory} {
		if err := os.Chtimes(directory, recent, recent); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := snapshots.Ensure(t.Context(), root, "bundle-skill"); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{local.Path, local.StateDirectory} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(recent) {
			t.Fatalf("%s was touched again within the interval: %v", directory, info.ModTime())
		}
	}
}
