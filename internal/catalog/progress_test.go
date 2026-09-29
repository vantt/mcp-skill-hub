package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestBuildReportsProgressAndCancelsBeforePublish(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var events []ProgressEvent
	_, err := BuildCatalogGeneration(ctx, root, BuildOptions{
		Progress: func(event ProgressEvent) { events = append(events, event) },
		BeforePublish: func() error {
			cancel()
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("build error = %v", err)
	}
	if len(events) < 3 || events[0].Stage != "capture" || events[len(events)-1].Stage != "verify" {
		t.Fatalf("progress events = %#v", events)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "catalog", "current.json")); !os.IsNotExist(err) {
		t.Fatalf("cancelled rebuild published pointer: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "runtime", "catalog", "generations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancelled rebuild left generation files: %#v", entries)
	}
}

func TestBuildPopulationReportsPeriodicProgressAndObservesCancellation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 70; index++ {
		writeCanonical(t, root, filepath.ToSlash(filepath.Join("config", "fixtures", fmt.Sprintf("resource-%03d.yaml", index))), "fixture: true\n")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var populationEvents int
	_, err := BuildCatalogGeneration(ctx, root, BuildOptions{Progress: func(event ProgressEvent) {
		if event.Stage == "build" && strings.HasPrefix(event.Message, "Populating catalog records") {
			populationEvents++
			if populationEvents == 2 {
				cancel()
			}
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("population cancellation error = %v", err)
	}
	if populationEvents < 2 {
		t.Fatalf("population progress events = %d", populationEvents)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "catalog", "current.json")); !os.IsNotExist(err) {
		t.Fatalf("population cancellation published pointer: %v", err)
	}
}

func TestBuildProgressCompletesAfterPublish(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	var events []ProgressEvent
	if _, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{Progress: func(event ProgressEvent) {
		events = append(events, event)
	}}); err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Stage != "complete" || last.Completed != last.Total || last.Total != 5 {
		t.Fatalf("last progress event = %#v", last)
	}
}
