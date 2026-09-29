package source

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestHTTPDocumentAdapterRevisionReadAndLimits(t *testing.T) {
	body := "version one"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Etag": []string{"v1"}}, Body: io.NopCloser(stringsReader(body)), ContentLength: int64(len(body)), Request: request}, nil
	})}
	adapter := HTTPDocumentAdapter{Client: client, MaxBytes: 32, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	source := Source{ID: "docs", Locator: Locator{URL: "https://docs.example.com/guide"}}
	revision, err := adapter.CurrentRevision(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if revision.Kind != "declared-version" || revision.Value != "v1" || revision.ContentDigest != Digest([]byte(body)) {
		t.Fatalf("revision = %#v", revision)
	}
	contents, err := adapter.Read(context.Background(), source, revision, "document")
	if err != nil || string(contents) != body {
		t.Fatalf("read = %q, %v", contents, err)
	}
	body = "changed"
	if _, err := adapter.Read(context.Background(), source, revision, "document"); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("read mismatch error = %v", err)
	}
	body = "this body is much larger than the configured bound"
	if _, err := adapter.CurrentRevision(context.Background(), source); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("limit error = %v", err)
	}
}

func TestRemotePolicyRejectsCredentialsProtocolsAndLocalAddresses(t *testing.T) {
	cases := []string{
		"http://example.com/doc", "https://user:secret@example.com/doc", "https://127.0.0.1/doc", "https://[::1]/doc",
		"file:///tmp/doc", "https://example.com/doc?access_token=secret", "https://2130706433/doc", "https://0177.0.0.1/doc",
		"https://0x7f000001/doc", "https://127.1/doc", "https://192.0.2.1/doc", "https://198.18.0.1/doc", "https://[::ffff:127.0.0.1]/doc",
	}
	for _, value := range cases {
		if _, err := ValidateRemoteURL(value, false); err == nil {
			t.Errorf("ValidateRemoteURL(%q) succeeded", value)
		}
	}
}

func TestFilesystemAdapterContainsPathsRejectsSymlinksAndDetectsChange(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	if err := os.Mkdir(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := FilesystemAdapter{Root: root, Now: func() time.Time { return time.Unix(2, 0).UTC() }}
	source := Source{ID: "local", Locator: Locator{Path: "source"}}
	first, err := adapter.CurrentRevision(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Read(context.Background(), source, first, "../escape"); !errors.Is(err, ErrInvalidLocator) {
		t.Fatalf("traversal error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "a.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := adapter.CurrentRevision(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentDigest == second.ContentDigest {
		t.Fatal("revision did not change")
	}
	oldContents, err := adapter.Read(context.Background(), source, first, "a.txt")
	if err != nil || string(oldContents) != "one" {
		t.Fatalf("pinned read = %q, %v", oldContents, err)
	}
	changes, err := adapter.Diff(context.Background(), source, first, second)
	if err != nil || len(changes.Changes) != 1 || changes.Changes[0].Path != "a.txt" || changes.Changes[0].Status != "modified" {
		t.Fatalf("snapshot diff = %#v, %v", changes, err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "b.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(sourceRoot, "a.txt")); err != nil {
		t.Fatal(err)
	}
	third, err := adapter.CurrentRevision(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	changes, err = adapter.Diff(context.Background(), source, second, third)
	if err != nil || len(changes.Changes) != 2 || changes.Changes[0].Status != "deleted" || changes.Changes[1].Status != "added" {
		t.Fatalf("add/delete diff = %#v, %v", changes, err)
	}
	if contents, err := adapter.Read(context.Background(), source, second, "a.txt"); err != nil || string(contents) != "two" {
		t.Fatalf("deleted historical read = %q, %v", contents, err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(sourceRoot, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CurrentRevision(context.Background(), source); err == nil {
		t.Fatal("symlink was accepted")
	}
}

func TestFilesystemSnapshotNeverCapturesConcurrentSymlinkSwap(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	if err := os.Mkdir(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sourceRoot, "value.txt")
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := FilesystemAdapter{Root: root}
	source := Source{ID: "racing", Locator: Locator{Path: "source"}}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(path)
			_ = os.Symlink(outside, path)
			_ = os.Remove(path)
			_ = os.WriteFile(path, []byte("inside"), 0o600)
		}
	}()
	for index := 0; index < 100; index++ {
		revision, err := adapter.CurrentRevision(context.Background(), source)
		if err != nil {
			continue
		}
		contents, err := adapter.Read(context.Background(), source, revision, "value.txt")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read pinned racing revision: %v", err)
		}
		if string(contents) == "outside" {
			t.Fatal("snapshot followed a swapped symlink")
		}
	}
	close(stop)
	<-done
}

func TestGitRepositoryAdapterRejectsNonHTTPSBeforeGitExecution(t *testing.T) {
	adapter := GitRepositoryAdapter{CacheRoot: t.TempDir()}
	for _, locator := range []Locator{{Repository: "file:///tmp/repo"}, {Repository: "ssh://git@example.com/repo"}, {Repository: "https://user:token@example.com/repo"}} {
		if _, err := adapter.Identify(context.Background(), locator); err == nil {
			t.Fatalf("Identify(%q) succeeded", locator.Repository)
		}
	}
}

type sequenceResolver struct {
	mu      sync.Mutex
	answers [][]netip.Addr
}

func (resolver *sequenceResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	answer := resolver.answers[0]
	if len(resolver.answers) > 1 {
		resolver.answers = resolver.answers[1:]
	}
	return answer, nil
}

func TestDNSAnswersArePublicAndRevalidatedForEveryConnection(t *testing.T) {
	private := &sequenceResolver{answers: [][]netip.Addr{{netip.MustParseAddr("10.0.0.1")}}}
	if _, err := resolvePublicAddresses(context.Background(), private, "example.test"); !errors.Is(err, ErrUnsafeAddress) {
		t.Fatalf("private DNS answer error = %v", err)
	}
	rebinding := &sequenceResolver{answers: [][]netip.Addr{{netip.MustParseAddr("8.8.8.8")}, {netip.MustParseAddr("127.0.0.1")}}}
	if addresses, err := resolvePublicAddresses(context.Background(), rebinding, "example.test"); err != nil || addresses[0].String() != "8.8.8.8" {
		t.Fatalf("first DNS answer = %v, %v", addresses, err)
	}
	if _, err := resolvePublicAddresses(context.Background(), rebinding, "example.test"); !errors.Is(err, ErrUnsafeAddress) {
		t.Fatalf("rebound DNS answer error = %v", err)
	}
	client := newPinnedHTTPClient(time.Second, rebinding, 1024, true)
	request, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1/private", nil)
	if err := client.CheckRedirect(request, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("Git redirect policy = %v", err)
	}
}

type stringReader string

func (reader *stringReader) Read(target []byte) (int, error) {
	if len(*reader) == 0 {
		return 0, io.EOF
	}
	n := copy(target, *reader)
	*reader = (*reader)[n:]
	return n, nil
}
func stringsReader(value string) io.Reader { reader := stringReader(value); return &reader }
