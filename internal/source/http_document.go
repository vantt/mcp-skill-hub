package source

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"
)

// HTTPDocumentAdapter reads one immutable or living HTTPS document.
type HTTPDocumentAdapter struct {
	Client    *http.Client
	MaxBytes  int64
	AllowHTTP bool
	Immutable bool
	Now       func() time.Time
}

func (adapter HTTPDocumentAdapter) defaults() HTTPDocumentAdapter {
	if adapter.Client == nil {
		adapter.Client = NewSafeHTTPClient(DefaultTimeout, adapter.AllowHTTP, nil)
	}
	if adapter.MaxBytes <= 0 {
		adapter.MaxBytes = DefaultMaxBytes
	}
	if adapter.Now == nil {
		adapter.Now = time.Now
	}
	return adapter
}

func (adapter HTTPDocumentAdapter) Identify(ctx context.Context, locator Locator) (Identity, error) {
	u, err := ValidateRemoteURL(locator.URL, adapter.AllowHTTP)
	if err != nil {
		return Identity{}, err
	}
	name := path.Base(strings.TrimSuffix(u.Path, "/"))
	if name == "." || name == "/" || name == "" {
		name = u.Hostname()
	}
	return Identity{Name: name, Canonical: u.Scheme + "://" + u.Host + u.EscapedPath()}, nil
}

func (adapter HTTPDocumentAdapter) CurrentRevision(ctx context.Context, source Source) (Revision, error) {
	adapter = adapter.defaults()
	if source.Limits.MaxBytes > 0 && source.Limits.MaxBytes < adapter.MaxBytes {
		adapter.MaxBytes = source.Limits.MaxBytes
	}
	contents, headers, err := adapter.fetch(ctx, source.Locator.URL)
	if err != nil {
		return Revision{}, err
	}
	digest := Digest(contents)
	kind, value := "content-digest", digest
	if !adapter.Immutable {
		if version := strings.TrimSpace(headers.Get("ETag")); version != "" && len(version) <= 256 {
			kind, value = "declared-version", version
		}
	}
	return Revision{Kind: kind, Value: value, ContentDigest: digest, ObservedAt: adapter.Now().UTC()}, nil
}

func (adapter HTTPDocumentAdapter) Diff(_ context.Context, _ Source, from, to Revision) (ChangeSet, error) {
	changes := []Change{}
	if from.Value != to.Value || from.ContentDigest != to.ContentDigest {
		changes = append(changes, Change{Path: "document", Status: "modified"})
	}
	return ChangeSet{From: from, To: to, Changes: changes}, nil
}

func (adapter HTTPDocumentAdapter) Read(ctx context.Context, source Source, revision Revision, resourcePath string) ([]byte, error) {
	if resourcePath != "document" {
		return nil, fmt.Errorf("%w: HTTP document resource is named document", ErrInvalidLocator)
	}
	adapter = adapter.defaults()
	if source.Limits.MaxBytes > 0 && source.Limits.MaxBytes < adapter.MaxBytes {
		adapter.MaxBytes = source.Limits.MaxBytes
	}
	contents, _, err := adapter.fetch(ctx, source.Locator.URL)
	if err != nil {
		return nil, err
	}
	if Digest(contents) != revision.ContentDigest {
		return nil, ErrRevisionMismatch
	}
	return contents, nil
}

func (adapter HTTPDocumentAdapter) List(ctx context.Context, source Source, revision Revision, scope Scope) ([]Resource, error) {
	if scope.Prefix != "" && scope.Prefix != "document" {
		return []Resource{}, nil
	}
	contents, err := adapter.Read(ctx, source, revision, "document")
	if err != nil {
		return nil, err
	}
	return []Resource{{Path: "document", Size: int64(len(contents))}}, nil
}

func (adapter HTTPDocumentAdapter) fetch(ctx context.Context, raw string) ([]byte, http.Header, error) {
	if _, err := ValidateRemoteURL(raw, adapter.AllowHTTP); err != nil {
		return nil, nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Accept", "application/octet-stream,text/plain,text/markdown,application/pdf;q=0.9,*/*;q=0.1")
	response, err := adapter.Client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch source document: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("fetch source document: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > adapter.MaxBytes {
		return nil, nil, &LimitExceededError{Limit: "bytes", Actual: response.ContentLength, Max: adapter.MaxBytes}
	}
	contents, err := boundedRead(response.Body, adapter.MaxBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("read source document: %w", err)
	}
	return contents, response.Header.Clone(), nil
}
