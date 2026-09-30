package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestDeleteRuntimeRebuildsOfflineAndRecreatesDisposableDatabases(t *testing.T) {
	root := newWorkspace(t)
	writeCanonical(t, root, "skills/core/review/skill.meta.yaml", "schema_version: 1\nid: review\nname: Review\nstatus: active\ndescription: Review code.\naliases: [patch-inspector]\nrouting:\n  triggers: [review code]\n  operations: [review]\n  not_for: [write prose]\n  min_scope: multi_step\n")
	writeCanonical(t, root, "skills/core/review/SKILL.md", "# Review\nUse evidence.\n")
	first := build(t, root, BuildOptions{})
	if err := os.RemoveAll(filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	second := build(t, root, BuildOptions{})
	if first.Pointer.CatalogSnapshot != second.Pointer.CatalogSnapshot || first.Pointer.ProjectionInputDigest != second.Pointer.ProjectionInputDigest {
		t.Fatal("delete-runtime rebuild changed logical digests")
	}
	for _, relative := range []string{"runtime/operational.db", "runtime/telemetry.db", "runtime/catalog/current.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("rebuild did not recreate %s: %v", relative, err)
		}
	}
	for _, check := range []struct{ file, table string }{{"operational.db", "operational_metadata"}, {"telemetry.db", "telemetry_events"}} {
		database, err := sql.Open("sqlite", sqliteDSN(filepath.Join(root, "runtime", check.file), true))
		if err != nil {
			t.Fatal(err)
		}
		var count int
		err = database.QueryRow(`SELECT count(*) FROM ` + check.table).Scan(&count)
		closeErr := database.Close()
		if err != nil || closeErr != nil || count != 0 {
			t.Fatalf("%s was not recreated empty: count=%d query=%v close=%v", check.file, count, err, closeErr)
		}
	}
	handle, err := OpenCurrent(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	var resources int
	if err := handle.DB.QueryRow(`SELECT count(*) FROM resources`).Scan(&resources); err != nil || resources != 1 {
		t.Fatalf("resource rows = %d, %v", resources, err)
	}
	var matches int
	if err := handle.DB.QueryRow(`SELECT count(*) FROM skill_fts WHERE skill_fts MATCH 'review'`).Scan(&matches); err != nil || matches != 1 {
		t.Fatalf("skill FTS matches = %d, %v", matches, err)
	}
	if err := handle.DB.QueryRow(`SELECT count(*) FROM skill_fts WHERE skill_fts MATCH '"patch-inspector"'`).Scan(&matches); err != nil || matches != 1 {
		t.Fatalf("skill alias FTS matches = %d, %v", matches, err)
	}
}

func TestEquivalentBuildsHaveDeterministicLogicalResults(t *testing.T) {
	root := newWorkspace(t)
	writeCanonical(t, root, "skills/core/review/skill.meta.yaml", "schema_version: 1\nid: review\nname: Review\nstatus: active\ndescription: Review code.\nrouting:\n  triggers: [review code, inspect diff]\n  not_for: [write prose]\n  min_scope: multi_step\n")
	writeCanonical(t, root, "skills/core/review/SKILL.md", "# Review\n")
	writeCanonical(t, root, "sources/catalog/upstream.yaml", "schema_version: 1\nid: upstream\nadapter: git\nlocator:\n  repository: https://example.invalid/repo\nrevision:\n  kind: git-tree\n  value: abc\n  content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n  observed_at: \"2026-09-28T10:10:00Z\"\n")
	first := build(t, root, BuildOptions{BuilderVersion: "test"})
	firstRows := logicalRows(t, root)
	second := build(t, root, BuildOptions{BuilderVersion: "test"})
	secondRows := logicalRows(t, root)
	if first.Pointer.CatalogSnapshot != second.Pointer.CatalogSnapshot || first.Pointer.ProjectionInputDigest != second.Pointer.ProjectionInputDigest {
		t.Fatal("same canonical bytes produced different digests")
	}
	if !reflect.DeepEqual(first.RowCounts, second.RowCounts) || !reflect.DeepEqual(firstRows, secondRows) {
		t.Fatalf("logical projections differ:\n%v\n%v", firstRows, secondRows)
	}
}

func TestCanonicalChangeDuringBuildRejectsPublish(t *testing.T) {
	root := newWorkspace(t)
	initial := build(t, root, BuildOptions{})
	_, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{BeforePublish: func() error {
		writeCanonical(t, root, "sources/catalog/SRC-CHANGED.yaml", "schema_version: 1\nid: SRC-CHANGED\nadapter: git\nlocator: https://example.invalid/repo\n")
		return nil
	}})
	if !errors.Is(err, ErrCanonicalChanged) || !strings.Contains(err.Error(), "canonical_changed_during_rebuild") {
		t.Fatalf("changed build input was published: %v", err)
	}
	current, err := readPointer(root)
	if err != nil {
		t.Fatal(err)
	}
	if current.Generation != initial.Pointer.Generation {
		t.Fatal("failed rebuild replaced the previous pointer")
	}
}

func TestCorruptGenerationPreservesPreviousPointer(t *testing.T) {
	root := newWorkspace(t)
	initial := build(t, root, BuildOptions{})
	_, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{BeforeVerify: func(path string) error {
		return os.WriteFile(path, []byte("not sqlite"), 0o600)
	}})
	if err == nil {
		t.Fatal("corrupt generation passed verification")
	}
	current, pointerErr := readPointer(root)
	if pointerErr != nil {
		t.Fatal(pointerErr)
	}
	if current.Generation != initial.Pointer.Generation {
		t.Fatal("corrupt generation replaced the previous pointer")
	}
}

func TestSourceOnlyChangeAltersProjectionDigestNotCatalogSnapshot(t *testing.T) {
	root := newWorkspace(t)
	writeCanonical(t, root, "sources/catalog/SRC-1.yaml", "schema_version: 1\nid: SRC-1\nadapter: git\nlocator: https://example.invalid/repo\n")
	first := build(t, root, BuildOptions{})
	writeCanonical(t, root, "sources/catalog/SRC-1.yaml", "schema_version: 1\nid: SRC-1\nadapter: git\nlocator: changed\n")
	second := build(t, root, BuildOptions{})
	if first.Pointer.CatalogSnapshot != second.Pointer.CatalogSnapshot {
		t.Fatal("source-only change altered catalog snapshot")
	}
	if first.Pointer.ProjectionInputDigest == second.Pointer.ProjectionInputDigest {
		t.Fatal("source-only change did not alter projection input digest")
	}
}

func TestOperationReceiptsRestoreIdempotencyProjectionWithoutReplay(t *testing.T) {
	root := newWorkspace(t)
	set := mutation.WriteSet{OperationID: "OP-RECEIPT", Command: "create_source", IdempotencyKey: "source:create:1", Changes: []mutation.Change{{Path: "sources/catalog/source.yaml", Contents: []byte("id: SRC-1\nadapter: git\n")}}}
	if _, err := mutation.Commit(root, set); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	// The receipt is an audit/idempotency input, not an event to replay. An
	// external canonical edit may remove the current source independently.
	if err := os.Remove(filepath.Join(root, "sources", "catalog", "source.yaml")); err != nil {
		t.Fatal(err)
	}
	build(t, root, BuildOptions{})
	handle, err := OpenCurrent(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	var operationID, requestDigest string
	if err := handle.DB.QueryRow(`SELECT id,request_digest FROM operations WHERE idempotency_key=?`, "source:create:1").Scan(&operationID, &requestDigest); err != nil {
		t.Fatal(err)
	}
	if operationID != "OP-RECEIPT" || !strings.HasPrefix(requestDigest, "sha256:") {
		t.Fatalf("unexpected idempotency projection: %s %s", operationID, requestDigest)
	}
	var sources int
	if err := handle.DB.QueryRow(`SELECT count(*) FROM sources`).Scan(&sources); err != nil || sources != 0 {
		t.Fatalf("operation receipt was replayed into source state: %d, %v", sources, err)
	}
}

func TestPinnedOldGenerationIsDeferredUntilClose(t *testing.T) {
	root := newWorkspace(t)
	first := build(t, root, BuildOptions{})
	handle, err := OpenCurrent(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	writeCanonical(t, root, "sources/catalog/SRC-1.yaml", "schema_version: 1\nid: SRC-1\nadapter: git\nlocator: https://example.invalid/repo\n")
	build(t, root, BuildOptions{})
	if err := CollectGarbage(root, 0); err != nil {
		t.Fatal(err)
	}
	oldPath := generationPath(root, first.Pointer)
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("pinned generation was collected: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if err := CollectGarbage(root, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unpinned old generation was not collected: %v", err)
	}
}

func TestStrictProjectionValidationRejectsInvalidCanonicalEntities(t *testing.T) {
	cases := []struct {
		name, path, contents, want string
	}{
		{"schema", "sources/catalog/source.yaml", "id: source\nadapter: git\nlocator: x\n", "schema_version must be 1"},
		{"path identity", "sources/catalog/wrong.yaml", "schema_version: 1\nid: source\nadapter: git\nlocator: x\n", "does not match path identity"},
		{"required", "skills/core/skill/skill.meta.yaml", "schema_version: 1\nid: skill\nname: Skill\nstatus: draft\n", "description must be"},
		{"enum", "sources/intake/candidate.yaml", "schema_version: 1\nid: candidate\nlocator: x\ncaptured_at: \"2026-09-28T10:00:00Z\"\nreason: x\nstatus: unknown\n", "status must be"},
		{"timestamp", "sources/intake/candidate.yaml", "schema_version: 1\nid: candidate\nlocator: x\ncaptured_at: yesterday\nreason: x\nstatus: pending\n", "RFC3339"},
		{"digest", "sources/catalog/source.yaml", "schema_version: 1\nid: source\nadapter: git\nlocator: x\nrevision:\n  kind: git-tree\n  value: abc\n  content_digest: bad\n  observed_at: \"2026-09-28T10:00:00Z\"\n", "lowercase SHA-256"},
		{"dangling", "sources/skills/link.yaml", "schema_version: 1\nid: link\nskill_id: missing\nsource_id: absent\n", "broken reference"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := newWorkspace(t)
			writeCanonical(t, root, test.path, test.contents)
			if strings.HasSuffix(test.path, "/skill.meta.yaml") {
				writeCanonical(t, root, filepath.ToSlash(filepath.Join(filepath.Dir(test.path), "SKILL.md")), "# Fixture\n")
			}
			_, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestFaultInjectionPreservesRecoverablePointer(t *testing.T) {
	for _, point := range []FaultPoint{FaultDatabaseClose, FaultGenerationSync, FaultGenerationsDirSync, FaultPointerTempSync, FaultPointerRename, FaultPointerDirSync} {
		t.Run(string(point), func(t *testing.T) {
			root := newWorkspace(t)
			initial := build(t, root, BuildOptions{})
			injected := errors.New("injected " + string(point))
			result, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{Fault: func(actual FaultPoint) error {
				if actual == point {
					return injected
				}
				return nil
			}})
			if point == FaultPointerDirSync {
				if err != nil || result.Freshness != FreshnessIndeterminate || len(result.Warnings) == 0 || !strings.Contains(result.FreshnessDetail, injected.Error()) {
					t.Fatalf("post-publish fault result = %#v, %v", result, err)
				}
			} else if !errors.Is(err, injected) {
				t.Fatalf("fault result = %v", err)
			}
			pointer, pointerErr := readPointer(root)
			if pointerErr != nil {
				t.Fatalf("pointer is not recoverable: %v", pointerErr)
			}
			if point == FaultPointerDirSync {
				if pointer.Generation == initial.Pointer.Generation {
					t.Fatal("post-rename fault did not expose the newly renamed pointer")
				}
				if _, statErr := os.Stat(generationPath(root, pointer)); statErr != nil {
					t.Fatalf("post-rename fault deleted referenced generation: %v", statErr)
				}
			} else if pointer.Generation != initial.Pointer.Generation {
				t.Fatal("pre-rename fault replaced previous pointer")
			}
		})
	}
}

func TestPostPublishCanonicalChangeReturnsExplicitStaleResult(t *testing.T) {
	root := newWorkspace(t)
	result, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{AfterPointerPublish: func() {
		writeCanonical(t, root, "sources/catalog/source.yaml", "schema_version: 1\nid: source\nadapter: git\nlocator: https://example.invalid/repo\n")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Freshness != FreshnessStale || result.FreshnessDetail == "" {
		t.Fatalf("freshness = %q (%s)", result.Freshness, result.FreshnessDetail)
	}
	if pointer, err := readPointer(root); err != nil || pointer.Generation != result.Pointer.Generation {
		t.Fatalf("stale publication pointer was removed: %#v, %v", pointer, err)
	}
}

func TestInspectPublishedReportsUnknownWithoutCanonicalFreshnessRead(t *testing.T) {
	root := newWorkspace(t)
	build(t, root, BuildOptions{})
	writeCanonical(t, root, "sources/catalog/source.yaml", "schema_version: 1\nid: source\nadapter: git\nlocator: https://example.invalid/repo\n")
	published, err := InspectPublished(context.Background(), root)
	if err != nil || published.State != StateUnknown || published.Pointer == nil {
		t.Fatalf("published-only inspect = %#v, %v", published, err)
	}
	full, err := Inspect(context.Background(), root)
	if err != nil || full.State != StateStale {
		t.Fatalf("full inspect = %#v, %v", full, err)
	}
}

func TestInspectReportsMissingStaleAndHealthy(t *testing.T) {
	root := newWorkspace(t)
	status, err := Inspect(context.Background(), root)
	if err != nil || status.State != StateMissing {
		t.Fatalf("missing inspect = %#v, %v", status, err)
	}
	build(t, root, BuildOptions{})
	status, err = Inspect(context.Background(), root)
	if err != nil || status.State != StateHealthy {
		t.Fatalf("healthy inspect = %#v, %v", status, err)
	}
	writeCanonical(t, root, "sources/catalog/source.yaml", "schema_version: 1\nid: source\nadapter: git\nlocator: https://example.invalid/repo\n")
	status, err = Inspect(context.Background(), root)
	if err != nil || status.State != StateStale {
		t.Fatalf("stale inspect = %#v, %v", status, err)
	}
}

func TestCorruptDisposableDatabasesAreResetWithoutBlockingCatalog(t *testing.T) {
	root := newWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operational.db", "telemetry.db"} {
		if err := os.WriteFile(filepath.Join(root, "runtime", name), []byte("corrupt"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result := build(t, root, BuildOptions{})
	if len(result.Warnings) != 2 {
		t.Fatalf("runtime reset warnings = %#v", result.Warnings)
	}
	if status, err := Inspect(context.Background(), root); err != nil || status.State != StateHealthy {
		t.Fatalf("catalog invalidated by disposable DBs: %#v, %v", status, err)
	}
}

func TestGarbageCollectionHonorsAgeAndReclaimsAbandonedPins(t *testing.T) {
	root := newWorkspace(t)
	first := build(t, root, BuildOptions{})
	handle, err := OpenCurrent(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	writeCanonical(t, root, "sources/catalog/source.yaml", "schema_version: 1\nid: source\nadapter: git\nlocator: https://example.invalid/repo\n")
	build(t, root, BuildOptions{})
	if err := CollectGarbage(root, time.Hour); err != nil {
		t.Fatal(err)
	}
	oldPath := generationPath(root, first.Pointer)
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("young generation was collected: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	pinDir := filepath.Join(root, "runtime", "catalog", "pins", first.Pointer.Generation)
	if err := os.MkdirAll(pinDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pin := filepath.Join(pinDir, "abandoned.pin")
	if err := os.WriteFile(pin, []byte(`{"pid":2147483647}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(pin, old, old); err != nil {
		t.Fatal(err)
	}
	if err := CollectGarbage(root, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned pinned generation was not collected: %v", err)
	}
}

func TestAllRecognizedKindsProjectIntoSupportedTables(t *testing.T) {
	root := newWorkspace(t)
	writeCanonical(t, root, "skills/core/skill/skill.meta.yaml", "schema_version: 1\nid: skill\nname: Alpha Skill\nstatus: active\ndescription: Alpha projection fixture.\nrouting:\n  triggers: [alpha]\n  not_for: [beta]\n  min_scope: multi_step\n")
	writeCanonical(t, root, "skills/core/skill/SKILL.md", "# Alpha fixture token\n")
	writeCanonical(t, root, "sources/catalog/source.yaml", "schema_version: 1\nid: source\nadapter: git\nlocator: https://example.invalid/repo\nrevision:\n  kind: git-tree\n  value: abc\n  content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n  observed_at: \"2026-09-28T10:00:00Z\"\n")
	writeCanonical(t, root, "sources/intake/candidate.yaml", "schema_version: 1\nid: candidate\nlocator: https://example.invalid/candidate\ncaptured_at: \"2026-09-28T10:00:00Z\"\nreason: useful\nstatus: pending\n")
	writeCanonical(t, root, "sources/skills/link.yaml", "schema_version: 1\nid: link\nskill_id: skill\nsource_id: source\n")
	writeCanonical(t, root, "distill/sources/source/observations/OBS-source--observation.yaml", "schema_version: 1\nid: OBS-source--observation\nsource_id: source\nrun_id: RUN-fixture\nstable_key: observation\nstatus: active\nfirst_seen: {kind: git-tree, value: abc, content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}\nlast_seen: {kind: git-tree, value: abc, content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}\nwhat: observed alpha behavior\nvocabulary: [alpha]\nevidence:\n  - revision: {kind: git-tree, value: abc, content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}\n    run_id: RUN-fixture\n    package_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n    path: SKILL.md\n    locator: SKILL.md\n    digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
	writeCanonical(t, root, "distill/sources/source/findings/OBS-source--finding.yaml", "schema_version: 1\nid: OBS-source--finding\nsource_id: source\nrun_id: RUN-fixture\nstable_key: finding\nstatus: active\nfirst_seen: {kind: git-tree, value: abc, content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}\nlast_seen: {kind: git-tree, value: abc, content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}\nwhat: finding alpha behavior\nvocabulary: [alpha]\nevidence:\n  - revision: {kind: git-tree, value: abc, content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}\n    run_id: RUN-fixture\n    package_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n    path: SKILL.md\n    locator: SKILL.md\n    digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
	writeCanonical(t, root, "distill/provenance/runs/RUN-fixture.yaml", "schema_version: 1\nid: RUN-fixture\nsource_id: source\nstate: prepared\nto_revision:\n  kind: git-tree\n  value: abc\n  content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n  observed_at: 2026-09-28T10:00:00Z\nchanged_resources: []\npackage_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nprepared_at: 2026-09-28T10:00:00Z\nattempt: 0\n")
	writeCanonical(t, root, "distill/comparisons/CMP-fixture.yaml", "schema_version: 1\nid: CMP-fixture\nrun_id: RUN-fixture\nsubject: alpha\nobservation_ids: [OBS-source--observation]\nverdict: convergent\ntradeoffs: none\nbased_on:\n  OBS-source--observation: {kind: git-tree, value: abc, content_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}\nstale: false\n")
	writeCanonical(t, root, "distill/skills/skill/insights/INS-skill--improve-alpha.yaml", "schema_version: 1\nid: INS-skill--improve-alpha\nrun_id: RUN-fixture\nstable_key: improve-alpha\nskill_id: skill\nstatus: pending\nrecommendation: improve alpha\nobservation_ids: [OBS-source--observation]\ncomparison_ids: [CMP-fixture]\ncategory: quality\npriority: medium\nrationale: alpha evidence\n")
	digest := "sha256:" + strings.Repeat("b", 64)
	writeCanonical(t, root, "distill/skills/skill/proposals/proposal.yaml", "schema_version: 1\nid: proposal\ninsight_id: INS-skill--improve-alpha\nbase_catalog_version: "+digest+"\ndigest: "+digest+"\nstatus: approved\nchanged_files: [skills/core/skill/SKILL.md]\npath_pins:\n  - path: skills/core/skill/SKILL.md\n    before: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n    after: "+digest+"\ncreated_at: 2026-09-28T10:00:00Z\n")
	writeCanonical(t, root, "history/operations/2026/operation.yaml", "schema_version: 1\nid: operation\nkind: fixture\noccurred_at: \"2026-09-28T10:00:00Z\"\nidempotency_key: fixture-key\nrequest_digest: "+digest+"\nbase_catalog_snapshot: "+digest+"\nresult_catalog_snapshot: "+digest+"\nstatus: applied\nchanges: []\n")
	writeCanonical(t, root, "distill/skills/skill/incorporations/incorporation.yaml", "schema_version: 1\nid: incorporation\ninsight_id: INS-skill--improve-alpha\nproposal_id: proposal\noperation_id: operation\nstate: incorporated\ntargets: [skills/core/skill/SKILL.md]\nsource_to_local:\n  - observation_id: OBS-source--observation\n    artifact_path: skills/core/skill/SKILL.md\n    concept: alpha-review\nincorporated_at: 2026-09-28T10:00:00Z\n")
	writeCanonical(t, root, "distill/skills/skill/outcomes/outcome.yaml", "schema_version: 1\nid: outcome\nincorporation_id: incorporation\nstate: confirmed\nevidence: [review:fixture]\nnote: Explicit fixture review.\nrecorded_at: 2026-09-28T11:00:00Z\n")
	writeCanonical(t, root, "registry/collections/collection.yaml", "schema_version: 1\nid: collection\n")
	writeCanonical(t, root, "evals/routing/cases/evaluation.yaml", "schema_version: 1\nid: evaluation\n")
	result := build(t, root, BuildOptions{})
	for _, table := range countedTables {
		if result.RowCounts[table] == 0 && table != "routing_documents" {
			t.Fatalf("supported table %s has no fixture row", table)
		}
	}
}

func newWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeCanonical(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func build(t *testing.T, root string, options BuildOptions) BuildResult {
	t.Helper()
	result, err := BuildCatalogGeneration(context.Background(), root, options)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func logicalRows(t *testing.T, root string) []string {
	t.Helper()
	handle, err := OpenCurrent(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	rows, err := handle.DB.Query(`SELECT kind,id,path,content_json FROM canonical_entities ORDER BY kind,id,path`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var kind, id, path, content string
		if err := rows.Scan(&kind, &id, &path, &content); err != nil {
			t.Fatal(err)
		}
		var normalized any
		if err := json.Unmarshal([]byte(content), &normalized); err != nil {
			t.Fatal(err)
		}
		result = append(result, kind+"\x00"+id+"\x00"+path+"\x00"+content)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
