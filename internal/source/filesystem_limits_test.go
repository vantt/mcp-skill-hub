package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilesystemAdapterAlwaysCapsOperationTimeout(t *testing.T) {
	adapter := (FilesystemAdapter{Timeout: filesystemMaxTimeout * 10}).defaults()
	if adapter.Timeout != filesystemMaxTimeout {
		t.Fatalf("default timeout = %s, want hard cap %s", adapter.Timeout, filesystemMaxTimeout)
	}
	adapter = adapter.withLimits(Limits{TimeoutSeconds: int((filesystemMaxTimeout * 10) / time.Second)})
	if adapter.Timeout != filesystemMaxTimeout {
		t.Fatalf("configured timeout = %s, want hard cap %s", adapter.Timeout, filesystemMaxTimeout)
	}
	short := 5 * time.Millisecond
	adapter = (FilesystemAdapter{Timeout: short}).withLimits(Limits{TimeoutSeconds: int((filesystemMaxTimeout * 10) / time.Second)})
	if adapter.Timeout != short {
		t.Fatalf("shorter adapter timeout = %s, want %s", adapter.Timeout, short)
	}
}

func TestFilesystemCaptureTimesOutWhileWaitingForCache(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	<-filesystemCacheGate
	t.Cleanup(func() { filesystemCacheGate <- struct{}{} })

	adapter := FilesystemAdapter{Root: root, CacheRoot: filepath.Join(t.TempDir(), "cache"), Timeout: 10 * time.Millisecond}
	_, err := adapter.CurrentRevision(context.Background(), Source{Locator: Locator{Path: "source"}, Limits: Limits{TimeoutSeconds: 3600}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cache wait error = %v, want deadline exceeded", err)
	}
}

func TestFilesystemWalkObservesCanceledContext(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	if err := os.Mkdir(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 32; index++ {
		name := filepath.Join(sourceRoot, time.Unix(int64(index), 0).Format("150405")+".txt")
		if err := os.WriteFile(name, []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := (FilesystemAdapter{Root: root, CacheRoot: filepath.Join(t.TempDir(), "cache")}).defaults()
	if _, err := adapter.capture(ctx, "source"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled walk error = %v", err)
	}
}
