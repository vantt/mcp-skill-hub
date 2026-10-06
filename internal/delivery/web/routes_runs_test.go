package web

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestRunEndpoints(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	adapter := fakeSourceAdapter{
		files: map[string]map[string][]byte{
			"r1": {"SKILL.md": []byte("# Version 1\n")},
			"r2": {"SKILL.md": []byte("# Version 2\n")},
		},
		fail: map[string]error{},
	}
	runID := seedRun(t, root, adapter)

	srv := newTestServer(t, root)
	srv.distill = app.DistillService{
		Clock:    app.SystemClock{},
		Adapters: map[string]sourcepkg.Adapter{"filesystem": adapter},
	}

	// 1. in-progress run -> 200 with state
	recGet := get(t, srv, "/api/v1/runs/"+runID)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for get run, got %d: %s", recGet.Code, recGet.Body.String())
	}
	var getResult app.DistillRunResult
	if err := json.Unmarshal(recGet.Body.Bytes(), &getResult); err != nil {
		t.Fatal(err)
	}
	if getResult.Run.ID != runID || getResult.Run.State != "in_progress" {
		t.Fatalf("unexpected get run result: %#v", getResult)
	}

	// 2. RUN-DOESNOTEXIST0 -> 404 with code invalid_request
	recNotFound := get(t, srv, "/api/v1/runs/RUN-DOESNOTEXIST0")
	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent run, got %d: %s", recNotFound.Code, recNotFound.Body.String())
	}
	var errResult app.Result
	if err := json.Unmarshal(recNotFound.Body.Bytes(), &errResult); err != nil {
		t.Fatal(err)
	}
	if errResult.Error == nil || errResult.Error.Code != app.ErrorInvalidRequest {
		t.Fatalf("expected ErrorInvalidRequest, got %#v", errResult)
	}

	// 3. Escaped path traversal ..%2Fetc -> 400
	recBadID := get(t, srv, "/api/v1/runs/..%2Fetc")
	if recBadID.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad run ID, got %d: %s", recBadID.Code, recBadID.Body.String())
	}

	// 4. Cancel -> 200 and state cancelled
	recCancel := postJSON(t, srv, "/api/v1/runs/"+runID+"/cancel", map[string]any{})
	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected 200 for cancel run, got %d: %s", recCancel.Code, recCancel.Body.String())
	}
	var cancelResult app.DistillRunResult
	if err := json.Unmarshal(recCancel.Body.Bytes(), &cancelResult); err != nil {
		t.Fatal(err)
	}
	if cancelResult.Run.State != "cancelled" {
		t.Fatalf("expected state 'cancelled', got %q", cancelResult.Run.State)
	}

	// 5. Cancel an already cancelled run -> idempotent 200
	recCancelAgain := postJSON(t, srv, "/api/v1/runs/"+runID+"/cancel", map[string]any{})
	if recCancelAgain.Code != http.StatusOK {
		t.Fatalf("expected 200 for idempotent cancel, got %d: %s", recCancelAgain.Code, recCancelAgain.Body.String())
	}
	var cancelAgainResult app.DistillRunResult
	if err := json.Unmarshal(recCancelAgain.Body.Bytes(), &cancelAgainResult); err != nil {
		t.Fatal(err)
	}
	if cancelAgainResult.Run.State != "cancelled" {
		t.Fatalf("expected state 'cancelled' on idempotent cancel, got %q", cancelAgainResult.Run.State)
	}
}
func TestRoutesNeverMutateRuns(t *testing.T) {
	t.Parallel()

	// 1. Static AST inspection of all non-test .go files in web package
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read web directory: %v", err)
	}

	var patterns []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", entry.Name(), err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "HandleFunc" || sel.Sel.Name == "Handle" {
				if len(call.Args) == 0 {
					t.Fatal("HandleFunc/Handle call has no arguments")
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("non-string-literal route pattern in %s: %#v", fset.Position(call.Pos()), call.Args[0])
				}
				pattern, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("failed to unquote route pattern %s: %v", lit.Value, err)
				}
				patterns = append(patterns, pattern)
			}
			return true
		})
	}

	if len(patterns) < 20 {
		t.Fatalf("expected at least 20 route patterns, found %d: %v", len(patterns), patterns)
	}

	hasGetRun := false
	hasCancelRun := false
	for _, p := range patterns {
		if p == "GET /api/v1/runs/{id}" {
			hasGetRun = true
		}
		if p == "POST /api/v1/runs/{id}/cancel" {
			hasCancelRun = true
		}
		lower := strings.ToLower(p)
		if strings.Contains(lower, "start") || strings.Contains(lower, "retry") || strings.Contains(lower, "submit") {
			t.Fatalf("route pattern contains forbidden mutating verb (start, retry, submit): %s", p)
		}
		if strings.Contains(p, "/runs/") && !strings.HasPrefix(p, "GET ") && p != "POST /api/v1/runs/{id}/cancel" {
			t.Fatalf("unexpected non-GET /runs/ route pattern: %s", p)
		}
	}
	if !hasGetRun {
		t.Fatal("missing route pattern 'GET /api/v1/runs/{id}'")
	}
	if !hasCancelRun {
		t.Fatal("missing route pattern 'POST /api/v1/runs/{id}/cancel'")
	}

	// 2. HTTP probe subtest: POST /api/v1/runs/RUN-X/start|retry|submit -> 404 or 405
	t.Run("forbidden mutate endpoints return 404 or 405", func(t *testing.T) {
		root := newWebWorkspace(t)
		srv := newTestServer(t, root)

		for _, action := range []string{"start", "retry", "submit"} {
			rec := postJSON(t, srv, "/api/v1/runs/RUN-X/"+action, map[string]any{})
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 404 or 405 for POST /runs/RUN-X/%s, got %d: %s", action, rec.Code, rec.Body.String())
			}
		}
	})
}

func TestRunGolden(t *testing.T) {
	root := newWebWorkspace(t)
	adapter := fakeSourceAdapter{
		files: map[string]map[string][]byte{
			"r1": {"SKILL.md": []byte("# Version 1\n")},
			"r2": {"SKILL.md": []byte("# Version 2\n")},
		},
		fail: map[string]error{},
	}
	runID := seedRun(t, root, adapter)
	srv := newTestServer(t, root)
	srv.distill = app.DistillService{
		Clock:    app.SystemClock{},
		Adapters: map[string]sourcepkg.Adapter{"filesystem": adapter},
	}

	rec := get(t, srv, "/api/v1/runs/"+runID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Also verify seedFinalizedRun loads successfully in a separate workspace
	rootFinal := newWebWorkspace(t)
	srvFinal := newTestServer(t, rootFinal)
	srvFinal.distill = app.DistillService{
		Clock:    app.SystemClock{},
		Adapters: map[string]sourcepkg.Adapter{"filesystem": adapter},
	}
	finalizedRunID := seedFinalizedRun(t, rootFinal, adapter)
	recFinalized := get(t, srvFinal, "/api/v1/runs/"+finalizedRunID)
	if recFinalized.Code != http.StatusOK {
		t.Fatalf("expected 200 for finalized run, got %d: %s", recFinalized.Code, recFinalized.Body.String())
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, rec.Body.Bytes(), "", "  "); err != nil {
		t.Fatalf("failed to indent response JSON: %v", err)
	}
	pretty.WriteString("\n")

	normalized := normalizeGolden(pretty.String(), root)
	goldenPath := filepath.Join("testdata", "golden", "run.json")

	if *update {
		if err := os.WriteFile(goldenPath, []byte(normalized), 0o644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file %s does not exist; run with -update to generate", goldenPath)
		}
		t.Fatalf("failed to read golden file: %v", err)
	}

	if string(expected) != normalized {
		t.Fatalf("golden mismatch for run.json:\nwant:\n%s\ngot:\n%s", string(expected), normalized)
	}
}
