package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

type UpstreamSourceRef struct {
	SourceID string `json:"source_id"`
	Created  bool   `json:"created"`
}

func normalizeRepositoryURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	uPath := u.Path
	if host == "github.com" {
		uPath = strings.ToLower(uPath)
	}
	uPath = strings.TrimSuffix(uPath, "/")
	uPath = strings.TrimSuffix(uPath, ".git")
	if u.Scheme == "" && u.Host == "" {
		return uPath
	}
	return scheme + "://" + host + uPath
}

func sameRepository(a, b string) bool {
	return normalizeRepositoryURL(a) == normalizeRepositoryURL(b)
}

func deriveUpstreamBaseID(repoURL string) string {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	var base string
	if err == nil && u.Path != "" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			owner := parts[len(parts)-2]
			repo := strings.TrimSuffix(parts[len(parts)-1], ".git")
			base = sanitizeSourceID(owner + "-" + repo)
		} else if len(parts) == 1 {
			repo := strings.TrimSuffix(parts[0], ".git")
			base = sanitizeSourceID(repo)
		}
	}
	if base == "" {
		base = deriveSourceID(repoURL, "")
	}
	if len(base) > 64 {
		base = base[:64]
	}
	if base == "" {
		base = "source"
	}
	return base
}

func upstreamSourceID(repoURL, ref string, taken map[string]bool) string {
	base := deriveUpstreamBaseID(repoURL)
	if !taken[base] {
		return base
	}

	sanitizedRef := sanitizeSourceID(ref)
	if sanitizedRef == "" {
		sanitizedRef = "ref"
	}
	cand := base + "-" + sanitizedRef
	if len(cand) > 64 {
		cand = cand[:64]
	}
	if !taken[cand] {
		return cand
	}

	for i := 2; ; i++ {
		suffix := fmt.Sprintf("-%d", i)
		pre := cand
		if len(pre)+len(suffix) > 64 {
			pre = pre[:64-len(suffix)]
		}
		candidate := pre + suffix
		if !taken[candidate] {
			return candidate
		}
	}
}

func ensureUpstreamSource(ctx context.Context, root string, adapter sourcepkg.Adapter, repository, ref, commit string) (*sourcepkg.Record, *mutation.Change, bool, error) {
	_, records, err := readSourceRecords(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, false, err
	}

	for _, rec := range records {
		if rec.Adapter == "git" && sameRepository(rec.Locator.Repository, repository) && rec.Locator.Ref == ref {
			return &rec, nil, false, nil
		}
	}

	skillIDs, _ := listWorkspaceSkillIDs(root)
	candidates, _, _ := readSourceRecords(root)

	taken := make(map[string]bool, len(records)+len(candidates)+len(skillIDs))
	for _, rec := range records {
		taken[rec.ID] = true
	}
	for _, cand := range candidates {
		taken[cand.ID] = true
	}
	for id := range skillIDs {
		taken[id] = true
	}

	sourceID := upstreamSourceID(repository, ref, taken)

	locator := sourcepkg.Locator{
		Repository: repository,
		Ref:        ref,
		Path:       "",
	}

	identity := sourcepkg.Identity{
		Name:      filepath.Base(repository),
		Canonical: repository,
	}
	if adapter != nil {
		if id, idErr := adapter.Identify(ctx, locator); idErr == nil {
			identity = id
		}
	}

	license := identity.License

	var currentRev *sourcepkg.Revision
	if adapter != nil {
		if revAdapter, ok := adapter.(revisionAtAdapter); ok && commit != "" {
			if rev, err := revAdapter.RevisionAt(ctx, sourcepkg.Source{ID: sourceID, Locator: locator}, commit); err == nil {
				currentRev = &rev
			}
		}
		if currentRev == nil {
			if rev, err := adapter.CurrentRevision(ctx, sourcepkg.Source{ID: sourceID, Locator: locator}); err == nil {
				currentRev = &rev
			}
		}
	}
	if currentRev == nil && commit != "" && len(commit) == 40 {
		currentRev = &sourcepkg.Revision{
			Kind:          "git-commit",
			Value:         commit,
			ContentDigest: sourcepkg.Digest([]byte(commit)),
			ObservedAt:    time.Now().UTC(),
		}
	}

	record := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            sourceID,
		Purpose:       "upstream",
		Adapter:       "git",
		Locator:       locator,
		Status:        "watching",
		Identity:      identity,
		License:       license,
		Trust:         sourcepkg.Trust{Source: "community"},
		Monitoring:    sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Limits: sourcepkg.Limits{
			TimeoutSeconds: int(sourcepkg.DefaultTimeout / time.Second),
			MaxBytes:       sourcepkg.DefaultMaxBytes,
			MaxFiles:       sourcepkg.DefaultMaxFiles,
			MaxFileBytes:   sourcepkg.DefaultMaxFileSize,
		},
		CurrentRevision: currentRev,
	}

	data, err := sourcepkg.MarshalCanonical(record)
	if err != nil {
		return nil, nil, false, err
	}
	if _, err := sourcepkg.ParseRecord(data); err != nil {
		return nil, nil, false, err
	}

	change := mutation.Change{
		Path:     "sources/catalog/" + sourceID + ".yaml",
		Contents: data,
	}

	return &record, &change, true, nil
}
