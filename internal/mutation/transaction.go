package mutation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
)

type manifest struct {
	TransactionVersion            int      `json:"transaction_version"`
	OperationID                   string   `json:"operation_id"`
	Command                       string   `json:"command"`
	Phase                         string   `json:"phase"`
	CreatedAt                     string   `json:"created_at"`
	BaseCatalogSnapshot           string   `json:"base_catalog_snapshot,omitempty"`
	ExpectedResultCatalogSnapshot string   `json:"expected_result_catalog_snapshot,omitempty"`
	PublishedCatalogSnapshot      string   `json:"published_catalog_snapshot,omitempty"`
	PublishedGeneration           string   `json:"published_generation,omitempty"`
	Files                         []change `json:"files"`
}

type change struct {
	Path         string `json:"path"`
	BeforeDigest string `json:"before"`
	AfterDigest  string `json:"after"`
	Staged       string `json:"staged"`
	Delete       bool   `json:"delete,omitempty"`
	BeforeStaged string `json:"before_staged,omitempty"`
	Displaced    string `json:"displaced,omitempty"`
	Receipt      bool   `json:"receipt,omitempty"`
}

func commitPrepared(root string, set WriteSet, changes []Change, resultSnapshot string, occurredAt time.Time, options Options) (Receipt, error) {
	transactionsRoot := filepath.Join(root, ".skillhub", "transactions")
	receiptPath := operationPath(set.OperationID, occurredAt)
	if actual, err := digestAt(root, receiptPath); err != nil {
		return Receipt{}, err
	} else if actual != "" {
		return Receipt{}, fmt.Errorf("%w: operation receipt path already exists", ErrConflict)
	}
	receipt, err := receiptYAML(root, set, changes, resultSnapshot, occurredAt)
	if err != nil {
		return Receipt{}, err
	}
	entries := make([]change, 0, len(changes)+1)
	beforeImages := make(map[string][]byte, len(changes))
	for _, item := range changes {
		entry := change{Path: item.Path, BeforeDigest: item.BeforeDigest, Delete: item.Delete}
		if item.BeforeDigest != "" {
			before, err := readCanonicalFile(root, item.Path)
			if err != nil {
				return Receipt{}, err
			}
			if digest(before) != item.BeforeDigest {
				return Receipt{}, fmt.Errorf("%w: %s", ErrConflict, item.Path)
			}
			entry.BeforeStaged = "before/" + item.Path
			entry.Displaced = ".skillhub/transactions/" + set.OperationID + "/displaced/" + item.Path
			beforeImages[item.Path] = before
		}
		if !item.Delete {
			entry.AfterDigest = digest(item.Contents)
			entry.Staged = "after/" + item.Path
		}
		entries = append(entries, entry)
	}
	entries = append(entries, change{Path: receiptPath, AfterDigest: digest(receipt), Staged: "after/" + receiptPath, Receipt: true})
	m := manifest{
		TransactionVersion: 1, OperationID: set.OperationID, Command: set.Command,
		Phase: "staging", CreatedAt: occurredAt.Format(time.RFC3339Nano),
		BaseCatalogSnapshot: set.BaseCatalogSnapshot, ExpectedResultCatalogSnapshot: resultSnapshot,
		Files: entries,
	}
	if err := ensureSafeParents(root, ".skillhub/transactions", 0o700); err != nil {
		return Receipt{}, err
	}
	txn := filepath.Join(transactionsRoot, set.OperationID)
	if err := os.Mkdir(txn, 0o700); err != nil {
		return Receipt{}, fmt.Errorf("create transaction directory: %w", err)
	}
	if err := syncDir(transactionsRoot); err != nil {
		return Receipt{}, err
	}
	if err := options.inject(FaultTransactionCreated); err != nil {
		return Receipt{}, err
	}
	// Writing the manifest before images makes every later staging failure
	// classifiable. In phase staging, all canonical paths must still be before.
	if err := writeManifest(txn, m); err != nil {
		return Receipt{}, err
	}
	for index, item := range entries {
		if item.BeforeStaged != "" {
			if err := writeFileSync(txn, item.BeforeStaged, beforeImages[item.Path]); err != nil {
				return Receipt{}, err
			}
			if err := options.inject(FaultStagedFile); err != nil {
				return Receipt{}, err
			}
		}
		if item.Delete {
			continue
		}
		contents := receipt
		if !item.Receipt {
			contents = changes[index].Contents
		}
		if err := writeFileSync(txn, item.Staged, contents); err != nil {
			return Receipt{}, err
		}
		if err := options.inject(FaultStagedFile); err != nil {
			return Receipt{}, err
		}
	}
	m.Phase = "prepared"
	if err := writeManifest(txn, m); err != nil {
		return Receipt{}, err
	}
	if err := options.inject(FaultManifestPhaseUpdate); err != nil {
		return Receipt{}, err
	}

	domainCount := len(entries) - 1
	for index := 0; index < domainCount; index++ {
		if err := applyStagedChecked(root, txn, entries[index], options); err != nil {
			return Receipt{}, err
		}
		switch {
		case index == 0:
			if err := options.inject(FaultFirstCanonicalReplace); err != nil {
				return Receipt{}, err
			}
		case index == domainCount-1:
			if err := options.inject(FaultLastCanonicalReplace); err != nil {
				return Receipt{}, err
			}
		default:
			if err := options.inject(FaultMiddleCanonicalReplace); err != nil {
				return Receipt{}, err
			}
		}
	}
	if err := applyStagedChecked(root, txn, entries[len(entries)-1], options); err != nil {
		return Receipt{}, err
	}
	if err := options.inject(FaultReceiptWrite); err != nil {
		return Receipt{}, err
	}
	if err := validateApplied(root, m); err != nil {
		return Receipt{}, err
	}
	if err := options.inject(FaultCanonicalValidation); err != nil {
		return Receipt{}, err
	}
	m.Phase = "canonical_applied"
	if err := writeManifest(txn, m); err != nil {
		return Receipt{}, err
	}
	if err := options.inject(FaultManifestPhaseUpdate); err != nil {
		return Receipt{}, err
	}
	if options.PostCanonical != nil {
		published, err := options.PostCanonical(resultSnapshot)
		if err != nil {
			return Receipt{}, fmt.Errorf("post-canonical publication failed; transaction %s remains recoverable: %w", set.OperationID, err)
		}
		if published.CatalogSnapshot != resultSnapshot || published.Generation == "" {
			return Receipt{}, fmt.Errorf("post-canonical publication returned mismatched result; transaction %s remains recoverable", set.OperationID)
		}
		m.PublishedCatalogSnapshot = published.CatalogSnapshot
		m.PublishedGeneration = published.Generation
		m.Phase = "catalog_published"
		if err := writeManifest(txn, m); err != nil {
			return Receipt{}, err
		}
		if err := options.inject(FaultManifestPhaseUpdate); err != nil {
			return Receipt{}, err
		}
	}
	m.Phase = "finalized"
	if err := writeManifest(txn, m); err != nil {
		return Receipt{}, err
	}
	if err := options.inject(FaultManifestPhaseUpdate); err != nil {
		return Receipt{}, err
	}
	if err := removeTransaction(txn); err != nil {
		return Receipt{}, fmt.Errorf("remove finalized transaction: %w", err)
	}
	if err := options.inject(FaultJournalCleanup); err != nil {
		return Receipt{}, err
	}
	dirty, err := gitDirty(root)
	if err != nil {
		return Receipt{}, err
	}
	paths := make([]string, 0, len(entries))
	for _, item := range entries {
		paths = append(paths, item.Path)
	}
	return Receipt{
		OperationID: set.OperationID, ChangedPaths: paths, CatalogSnapshot: resultSnapshot,
		Generation: m.PublishedGeneration, GitDirty: dirty,
		SourceSchemaVersion: set.SourceSchemaVersion, TargetSchemaVersion: set.TargetSchemaVersion,
	}, nil
}

func writeManifest(txn string, m manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileSync(txn, "manifest.json", append(data, '\n'))
}

func readCanonicalFile(root, relative string) ([]byte, error) {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	return handle.ReadFile(relative)
}

func applyStagedChecked(root, txn string, item change, options Options) error {
	actual, err := digestAt(root, item.Path)
	if err != nil {
		return err
	}
	if actual == item.AfterDigest {
		return nil
	}
	if actual != item.BeforeDigest {
		displacedDigest := ""
		if actual == "" && item.Displaced != "" {
			displacedDigest, err = digestAt(root, item.Displaced)
			if err != nil {
				return err
			}
		}
		if displacedDigest != item.BeforeDigest {
			return fmt.Errorf("%w: path %s changed before replacement", ErrConflict, item.Path)
		}
	}
	var data []byte
	if !item.Delete {
		data, err = readStagedFile(txn, item.Staged)
		if err != nil {
			return err
		}
		if digest(data) != item.AfterDigest {
			return fmt.Errorf("staged digest mismatch for %s", item.Path)
		}
	}
	return replaceCanonicalChecked(root, item, data, options)
}

func validateApplied(root string, m manifest) error {
	for _, item := range m.Files {
		actual, err := digestAt(root, item.Path)
		if err != nil {
			return err
		}
		if actual != item.AfterDigest {
			return fmt.Errorf("%w: applied digest mismatch for %s", ErrRecoveryRequired, item.Path)
		}
	}
	issues, err := canonical.Validate(root)
	if err != nil {
		return err
	}
	if len(issues) != 0 {
		return fmt.Errorf("mutation produced invalid canonical state: %s: %s", issues[0].Path, issues[0].Message)
	}
	snapshot, err := canonical.Scan(root)
	if err != nil {
		return err
	}
	if m.ExpectedResultCatalogSnapshot != "" && snapshot.CatalogSnapshot != m.ExpectedResultCatalogSnapshot {
		return fmt.Errorf("%w: result catalog snapshot mismatch", ErrRecoveryRequired)
	}
	return nil
}

func removeTransaction(txn string) error {
	parent := filepath.Dir(txn)
	if err := os.RemoveAll(txn); err != nil {
		return err
	}
	return syncDir(parent)
}
