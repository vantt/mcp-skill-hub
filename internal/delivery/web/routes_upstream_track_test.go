package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

// untrackedUpstreamMeta is the metadata of a skill vendored from GitHub whose
// upstream entry has no source ID yet.
const untrackedUpstreamMeta = `description: Plan runner.
id: plan-runner
name: Plan Runner
schema_version: 1
sources:
    - commit: 619b8cdea5de1f57d48814c5c87ba623575bb41d
      folder_digest: sha256:0382941395c0d6eca961c5a7005d8034c3d1ce690985e362147e35603d15c71d
      id: ""
      kind: github
      path: plan-runner
      ref: main
      repo: https://github.com/example/plan-runner-upstream
      roles:
        - upstream
      synced: 619b8cdea5de1f57d48814c5c87ba623575bb41d
status: draft
`

func addUntrackedUpstreamSkill(t *testing.T, root string) string {
	t.Helper()
	ctx := context.Background()
	service := app.SkillService{}
	preview, err := service.PreviewCreate(ctx, root, skill.CreateInput{ID: "plan-runner", Collection: "core", Name: "Plan Runner", Description: "Plan runner."}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create = %#v, %v", result, err)
	}
	metaPath := filepath.Join(root, "skills", "core", "plan-runner", ".meta", "skill.yaml")
	if err := os.WriteFile(metaPath, []byte(untrackedUpstreamMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	return metaPath
}

type trackPreviewResponse struct {
	Status    string `json:"status"`
	FullDiff  string `json:"full_diff"`
	Candidate struct {
		SourceID     string `json:"source_id"`
		CreateSource bool   `json:"create_source"`
	} `json:"candidate"`
	Diff struct {
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
	} `json:"diff"`
	Confirmation struct {
		Confirmation struct {
			Pins app.ConfirmationPins `json:"pins"`
		} `json:"confirmation"`
	} `json:"confirmation"`
}

func previewTrack(t *testing.T, srv *Server, id string) trackPreviewResponse {
	t.Helper()
	rec := postJSON(t, srv, "/api/v1/skills/"+id+"/upstream/track/preview", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("track preview status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res trackPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode track preview: %v (body: %s)", err, rec.Body.String())
	}
	return res
}

func confirmSourceProposal(t *testing.T, srv *Server, pins app.ConfirmationPins) (int, string) {
	t.Helper()
	rec := postJSON(t, srv, "/api/v1/sources/proposals/"+pins.ProposalID+"/confirm", map[string]string{
		"proposal_digest": pins.ProposalDigest,
		"base_version":    pins.BaseVersion,
	})
	return rec.Code, rec.Body.String()
}

func TestTrackUpstreamPreviewAndConfirmTracksTheUpstream(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	metaPath := addUntrackedUpstreamSkill(t, root)
	srv := newTestServer(t, root)

	res := previewTrack(t, srv, "plan-runner")
	if res.Status != string(app.StatusActionRequired) {
		t.Fatalf("preview status = %q", res.Status)
	}
	if res.Candidate.SourceID != "plan-runner-upstream" || !res.Candidate.CreateSource {
		t.Fatalf("candidate = %#v", res.Candidate)
	}
	pins := res.Confirmation.Confirmation.Pins
	if pins.ProposalID == "" || pins.ProposalDigest == "" || pins.BaseVersion == "" {
		t.Fatalf("pins = %#v", pins)
	}
	if !strings.Contains(res.FullDiff, "+++ b/sources/catalog/plan-runner-upstream.yaml") ||
		!strings.Contains(res.FullDiff, "+      id: plan-runner-upstream") {
		t.Fatalf("full diff = %s", res.FullDiff)
	}
	if current, _ := os.ReadFile(metaPath); string(current) != untrackedUpstreamMeta {
		t.Fatalf("preview changed the skill metadata:\n%s", current)
	}

	code, body := confirmSourceProposal(t, srv, pins)
	if code != http.StatusOK || !strings.Contains(body, "Now tracking upstream plan-runner-upstream for plan-runner.") {
		t.Fatalf("confirm status = %d, body = %s", code, body)
	}

	rec := get(t, srv, "/api/v1/skills/plan-runner/sources")
	if rec.Code != http.StatusOK {
		t.Fatalf("sources status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var sources struct {
		Upstream *struct {
			SourceID string `json:"source_id"`
			Status   string `json:"status"`
		} `json:"upstream"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sources); err != nil {
		t.Fatal(err)
	}
	if sources.Upstream == nil || sources.Upstream.SourceID != "plan-runner-upstream" || sources.Upstream.Status == "untracked" {
		t.Fatalf("sources after confirm = %s", rec.Body.String())
	}

	// Replaying the same confirmation returns the applied operation again
	// without writing anything twice.
	if code, body := confirmSourceProposal(t, srv, pins); code != http.StatusOK {
		t.Fatalf("replayed confirm status = %d, body = %s", code, body)
	}

	again := previewTrack(t, srv, "plan-runner")
	if again.Status != string(app.StatusOK) || again.Confirmation.Confirmation.Pins.ProposalID != "" {
		t.Fatalf("preview of a tracked upstream = %#v", again)
	}
}

func TestTrackUpstreamConfirmIsStaleAfterTheMetadataChanged(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	metaPath := addUntrackedUpstreamSkill(t, root)
	srv := newTestServer(t, root)

	pins := previewTrack(t, srv, "plan-runner").Confirmation.Confirmation.Pins
	edited := strings.Replace(untrackedUpstreamMeta, "description: Plan runner.", "description: Plan runner, edited.", 1)
	if err := os.WriteFile(metaPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	code, body := confirmSourceProposal(t, srv, pins)
	if code != http.StatusConflict || !strings.Contains(body, string(app.ErrorStaleProposal)) {
		t.Fatalf("stale confirm status = %d, body = %s", code, body)
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "plan-runner-upstream.yaml")); !os.IsNotExist(err) {
		t.Fatalf("stale confirm wrote the source record: %v", err)
	}
}

func TestTrackUpstreamPreviewUnknownSkillIsNotFound(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, newWebWorkspace(t))
	rec := postJSON(t, srv, "/api/v1/skills/missing-skill/upstream/track/preview", map[string]any{})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
