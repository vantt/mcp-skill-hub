package mutation

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type receiptDocument struct {
	SchemaVersion         int              `yaml:"schema_version"`
	ID                    string           `yaml:"id"`
	Kind                  string           `yaml:"kind"`
	OccurredAt            string           `yaml:"occurred_at"`
	IdempotencyKey        string           `yaml:"idempotency_key"`
	RequestDigest         string           `yaml:"request_digest"`
	Proposal              *receiptProposal `yaml:"proposal,omitempty"`
	BaseCatalogSnapshot   string           `yaml:"base_catalog_snapshot,omitempty"`
	SourceSchemaVersion   *int             `yaml:"source_schema_version,omitempty"`
	TargetSchemaVersion   *int             `yaml:"target_schema_version,omitempty"`
	ResultCatalogSnapshot string           `yaml:"result_catalog_snapshot"`
	Changes               []receiptChange  `yaml:"changes"`
	Status                string           `yaml:"status"`
}

type receiptProposal struct {
	ID     string `yaml:"id"`
	Digest string `yaml:"digest"`
}

type receiptChange struct {
	Path             string `yaml:"path"`
	Before           string `yaml:"before"`
	After            string `yaml:"after"`
	BeforeContent    string `yaml:"before_content,omitempty"`
	AfterContent     string `yaml:"after_content,omitempty"`
	ContentAvailable bool   `yaml:"content_available,omitempty"`
}

func operationPath(id string, occurredAt time.Time) string {
	return "history/operations/" + occurredAt.UTC().Format("2006/01/") + id + ".yaml"
}

func receiptYAML(root string, set WriteSet, changes []Change, resultSnapshot string, occurredAt time.Time) ([]byte, error) {
	document := receiptDocument{
		SchemaVersion:         1,
		ID:                    set.OperationID,
		Kind:                  set.Command,
		OccurredAt:            occurredAt.UTC().Format(time.RFC3339Nano),
		IdempotencyKey:        effectiveIdempotencyKey(set),
		RequestDigest:         requestDigest(set),
		BaseCatalogSnapshot:   set.BaseCatalogSnapshot,
		SourceSchemaVersion:   set.SourceSchemaVersion,
		TargetSchemaVersion:   set.TargetSchemaVersion,
		ResultCatalogSnapshot: resultSnapshot,
		Changes:               make([]receiptChange, 0, len(changes)),
		Status:                "applied",
	}
	if set.ProposalID != "" {
		document.Proposal = &receiptProposal{ID: set.ProposalID, Digest: set.ProposalDigest}
	}
	const perChangeContentLimit = 64 << 10
	const totalContentLimit = 512 << 10
	contentBytes := 0
	for _, item := range changes {
		after := ""
		if !item.Delete {
			after = digest(item.Contents)
		}
		change := receiptChange{Path: item.Path, Before: item.BeforeDigest, After: after}
		var before []byte
		if item.BeforeDigest != "" {
			var err error
			before, err = readCanonicalFile(root, item.Path)
			if err != nil {
				return nil, fmt.Errorf("read operation before-image %s: %w", item.Path, err)
			}
		}
		afterContent := item.Contents
		if item.Delete {
			afterContent = nil
		}
		combined := len(before) + len(afterContent)
		if len(before) <= perChangeContentLimit && len(afterContent) <= perChangeContentLimit && contentBytes+combined <= totalContentLimit && utf8.Valid(before) && utf8.Valid(afterContent) && !strings.ContainsRune(string(before), '\x00') && !strings.ContainsRune(string(afterContent), '\x00') {
			change.BeforeContent = string(before)
			change.AfterContent = string(afterContent)
			change.ContentAvailable = true
			contentBytes += combined
		}
		document.Changes = append(document.Changes, change)
	}
	contents, err := yaml.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode operation receipt: %w", err)
	}
	return contents, nil
}

func existingOperation(root string, set WriteSet) (Receipt, bool, error) {
	operationRoot := filepath.Join(root, "history", "operations")
	key := effectiveIdempotencyKey(set)
	wantedDigest := requestDigest(set)
	var matched receiptDocument
	var receiptPath string
	found := false
	err := filepath.WalkDir(operationRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("unsafe operation receipt path: %s", path)
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var document receiptDocument
		if err := yaml.Unmarshal(contents, &document); err != nil {
			return fmt.Errorf("parse operation receipt %s: %w", path, err)
		}
		if document.ID == set.OperationID && document.IdempotencyKey != key {
			return fmt.Errorf("%w: operation ID %s already exists", ErrConflict, set.OperationID)
		}
		if document.IdempotencyKey != key {
			return nil
		}
		if document.SchemaVersion != 1 || document.Status != "applied" || document.ID == "" || document.RequestDigest == "" || document.ResultCatalogSnapshot == "" {
			return fmt.Errorf("invalid operation receipt for idempotency key %q", key)
		}
		if (document.SourceSchemaVersion == nil) != (document.TargetSchemaVersion == nil) ||
			(document.SourceSchemaVersion != nil && (*document.SourceSchemaVersion < 0 || *document.TargetSchemaVersion <= *document.SourceSchemaVersion)) {
			return fmt.Errorf("invalid schema migration versions in operation receipt for idempotency key %q", key)
		}
		if document.RequestDigest != wantedDigest {
			return ErrIdempotencyConflict
		}
		if found {
			return fmt.Errorf("duplicate operation receipt for idempotency key %q", key)
		}
		found, matched, receiptPath = true, document, path
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, err
	}
	if !found {
		return Receipt{}, false, nil
	}
	dirty, err := gitDirty(root)
	if err != nil {
		return Receipt{}, false, err
	}
	paths := make([]string, 0, len(matched.Changes)+1)
	for _, item := range matched.Changes {
		paths = append(paths, item.Path)
	}
	relativeReceipt, err := filepath.Rel(root, receiptPath)
	if err != nil {
		return Receipt{}, false, err
	}
	paths = append(paths, filepath.ToSlash(relativeReceipt))
	return Receipt{
		OperationID: matched.ID, ChangedPaths: paths, CatalogSnapshot: matched.ResultCatalogSnapshot, GitDirty: dirty,
		SourceSchemaVersion: matched.SourceSchemaVersion, TargetSchemaVersion: matched.TargetSchemaVersion,
	}, true, nil
}
