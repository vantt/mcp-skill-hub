// Package source provides bounded, non-executing adapters for untrusted upstream material.
package source

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultTimeout     = 20 * time.Second
	DefaultMaxBytes    = int64(8 << 20)
	DefaultMaxFiles    = 2048
	DefaultMaxFileSize = int64(2 << 20)
)

var (
	ErrInvalidLocator        = errors.New("invalid source locator")
	ErrUnsafeAddress         = errors.New("source address is local or private")
	ErrLimitExceeded         = errors.New("source resource limit exceeded")
	ErrRevisionMismatch      = errors.New("source revision is no longer current")
	ErrHistoryUnavailable    = errors.New("source revision history is unavailable")
	ErrAmbiguousRef          = errors.New("ambiguous git reference")
	ErrAmbiguousLocator      = errors.New("ambiguous source locator")
	ErrLocalWatchUnsupported = errors.New("local watch unsupported")
	ErrUnsafeFile            = errors.New("unsafe file in source directory")
)

// AmbiguousRefError provides specific information and guidance for ambiguous git references.
type AmbiguousRefError struct {
	Candidates []string
	Ref        string
	Path       string
}

func (e *AmbiguousRefError) Error() string {
	if len(e.Candidates) > 0 {
		return fmt.Sprintf("ambiguous git reference %q matches candidates: %s (specify --ref or --path)", e.Ref, strings.Join(e.Candidates, ", "))
	}
	return fmt.Sprintf("ambiguous git reference %q (specify --ref or --path)", e.Ref)
}

func (e *AmbiguousRefError) Is(target error) bool {
	return target == ErrAmbiguousRef
}

// LimitExceededError provides specific information about which resource limit was breached.
type LimitExceededError struct {
	Limit  string // "files", "bytes", "file_size"
	Actual int64
	Max    int64
	Path   string
}

func (e *LimitExceededError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("source exceeded %s limit on %s (%d > %d)", e.Limit, e.Path, e.Actual, e.Max)
	}
	return fmt.Sprintf("source exceeded %s limit (%d > %d)", e.Limit, e.Actual, e.Max)
}

func (e *LimitExceededError) Is(target error) bool {
	return target == ErrLimitExceeded
}

// Locator is an adapter-neutral, credential-free source location.
type Locator struct {
	URL            string `json:"url,omitempty" yaml:"url,omitempty"`
	Repository     string `json:"repository,omitempty" yaml:"repository,omitempty"`
	Path           string `json:"path,omitempty" yaml:"path,omitempty"`
	Ref            string `json:"ref,omitempty" yaml:"ref,omitempty"`
	Mode           string `json:"mode,omitempty" yaml:"mode,omitempty"`
	SnapshotID     string `json:"snapshot_id,omitempty" yaml:"snapshot_id,omitempty"`
	SnapshotDigest string `json:"snapshot_digest,omitempty" yaml:"snapshot_digest,omitempty"`
}

// Source is the minimum adapter input derived from a canonical source record.
type Source struct {
	ID      string
	Locator Locator
	Limits  Limits
}

// Limits are durable per-source ceilings; adapters also enforce their lower global ceilings.
type Limits struct {
	TimeoutSeconds int   `json:"timeout_seconds" yaml:"timeout_seconds"`
	MaxBytes       int64 `json:"max_bytes" yaml:"max_bytes"`
	MaxFiles       int   `json:"max_files" yaml:"max_files"`
	MaxFileBytes   int64 `json:"max_file_bytes" yaml:"max_file_bytes"`
}

// Identity is metadata detected during the single onboarding inspection.
type Identity struct {
	Name          string `json:"name"`
	Canonical     string `json:"canonical"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Path          string `json:"path,omitempty"`
	License       string `json:"license,omitempty"`
}

// Revision is opaque to application services except for equality.
type Revision struct {
	Kind          string    `json:"kind" yaml:"kind"`
	Value         string    `json:"value" yaml:"value"`
	ContentDigest string    `json:"content_digest" yaml:"content_digest"`
	ObservedAt    time.Time `json:"observed_at" yaml:"observed_at"`
}

// Change describes one bounded source-relative change.
type Change struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// ChangeSet is a deterministic adapter diff.
type ChangeSet struct {
	From    Revision `json:"from"`
	To      Revision `json:"to"`
	Changes []Change `json:"changes"`
}

// Scope limits adapter enumeration to a source-relative prefix.
type Scope struct{ Prefix string }

// Resource identifies readable data without executing it.
type Resource struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Adapter is the common contract for Git, filesystem, and document sources.
type Adapter interface {
	Identify(context.Context, Locator) (Identity, error)
	CurrentRevision(context.Context, Source) (Revision, error)
	Diff(context.Context, Source, Revision, Revision) (ChangeSet, error)
	Read(context.Context, Source, Revision, string) ([]byte, error)
	List(context.Context, Source, Revision, Scope) ([]Resource, error)
}
